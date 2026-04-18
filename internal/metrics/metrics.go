package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	httpRequestsTotal        *prometheus.CounterVec
	httpRequestDuration      *prometheus.HistogramVec
	redirectRequestsTotal    prometheus.Counter
	redirectLatencySeconds   prometheus.Histogram
	cacheHitsTotal           prometheus.Counter
	cacheMissesTotal         prometheus.Counter
	dbQueryDurationSeconds   *prometheus.HistogramVec
	redisOpDurationSeconds   *prometheus.HistogramVec
}

func New() *Metrics {
	m := &Metrics{
		httpRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status_code"},
		),
		httpRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "endpoint", "status_code"},
		),
		redirectRequestsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "redirect_requests_total",
				Help: "Total number of redirect requests",
			},
		),
		redirectLatencySeconds: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "redirect_latency_seconds",
				Help:    "Latency of redirect request handling in seconds",
				Buckets: prometheus.DefBuckets,
			},
		),
		cacheHitsTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "cache_hits_total",
				Help: "Total number of cache hits",
			},
		),
		cacheMissesTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "cache_misses_total",
				Help: "Total number of cache misses",
			},
		),
		dbQueryDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "db_query_duration_seconds",
				Help:    "Database query duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
		redisOpDurationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "redis_operation_duration_seconds",
				Help:    "Redis operation duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),
	}

	prometheus.MustRegister(
		m.httpRequestsTotal,
		m.httpRequestDuration,
		m.redirectRequestsTotal,
		m.redirectLatencySeconds,
		m.cacheHitsTotal,
		m.cacheMissesTotal,
		m.dbQueryDurationSeconds,
		m.redisOpDurationSeconds,
	)

	return m
}

func (m *Metrics) ObserveHTTPRequest(method, endpoint, statusCode string, duration time.Duration) {
	if m == nil {
		return
	}
	m.httpRequestsTotal.WithLabelValues(method, endpoint, statusCode).Inc()
	m.httpRequestDuration.WithLabelValues(method, endpoint, statusCode).Observe(duration.Seconds())
}

func (m *Metrics) IncRedirectRequests() {
	if m == nil {
		return
	}
	m.redirectRequestsTotal.Inc()
}

func (m *Metrics) ObserveRedirectLatency(duration time.Duration) {
	if m == nil {
		return
	}
	m.redirectLatencySeconds.Observe(duration.Seconds())
}

func (m *Metrics) IncCacheHit() {
	if m == nil {
		return
	}
	m.cacheHitsTotal.Inc()
}

func (m *Metrics) IncCacheMiss() {
	if m == nil {
		return
	}
	m.cacheMissesTotal.Inc()
}

func (m *Metrics) ObserveDBQuery(operation string, duration time.Duration) {
	if m == nil {
		return
	}
	m.dbQueryDurationSeconds.WithLabelValues(operation).Observe(duration.Seconds())
}

func (m *Metrics) ObserveRedisOperation(operation string, duration time.Duration) {
	if m == nil {
		return
	}
	m.redisOpDurationSeconds.WithLabelValues(operation).Observe(duration.Seconds())
}
