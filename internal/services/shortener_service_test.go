package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shrika/url-shortener-tracking-api/internal/models"
	"github.com/shrika/url-shortener-tracking-api/internal/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type repoMock struct {
	createURLFn      func(ctx context.Context, url *models.URL) error
	getByCodeFn      func(ctx context.Context, shortCode string) (*models.URL, error)
	getByOriginalFn  func(ctx context.Context, originalURL string) (*models.URL, error)
	getStatsFn       func(ctx context.Context, shortCode string, from, to *time.Time, limit, offset int) (*models.URLStats, error)
	bulkInsertFn     func(ctx context.Context, events []models.ClickEvent) error
}

func (m *repoMock) CreateURL(ctx context.Context, url *models.URL) error {
	if m.createURLFn != nil {
		return m.createURLFn(ctx, url)
	}
	return nil
}

func (m *repoMock) GetByCode(ctx context.Context, shortCode string) (*models.URL, error) {
	if m.getByCodeFn != nil {
		return m.getByCodeFn(ctx, shortCode)
	}
	return nil, repositories.ErrURLNotFound
}

func (m *repoMock) GetByOriginalURL(ctx context.Context, originalURL string) (*models.URL, error) {
	if m.getByOriginalFn != nil {
		return m.getByOriginalFn(ctx, originalURL)
	}
	return nil, repositories.ErrURLNotFound
}

func (m *repoMock) GetStats(ctx context.Context, shortCode string, from, to *time.Time, limit, offset int) (*models.URLStats, error) {
	if m.getStatsFn != nil {
		return m.getStatsFn(ctx, shortCode, from, to, limit, offset)
	}
	return nil, repositories.ErrURLNotFound
}

func (m *repoMock) BulkInsertClicks(ctx context.Context, events []models.ClickEvent) error {
	if m.bulkInsertFn != nil {
		return m.bulkInsertFn(ctx, events)
	}
	return nil
}

type cacheMock struct {
	getURLFn            func(ctx context.Context, shortCode string) (*models.CachedURL, error)
	setURLFn            func(ctx context.Context, shortCode string, value models.CachedURL, ttl time.Duration) error
	trackClickFn        func(ctx context.Context, urlID uuid.UUID, accessedAt time.Time) error
	dequeueFn           func(ctx context.Context, batchSize int64) ([]models.ClickEvent, error)
	requeueFn           func(ctx context.Context, events []models.ClickEvent) error
	getPendingClicksFn  func(ctx context.Context, urlID uuid.UUID) (int64, error)
	decrementFn         func(ctx context.Context, urlID uuid.UUID, amount int64) error
	getLastAccessFn     func(ctx context.Context, urlID uuid.UUID) (*time.Time, error)
}

func (m *cacheMock) GetURL(ctx context.Context, shortCode string) (*models.CachedURL, error) {
	if m.getURLFn != nil {
		return m.getURLFn(ctx, shortCode)
	}
	return nil, nil
}

func (m *cacheMock) SetURL(ctx context.Context, shortCode string, value models.CachedURL, ttl time.Duration) error {
	if m.setURLFn != nil {
		return m.setURLFn(ctx, shortCode, value, ttl)
	}
	return nil
}

func (m *cacheMock) TrackClick(ctx context.Context, urlID uuid.UUID, accessedAt time.Time) error {
	if m.trackClickFn != nil {
		return m.trackClickFn(ctx, urlID, accessedAt)
	}
	return nil
}

func (m *cacheMock) DequeueClickEvents(ctx context.Context, batchSize int64) ([]models.ClickEvent, error) {
	if m.dequeueFn != nil {
		return m.dequeueFn(ctx, batchSize)
	}
	return nil, nil
}

func (m *cacheMock) RequeueClickEvents(ctx context.Context, events []models.ClickEvent) error {
	if m.requeueFn != nil {
		return m.requeueFn(ctx, events)
	}
	return nil
}

func (m *cacheMock) GetPendingClicks(ctx context.Context, urlID uuid.UUID) (int64, error) {
	if m.getPendingClicksFn != nil {
		return m.getPendingClicksFn(ctx, urlID)
	}
	return 0, nil
}

