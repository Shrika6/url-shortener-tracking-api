CREATE INDEX IF NOT EXISTS idx_clicks_url_id_accessed_at
ON clicks(url_id, accessed_at);
