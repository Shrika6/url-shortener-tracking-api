package repositories

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	appmetrics "github.com/shrika/url-shortener-tracking-api/internal/metrics"
	"github.com/shrika/url-shortener-tracking-api/internal/models"
)

type PostgresURLRepository struct {
	db      *pgxpool.Pool
	metrics *appmetrics.Metrics
}

func NewPostgresURLRepository(db *pgxpool.Pool, metrics *appmetrics.Metrics) *PostgresURLRepository {
	return &PostgresURLRepository{db: db, metrics: metrics}
}

func (r *PostgresURLRepository) CreateURL(ctx context.Context, url *models.URL) error {
	start := time.Now()
	defer r.metrics.ObserveDBQuery("create_url", time.Since(start))

	const query = `
		INSERT INTO urls (id, original_url, short_code, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, expires_at
	`

	var expiresAt pgtype.Timestamptz
	err := r.db.QueryRow(ctx, query, url.ID, url.OriginalURL, url.ShortCode, url.ExpiresAt).Scan(&url.CreatedAt, &expiresAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "urls_short_code_key":
				return ErrShortCodeConflict
			case "urls_original_url_key":
				return ErrOriginalURLConflict
			}
			if strings.Contains(pgErr.Detail, "(original_url)") {
				return ErrOriginalURLConflict
			}
			if pgErr.ConstraintName == "" || strings.Contains(pgErr.Detail, "(short_code)") {
				return ErrShortCodeConflict
			}
		}
		return err
	}

	url.CreatedAt = url.CreatedAt.UTC()
	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		url.ExpiresAt = &t
	}
	return nil
}

func (r *PostgresURLRepository) GetByCode(ctx context.Context, shortCode string) (*models.URL, error) {
	start := time.Now()
	defer r.metrics.ObserveDBQuery("get_by_code", time.Since(start))

	const query = `
		SELECT id, original_url, short_code, created_at, expires_at
		FROM urls
		WHERE short_code = $1
	`

	item := &models.URL{}
	var expiresAt pgtype.Timestamptz
	err := r.db.QueryRow(ctx, query, shortCode).Scan(
		&item.ID,
		&item.OriginalURL,
		&item.ShortCode,
		&item.CreatedAt,
		&expiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrURLNotFound
		}
		return nil, err
	}

	item.CreatedAt = item.CreatedAt.UTC()
	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		item.ExpiresAt = &t
	}
	return item, nil
}

func (r *PostgresURLRepository) GetByOriginalURL(ctx context.Context, originalURL string) (*models.URL, error) {
	start := time.Now()
	defer r.metrics.ObserveDBQuery("get_by_original_url", time.Since(start))

	const query = `
		SELECT id, original_url, short_code, created_at, expires_at
		FROM urls
		WHERE original_url = $1
	`

	item := &models.URL{}
	var expiresAt pgtype.Timestamptz
	err := r.db.QueryRow(ctx, query, originalURL).Scan(
		&item.ID,
		&item.OriginalURL,
		&item.ShortCode,
		&item.CreatedAt,
		&expiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrURLNotFound
		}
		return nil, err
	}

	item.CreatedAt = item.CreatedAt.UTC()
	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		item.ExpiresAt = &t
	}
	return item, nil
}

func (r *PostgresURLRepository) GetStats(ctx context.Context, shortCode string, from, to *time.Time, limit, offset int) (*models.URLStats, error) {
	start := time.Now()
	defer r.metrics.ObserveDBQuery("get_stats", time.Since(start))

	const urlQuery = `
		SELECT id, original_url, short_code, created_at, expires_at
		FROM urls
		WHERE short_code = $1
	`
	const countQuery = `
		SELECT COALESCE(COUNT(id), 0), MAX(accessed_at)
		FROM clicks
		WHERE url_id = $1
		  AND ($2::timestamptz IS NULL OR accessed_at >= $2)
		  AND ($3::timestamptz IS NULL OR accessed_at <= $3)
	`
	const clicksQuery = `
		SELECT accessed_at
		FROM clicks
		WHERE url_id = $1
		  AND ($2::timestamptz IS NULL OR accessed_at >= $2)
		  AND ($3::timestamptz IS NULL OR accessed_at <= $3)
		ORDER BY accessed_at DESC
		LIMIT $4 OFFSET $5
	`

	var stats models.URLStats
	var expiresAt pgtype.Timestamptz
	err := r.db.QueryRow(ctx, urlQuery, shortCode).Scan(
		&stats.URL.ID,
		&stats.URL.OriginalURL,
		&stats.URL.ShortCode,
		&stats.URL.CreatedAt,
		&expiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrURLNotFound
		}
		return nil, err
	}
	stats.URL.CreatedAt = stats.URL.CreatedAt.UTC()
	if expiresAt.Valid {
		t := expiresAt.Time.UTC()
		stats.URL.ExpiresAt = &t
	}

	var lastAccess pgtype.Timestamptz
	err = r.db.QueryRow(ctx, countQuery, stats.URL.ID, from, to).Scan(&stats.TotalClicks, &lastAccess)
	if err != nil {
		return nil, err
	}
	if lastAccess.Valid {
		t := lastAccess.Time.UTC()
		stats.LastAccessedAt = &t
	}

	rows, err := r.db.Query(ctx, clicksQuery, stats.URL.ID, from, to, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats.Clicks = make([]time.Time, 0, limit)
	for rows.Next() {
		var ts time.Time
		if err := rows.Scan(&ts); err != nil {
			return nil, err
		}
		stats.Clicks = append(stats.Clicks, ts.UTC())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (r *PostgresURLRepository) BulkInsertClicks(ctx context.Context, events []models.ClickEvent) error {
	start := time.Now()
	defer r.metrics.ObserveDBQuery("bulk_insert_clicks", time.Since(start))

	if len(events) == 0 {
		return nil
	}

	rows := make([][]interface{}, 0, len(events))
	for _, event := range events {
		rows = append(rows, []interface{}{
			uuid.New(),
			event.URLID,
			event.AccessedAt.UTC(),
		})
	}

	_, err := r.db.CopyFrom(
		ctx,
		pgx.Identifier{"clicks"},
		[]string{"id", "url_id", "accessed_at"},
		pgx.CopyFromRows(rows),
	)
	return err
}