func (m *cacheMock) DecrementPendingClicks(ctx context.Context, urlID uuid.UUID, amount int64) error {
	if m.decrementFn != nil {
		return m.decrementFn(ctx, urlID, amount)
	}
	return nil
}

func (m *cacheMock) GetLastAccess(ctx context.Context, urlID uuid.UUID) (*time.Time, error) {
	if m.getLastAccessFn != nil {
		return m.getLastAccessFn(ctx, urlID)
	}
	return nil, nil
}

func TestShortenURL_InvalidURL(t *testing.T) {
	svc := NewShortenerService(
		&repoMock{},
		&cacheMock{},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	_, err := svc.ShortenURL(context.Background(), "not-a-valid-url", "", nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidURL))
}

func TestShortenURL_ReturnsExistingCodeForDuplicateURL(t *testing.T) {
	existingID := uuid.New()
	svc := NewShortenerService(
		&repoMock{
			getByOriginalFn: func(_ context.Context, originalURL string) (*models.URL, error) {
				require.Equal(t, "https://example.com", originalURL)
				return &models.URL{
					ID:          existingID,
					OriginalURL: originalURL,
					ShortCode:   "abc123",
					CreatedAt:   time.Now().UTC(),
				}, nil
			},
		},
		&cacheMock{},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	resp, err := svc.ShortenURL(context.Background(), "https://example.com", "", nil)
	require.NoError(t, err)
	require.Equal(t, "abc123", resp.ShortCode)
	require.Equal(t, "http://localhost:8080/abc123", resp.ShortURL)
}

func TestShortenURL_CustomCodeValidation(t *testing.T) {
	svc := NewShortenerService(
		&repoMock{},
		&cacheMock{},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	_, err := svc.ShortenURL(context.Background(), "https://example.com", "bad code", nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidCustomCode))

	_, err = svc.ShortenURL(context.Background(), "https://example.com", "health", nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrReservedShortCode))
}

func TestShortenURL_CustomCodeConflict(t *testing.T) {
	svc := NewShortenerService(
		&repoMock{},
		&cacheMock{
			getURLFn: func(_ context.Context, shortCode string) (*models.CachedURL, error) {
				require.Equal(t, "my-link", shortCode)
				return &models.CachedURL{ID: uuid.New(), OriginalURL: "https://already.com"}, nil
			},
		},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	_, err := svc.ShortenURL(context.Background(), "https://example.com", "my-link", nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCustomCodeConflict))
}

func TestShortenURL_CustomCodeConflictFromDB(t *testing.T) {
	svc := NewShortenerService(
		&repoMock{
			getByCodeFn: func(_ context.Context, shortCode string) (*models.URL, error) {
				require.Equal(t, "my-link", shortCode)
				return &models.URL{
					ID:          uuid.New(),
					OriginalURL: "https://existing.com",
					ShortCode:   "my-link",
					CreatedAt:   time.Now().UTC(),
				}, nil
			},
		},
		&cacheMock{},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	_, err := svc.ShortenURL(context.Background(), "https://example.com", "my-link", nil)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCustomCodeConflict))
}

func TestShortenURL_InvalidExpiry(t *testing.T) {
	invalid := int64(0)
	svc := NewShortenerService(
		&repoMock{},
		&cacheMock{},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	_, err := svc.ShortenURL(context.Background(), "https://example.com", "", &invalid)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidExpiry))
}

