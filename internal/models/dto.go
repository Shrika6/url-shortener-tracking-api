package models

import "time"

type ShortenRequest struct {
	URL              string `json:"url"`
	CustomCode       string `json:"custom_code,omitempty"`
	ExpiresInSeconds *int64 `json:"expires_in_seconds,omitempty"`
}

type ShortenResponse struct {
	ShortCode string `json:"short_code"`
	ShortURL  string `json:"short_url"`
}

type StatsResponse struct {
	URL         string      `json:"url"`
	TotalClicks int64       `json:"total_clicks"`
	Clicks      []ClickItem `json:"clicks"`
	Page        int         `json:"page"`
	TotalPages  int         `json:"total_pages"`
}

type ClickItem struct {
	Timestamp string `json:"timestamp"`
}

type StatsQuery struct {
	Page  int
	Limit int
	From  *time.Time
	To    *time.Time
}

type ErrorResponse struct {
	Error string `json:"error"`
}
