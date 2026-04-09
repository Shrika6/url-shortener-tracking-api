package repositories

import (
	"context"
	"errors"

	"github.com/shrika/url-shortener-tracking-api/internal/models"
)

var (
	ErrURLNotFound         = errors.New("url not found")
	ErrShortCodeConflict   = errors.New("short code conflict")
	ErrOriginalURLConflict = errors.New("original url conflict")
)

type URLRepository interface {
	CreateURL(ctx context.Context, url *models.URL) error
	GetByCode(ctx context.Context, shortCode string) (*models.URL, error)
	GetByOriginalURL(ctx context.Context, originalURL string) (*models.URL, error)
	GetStats(ctx context.Context, shortCode string) (*models.URLStats, error)
	BulkInsertClicks(ctx context.Context, events []models.ClickEvent) error
}
