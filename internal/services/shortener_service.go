package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shrika/url-shortener-tracking-api/internal/cache"
	"github.com/shrika/url-shortener-tracking-api/internal/models"
	"github.com/shrika/url-shortener-tracking-api/internal/repositories"
	"github.com/shrika/url-shortener-tracking-api/internal/utils"
	"go.uber.org/zap"
)

type URLService interface {
	ShortenURL(ctx context.Context, rawURL string) (*models.ShortenResponse, error)
	ResolveAndTrack(ctx context.Context, shortCode string) (string, error)
	GetStats(ctx context.Context, shortCode string) (*models.StatsResponse, error)
	RunClickSync(ctx context.Context)
	FlushAllPendingClicks(ctx context.Context) error
}

type ShortenerService struct {
	repo              repositories.URLRepository
	cache             cache.URLCache
	logger            *zap.Logger
	baseURL           string
	cacheTTL          time.Duration
	codeLength        int
	flushInterval     time.Duration
	flushBatchSize    int64
}

func NewShortenerService(
	repo repositories.URLRepository,
	cache cache.URLCache,
	logger *zap.Logger,
	baseURL string,
	cacheTTL time.Duration,
	codeLength int,
	flushInterval time.Duration,
	flushBatchSize int64,
) *ShortenerService {
	return &ShortenerService{
		repo:           repo,
		cache:          cache,
		logger:         logger,
		baseURL:        strings.TrimRight(baseURL, "/"),
		cacheTTL:       cacheTTL,
		codeLength:     codeLength,
		flushInterval:  flushInterval,
		flushBatchSize: flushBatchSize,
	}
}

func (s *ShortenerService) ShortenURL(ctx context.Context, rawURL string) (*models.ShortenResponse, error) {
	rawURL = strings.TrimSpace(rawURL)
	if err := utils.ValidateURL(rawURL); err != nil {
		return nil, ErrInvalidURL
	}

	existing, err := s.repo.GetByOriginalURL(ctx, rawURL)
	if err == nil {
		s.cacheURL(ctx, *existing)
		return s.buildShortenResponse(existing.ShortCode), nil
	}
	if err != nil && !errors.Is(err, repositories.ErrURLNotFound) {
		return nil, fmt.Errorf("lookup original url: %w", err)
	}

	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		code, err := utils.GenerateBase62(s.codeLength)
		if err != nil {
			return nil, fmt.Errorf("generate short code: %w", err)
		}

		item := &models.URL{
			ID:          uuid.New(),
			OriginalURL: rawURL,
			ShortCode:   code,
		}

		err = s.repo.CreateURL(ctx, item)
		if err == nil {
			s.cacheURL(ctx, *item)
			return s.buildShortenResponse(code), nil
		}

		switch {
		case errors.Is(err, repositories.ErrShortCodeConflict):
			continue
		case errors.Is(err, repositories.ErrOriginalURLConflict):
			again, againErr := s.repo.GetByOriginalURL(ctx, rawURL)
			if againErr != nil {
				return nil, fmt.Errorf("lookup existing original url: %w", againErr)
			}
			s.cacheURL(ctx, *again)
			return s.buildShortenResponse(again.ShortCode), nil
		default:
			return nil, fmt.Errorf("create url: %w", err)
		}
	}

	return nil, fmt.Errorf("failed to generate unique short code after retries")
}

func (s *ShortenerService) ResolveAndTrack(ctx context.Context, shortCode string) (string, error) {
	shortCode = strings.TrimSpace(shortCode)
	if shortCode == "" {
		return "", ErrShortCodeNotFound
	}

	item, err := s.getURLByCode(ctx, shortCode)
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	if err := s.cache.TrackClick(ctx, item.ID, now); err != nil {
		s.logger.Warn("cache track click failed, falling back to db", zap.Error(err), zap.String("short_code", shortCode))
		fallbackErr := s.repo.BulkInsertClicks(ctx, []models.ClickEvent{{
			URLID:      item.ID,
			AccessedAt: now,
		}})
		if fallbackErr != nil {
			s.logger.Error("db fallback click insert failed", zap.Error(fallbackErr), zap.String("short_code", shortCode))
		}
	}

	return item.OriginalURL, nil
}

