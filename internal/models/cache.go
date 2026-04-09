package models

import "github.com/google/uuid"

type CachedURL struct {
	ID          uuid.UUID `json:"id"`
	OriginalURL string    `json:"original_url"`
}
