package models

import (
	"time"

	"github.com/google/uuid"
)

type URL struct {
	ID          uuid.UUID `json:"id"`
	OriginalURL string    `json:"original_url"`
	ShortCode   string    `json:"short_code"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

type ClickEvent struct {
	URLID      uuid.UUID `json:"url_id"`
	AccessedAt time.Time `json:"accessed_at"`
}

type URLStats struct {
	URL            URL        `json:"url"`
	TotalClicks    int64      `json:"total_clicks"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
}
