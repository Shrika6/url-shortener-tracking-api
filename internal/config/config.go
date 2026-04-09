package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port string

	DBURL    string
	RedisURL string
	BaseURL  string

	CacheTTL           time.Duration
	CodeLength         int
	RateLimitRequests  int64
	RateLimitWindow    time.Duration
	ClickFlushInterval time.Duration
	ClickFlushBatchSize int64

	HTTPReadTimeout  time.Duration
	HTTPWriteTimeout time.Duration
	HTTPIdleTimeout  time.Duration
	ShutdownTimeout  time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		DBURL:              os.Getenv("DB_URL"),
		RedisURL:           os.Getenv("REDIS_URL"),
		BaseURL:            strings.TrimRight(getEnv("BASE_URL", "http://localhost:8080"), "/"),
		CacheTTL:           getDurationEnv("CACHE_TTL", 24*time.Hour),
		CodeLength:         getIntEnv("CODE_LENGTH", 6),
		RateLimitRequests:  getInt64Env("RATE_LIMIT_REQUESTS", 120),
		RateLimitWindow:    getDurationEnv("RATE_LIMIT_WINDOW", time.Minute),
		ClickFlushInterval: getDurationEnv("CLICK_FLUSH_INTERVAL", 5*time.Second),
		ClickFlushBatchSize: getInt64Env("CLICK_FLUSH_BATCH_SIZE", 500),
		HTTPReadTimeout:    getDurationEnv("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:   getDurationEnv("HTTP_WRITE_TIMEOUT", 10*time.Second),
		HTTPIdleTimeout:    getDurationEnv("HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:    getDurationEnv("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	if cfg.DBURL == "" {
		return nil, fmt.Errorf("DB_URL is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}
	if cfg.CodeLength < 4 {
		return nil, fmt.Errorf("CODE_LENGTH must be >= 4")
	}
	if cfg.RateLimitRequests < 1 {
		return nil, fmt.Errorf("RATE_LIMIT_REQUESTS must be >= 1")
	}
	if cfg.ClickFlushBatchSize < 1 {
		return nil, fmt.Errorf("CLICK_FLUSH_BATCH_SIZE must be >= 1")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

func getIntEnv(key string, fallback int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return fallback
	}
	return parsed
}

func getInt64Env(key string, fallback int64) int64 {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(val)
	if err != nil {
		return fallback
	}
	return parsed
}
