package models

import (
	"time"

	"github.com/google/uuid"
)

type CachedURL struct {
	ID          uuid.UUID `json:"id"`
	OriginalURL string    `json:"original_url"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}