func (s *ShortenerService) GetStats(ctx context.Context, shortCode string) (*models.StatsResponse, error) {
	shortCode = strings.TrimSpace(shortCode)
	if shortCode == "" {
		return nil, ErrShortCodeNotFound
	}

	stats, err := s.repo.GetStats(ctx, shortCode)
	if err != nil {
		if errors.Is(err, repositories.ErrURLNotFound) {
			return nil, ErrShortCodeNotFound
		}
		return nil, fmt.Errorf("get stats: %w", err)
	}

	pending, err := s.cache.GetPendingClicks(ctx, stats.URL.ID)
	if err != nil {
		s.logger.Warn("get pending clicks failed", zap.Error(err), zap.String("short_code", shortCode))
	}

	lastAccess := stats.LastAccessedAt
	cachedLastAccess, err := s.cache.GetLastAccess(ctx, stats.URL.ID)
	if err != nil {
		s.logger.Warn("get cached last access failed", zap.Error(err), zap.String("short_code", shortCode))
	} else if cachedLastAccess != nil {
		if lastAccess == nil || cachedLastAccess.After(*lastAccess) {
			lastAccess = cachedLastAccess
		}
	}

	var lastAccessString *string
	if lastAccess != nil {
		formatted := lastAccess.UTC().Format(time.RFC3339)
		lastAccessString = &formatted
	}

	return &models.StatsResponse{
		URL:            stats.URL.OriginalURL,
		TotalClicks:    stats.TotalClicks + pending,
		CreatedAt:      stats.URL.CreatedAt.UTC().Format(time.RFC3339),
		LastAccessedAt: lastAccessString,
	}, nil
}

func (s *ShortenerService) RunClickSync(ctx context.Context) {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.flushBatch(ctx); err != nil {
				s.logger.Error("failed to flush click events", zap.Error(err))
			}
		}
	}
}

func (s *ShortenerService) FlushAllPendingClicks(ctx context.Context) error {
	for {
		flushed, err := s.flushBatch(ctx)
		if err != nil {
			return err
		}
		if flushed == 0 {
			return nil
		}
	}
}

func (s *ShortenerService) flushBatch(ctx context.Context) (int, error) {
	events, err := s.cache.DequeueClickEvents(ctx, s.flushBatchSize)
	if err != nil {
		return 0, fmt.Errorf("dequeue click events: %w", err)
	}
	if len(events) == 0 {
		return 0, nil
	}

	if err := s.repo.BulkInsertClicks(ctx, events); err != nil {
		if requeueErr := s.cache.RequeueClickEvents(ctx, events); requeueErr != nil {
			s.logger.Error("failed to requeue click events", zap.Error(requeueErr), zap.Int("count", len(events)))
		}
		return 0, fmt.Errorf("insert click events: %w", err)
	}

	byURL := make(map[uuid.UUID]int64, len(events))
	for _, event := range events {
		byURL[event.URLID]++
	}
	for urlID, count := range byURL {
		if err := s.cache.DecrementPendingClicks(ctx, urlID, count); err != nil {
			s.logger.Warn("failed to decrement pending clicks", zap.Error(err), zap.String("url_id", urlID.String()))
		}
	}

	return len(events), nil
}

func (s *ShortenerService) getURLByCode(ctx context.Context, shortCode string) (*models.URL, error) {
	cached, err := s.cache.GetURL(ctx, shortCode)
	if err != nil {
		s.logger.Warn("cache get url failed", zap.Error(err), zap.String("short_code", shortCode))
	}
	if cached != nil {
		return &models.URL{
			ID:          cached.ID,
			OriginalURL: cached.OriginalURL,
			ShortCode:   shortCode,
		}, nil
	}

	item, err := s.repo.GetByCode(ctx, shortCode)
	if err != nil {
		if errors.Is(err, repositories.ErrURLNotFound) {
			return nil, ErrShortCodeNotFound
		}
		return nil, fmt.Errorf("get by code: %w", err)
	}

	s.cacheURL(ctx, *item)
	return item, nil
}

func (s *ShortenerService) cacheURL(ctx context.Context, item models.URL) {
	err := s.cache.SetURL(ctx, item.ShortCode, models.CachedURL{
		ID:          item.ID,
		OriginalURL: item.OriginalURL,
	}, s.cacheTTL)
	if err != nil {
		s.logger.Warn("cache set url failed", zap.Error(err), zap.String("short_code", item.ShortCode))
	}
}

func (s *ShortenerService) buildShortenResponse(shortCode string) *models.ShortenResponse {
	return &models.ShortenResponse{
		ShortCode: shortCode,
		ShortURL:  fmt.Sprintf("%s/%s", s.baseURL, shortCode),
	}
}
