package models

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
	URL            string  `json:"url"`
	TotalClicks    int64   `json:"total_clicks"`
	CreatedAt      string  `json:"created_at"`
	LastAccessedAt *string `json:"last_accessed_at,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
