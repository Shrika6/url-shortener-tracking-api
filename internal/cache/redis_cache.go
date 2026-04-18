package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	appmetrics "github.com/shrika/url-shortener-tracking-api/internal/metrics"
	"github.com/shrika/url-shortener-tracking-api/internal/models"
)

const (
	urlPrefixKey          = "shortener:url:"
	pendingClicksPrefix   = "shortener:clicks:pending:"
	lastAccessPrefix      = "shortener:last_access:"
	clickEventsQueueKey   = "shortener:click_events"
	lastAccessRetention   = 30 * 24 * time.Hour
)

type URLCache interface {
	GetURL(ctx context.Context, shortCode string) (*models.CachedURL, error)
	SetURL(ctx context.Context, shortCode string, value models.CachedURL, ttl time.Duration) error
	TrackClick(ctx context.Context, urlID uuid.UUID, accessedAt time.Time) error
	DequeueClickEvents(ctx context.Context, batchSize int64) ([]models.ClickEvent, error)
	RequeueClickEvents(ctx context.Context, events []models.ClickEvent) error
	GetPendingClicks(ctx context.Context, urlID uuid.UUID) (int64, error)
	DecrementPendingClicks(ctx context.Context, urlID uuid.UUID, amount int64) error
	GetLastAccess(ctx context.Context, urlID uuid.UUID) (*time.Time, error)
}

type RedisCache struct {
	client  *redis.Client
	metrics *appmetrics.Metrics
}

type clickEventPayload struct {
	URLID      string `json:"url_id"`
	AccessedAt string `json:"accessed_at"`
}

func NewRedisClient(redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return redis.NewClient(opts), nil
}

func NewRedisCache(client *redis.Client, metrics *appmetrics.Metrics) *RedisCache {
	return &RedisCache{client: client, metrics: metrics}
}

func (c *RedisCache) GetURL(ctx context.Context, shortCode string) (*models.CachedURL, error) {
	start := time.Now()
	defer c.metrics.ObserveRedisOperation("get_url", time.Since(start))

	val, err := c.client.Get(ctx, urlCacheKey(shortCode)).Result()
	if err != nil {
		if err == redis.Nil {
			c.metrics.IncCacheMiss()
			return nil, nil
		}
		return nil, err
	}

	var item models.CachedURL
	if err := json.Unmarshal([]byte(val), &item); err != nil {
		return nil, err
	}
	c.metrics.IncCacheHit()

	return &item, nil
}

func (c *RedisCache) SetURL(ctx context.Context, shortCode string, value models.CachedURL, ttl time.Duration) error {
	start := time.Now()
	defer c.metrics.ObserveRedisOperation("set_url", time.Since(start))

	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, urlCacheKey(shortCode), payload, ttl).Err()
}

func (c *RedisCache) TrackClick(ctx context.Context, urlID uuid.UUID, accessedAt time.Time) error {
	start := time.Now()
	defer c.metrics.ObserveRedisOperation("track_click", time.Since(start))

	accessedAt = accessedAt.UTC()
	payload, err := json.Marshal(clickEventPayload{
		URLID:      urlID.String(),
		AccessedAt: accessedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return err
	}

	pipe := c.client.TxPipeline()
	pipe.LPush(ctx, clickEventsQueueKey, payload)
	pipe.Incr(ctx, pendingClicksKey(urlID))
	pipe.Set(ctx, lastAccessKey(urlID), accessedAt.Format(time.RFC3339Nano), lastAccessRetention)
	_, err = pipe.Exec(ctx)
	return err
}

func (c *RedisCache) DequeueClickEvents(ctx context.Context, batchSize int64) ([]models.ClickEvent, error) {
	start := time.Now()
	defer c.metrics.ObserveRedisOperation("dequeue_click_events", time.Since(start))

	if batchSize <= 0 {
		return nil, nil
	}

	values, err := c.client.RPopCount(ctx, clickEventsQueueKey, int(batchSize)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	events := make([]models.ClickEvent, 0, len(values))
	for _, raw := range values {
		var payload clickEventPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			continue
		}
		id, err := uuid.Parse(payload.URLID)
		if err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, payload.AccessedAt)
		if err != nil {
			continue
		}

		events = append(events, models.ClickEvent{
			URLID:      id,
			AccessedAt: ts.UTC(),
		})
	}

	return events, nil
}

func (c *RedisCache) RequeueClickEvents(ctx context.Context, events []models.ClickEvent) error {
	start := time.Now()
	defer c.metrics.ObserveRedisOperation("requeue_click_events", time.Since(start))

	if len(events) == 0 {
		return nil
	}

	values := make([]interface{}, 0, len(events))
	for i := len(events) - 1; i >= 0; i-- {
		payload, err := json.Marshal(clickEventPayload{
			URLID:      events[i].URLID.String(),
			AccessedAt: events[i].AccessedAt.UTC().Format(time.RFC3339Nano),
		})
		if err != nil {
			return err
		}
		values = append(values, payload)
	}

	return c.client.RPush(ctx, clickEventsQueueKey, values...).Err()
}

func (c *RedisCache) GetPendingClicks(ctx context.Context, urlID uuid.UUID) (int64, error) {
	start := time.Now()
	defer c.metrics.ObserveRedisOperation("get_pending_clicks", time.Since(start))

	count, err := c.client.Get(ctx, pendingClicksKey(urlID)).Int64()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}
	return count, nil
}

func (c *RedisCache) DecrementPendingClicks(ctx context.Context, urlID uuid.UUID, amount int64) error {
	start := time.Now()
	defer c.metrics.ObserveRedisOperation("decrement_pending_clicks", time.Since(start))

	if amount <= 0 {
		return nil
	}

	newValue, err := c.client.DecrBy(ctx, pendingClicksKey(urlID), amount).Result()
	if err != nil {
		if err == redis.Nil {
			return nil
		}
		return err
	}
	if newValue < 0 {
		if err := c.client.Set(ctx, pendingClicksKey(urlID), 0, 0).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (c *RedisCache) GetLastAccess(ctx context.Context, urlID uuid.UUID) (*time.Time, error) {
	start := time.Now()
	defer c.metrics.ObserveRedisOperation("get_last_access", time.Since(start))

	val, err := c.client.Get(ctx, lastAccessKey(urlID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	ts, err := time.Parse(time.RFC3339Nano, val)
	if err != nil {
		return nil, err
	}
	ts = ts.UTC()
	return &ts, nil
}

func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

func urlCacheKey(shortCode string) string {
	return urlPrefixKey + shortCode
}

func pendingClicksKey(urlID uuid.UUID) string {
	return pendingClicksPrefix + urlID.String()
}

func lastAccessKey(urlID uuid.UUID) string {
	return lastAccessPrefix + urlID.String()
}
