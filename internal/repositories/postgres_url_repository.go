package repositories

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shrika/url-shortener-tracking-api/internal/models"
)

type PostgresURLRepository struct {
	db *pgxpool.Pool
}

func NewPostgresURLRepository(db *pgxpool.Pool) *PostgresURLRepository {
	return &PostgresURLRepository{db: db}
}

func (r *PostgresURLRepository) CreateURL(ctx context.Context, url *models.URL) error {
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

func (r *PostgresURLRepository) GetStats(ctx context.Context, shortCode string) (*models.URLStats, error) {
	const query = `
		SELECT
			u.id,
			u.original_url,
			u.short_code,
			u.created_at,
			u.expires_at,
			COALESCE(COUNT(c.id), 0) AS total_clicks,
			MAX(c.accessed_at) AS last_accessed_at
		FROM urls u
		LEFT JOIN clicks c ON c.url_id = u.id
		WHERE u.short_code = $1
		GROUP BY u.id
	`

	var stats models.URLStats
	var expiresAt pgtype.Timestamptz
	var lastAccess pgtype.Timestamptz

	err := r.db.QueryRow(ctx, query, shortCode).Scan(
		&stats.URL.ID,
		&stats.URL.OriginalURL,
		&stats.URL.ShortCode,
		&stats.URL.CreatedAt,
		&expiresAt,
		&stats.TotalClicks,
		&lastAccess,
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
	if lastAccess.Valid {
		t := lastAccess.Time.UTC()
		stats.LastAccessedAt = &t
	}

	return &stats, nil
}

func (r *PostgresURLRepository) BulkInsertClicks(ctx context.Context, events []models.ClickEvent) error {
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