func TestGetStats_IncludesPendingClicks(t *testing.T) {
	urlID := uuid.New()
	createdAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	svc := NewShortenerService(
		&repoMock{
			getStatsFn: func(_ context.Context, shortCode string, from, to *time.Time, limit, offset int) (*models.URLStats, error) {
				require.Equal(t, "abc123", shortCode)
				require.Nil(t, from)
				require.Nil(t, to)
				require.Equal(t, 10, limit)
				require.Equal(t, 0, offset)
				return &models.URLStats{
					URL: models.URL{
						ID:          urlID,
						OriginalURL: "https://example.com",
						ShortCode:   "abc123",
						CreatedAt:   createdAt,
					},
					TotalClicks: 10,
					Clicks: []time.Time{
						time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC),
					},
				}, nil
			},
		},
		&cacheMock{
			getPendingClicksFn: func(_ context.Context, gotURLID uuid.UUID) (int64, error) {
				require.Equal(t, urlID, gotURLID)
				return 5, nil
			},
		},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	stats, err := svc.GetStats(context.Background(), "abc123", models.StatsQuery{Page: 1, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, int64(15), stats.TotalClicks)
	require.Equal(t, "https://example.com", stats.URL)
	require.Equal(t, 1, stats.Page)
	require.Equal(t, 2, stats.TotalPages)
	require.Len(t, stats.Clicks, 1)
	require.Equal(t, "2026-01-02T10:00:00Z", stats.Clicks[0].Timestamp)
}

func TestGetStats_WithFilters_DoesNotIncludePendingClicks(t *testing.T) {
	urlID := uuid.New()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	pendingCalled := false

	svc := NewShortenerService(
		&repoMock{
			getStatsFn: func(_ context.Context, shortCode string, gotFrom, gotTo *time.Time, limit, offset int) (*models.URLStats, error) {
				require.Equal(t, "abc123", shortCode)
				require.NotNil(t, gotFrom)
				require.NotNil(t, gotTo)
				require.Equal(t, from, gotFrom.UTC())
				require.Equal(t, to, gotTo.UTC())
				require.Equal(t, 5, limit)
				require.Equal(t, 5, offset)
				return &models.URLStats{
					URL: models.URL{
						ID:          urlID,
						OriginalURL: "https://example.com",
						ShortCode:   "abc123",
						CreatedAt:   time.Now().UTC(),
					},
					TotalClicks: 2,
					Clicks: []time.Time{
						time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
					},
				}, nil
			},
		},
		&cacheMock{
			getPendingClicksFn: func(_ context.Context, _ uuid.UUID) (int64, error) {
				pendingCalled = true
				return 50, nil
			},
		},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	stats, err := svc.GetStats(context.Background(), "abc123", models.StatsQuery{
		Page:  2,
		Limit: 5,
		From:  &from,
		To:    &to,
	})
	require.NoError(t, err)
	require.False(t, pendingCalled)
	require.Equal(t, int64(2), stats.TotalClicks)
	require.Equal(t, 2, stats.Page)
	require.Equal(t, 1, stats.TotalPages)
	require.Len(t, stats.Clicks, 1)
}

func TestGetStats_InvalidQuery(t *testing.T) {
	svc := NewShortenerService(
		&repoMock{},
		&cacheMock{},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	_, err := svc.GetStats(context.Background(), "abc123", models.StatsQuery{Page: 0, Limit: 10})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidStatsQuery))
}

func TestResolveAndTrack_NotFound(t *testing.T) {
	svc := NewShortenerService(
		&repoMock{
			getByCodeFn: func(_ context.Context, shortCode string) (*models.URL, error) {
				require.Equal(t, "missing", shortCode)
				return nil, repositories.ErrURLNotFound
			},
		},
		&cacheMock{},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	_, err := svc.ResolveAndTrack(context.Background(), "missing")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrShortCodeNotFound))
}

func TestResolveAndTrack_Expired(t *testing.T) {
	expiredAt := time.Now().UTC().Add(-1 * time.Minute)
	cacheCalled := false

	svc := NewShortenerService(
		&repoMock{},
		&cacheMock{
			getURLFn: func(_ context.Context, shortCode string) (*models.CachedURL, error) {
				require.Equal(t, "dead1", shortCode)
				return &models.CachedURL{
					ID:          uuid.New(),
					OriginalURL: "https://example.com",
					ExpiresAt:   &expiredAt,
				}, nil
			},
			trackClickFn: func(_ context.Context, _ uuid.UUID, _ time.Time) error {
				cacheCalled = true
				return nil
			},
		},
		zap.NewNop(),
		"http://localhost:8080",
		time.Hour,
		6,
		time.Second,
		100,
	)

	_, err := svc.ResolveAndTrack(context.Background(), "dead1")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrShortCodeExpired))
	require.False(t, cacheCalled, "expired links must not increment click tracking")
}
