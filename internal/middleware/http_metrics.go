package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	appmetrics "github.com/shrika/url-shortener-tracking-api/internal/metrics"
)

func HTTPMetrics(m *appmetrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			endpoint := routePatternOrPath(r)
			m.ObserveHTTPRequest(r.Method, endpoint, strconv.Itoa(rec.status), time.Since(start))
		})
	}
}

func routePatternOrPath(r *http.Request) string {
	if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
		if pattern := routeCtx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return r.URL.Path
}
