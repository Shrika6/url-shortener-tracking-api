package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/shrika/url-shortener-tracking-api/internal/models"
	"go.uber.org/zap"
)

type RateLimiter struct {
	redis  *redis.Client
	limit  int64
	window time.Duration
	logger *zap.Logger
}

func NewRateLimiter(redis *redis.Client, limit int64, window time.Duration, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		redis:  redis,
		limit:  limit,
		window: window,
		logger: logger,
	}
}

func (rl *RateLimiter) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r)
		windowSeconds := int64(rl.window.Seconds())
		if windowSeconds <= 0 {
			windowSeconds = 60
		}
		windowID := time.Now().UTC().Unix() / windowSeconds
		key := "shortener:rate_limit:" + ip + ":" + strconv.FormatInt(windowID, 10)

		count, err := rl.redis.Incr(r.Context(), key).Result()
		if err != nil {
			rl.logger.Warn("rate limit redis failure, allowing request", zap.Error(err), zap.String("ip", ip))
			next.ServeHTTP(w, r)
			return
		}

		if count == 1 {
			_ = rl.redis.Expire(r.Context(), key, rl.window).Err()
		}

		if count > rl.limit {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(models.ErrorResponse{Error: "rate limit exceeded"})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func extractClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if realIP != "" {
		return realIP
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) == "" {
		return "unknown"
	}
	return strings.TrimSpace(r.RemoteAddr)
}
