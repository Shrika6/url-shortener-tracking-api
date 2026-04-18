package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/shrika/url-shortener-tracking-api/internal/middleware"
	appmetrics "github.com/shrika/url-shortener-tracking-api/internal/metrics"
	"github.com/shrika/url-shortener-tracking-api/internal/models"
	"github.com/shrika/url-shortener-tracking-api/internal/services"
	"go.uber.org/zap"
)

type Handler struct {
	service services.URLService
	logger  *zap.Logger
	metrics *appmetrics.Metrics
}

func New(service services.URLService, logger *zap.Logger, metrics *appmetrics.Metrics) *Handler {
	return &Handler{service: service, logger: logger, metrics: metrics}
}

func (h *Handler) Router(rateLimiter *middleware.RateLimiter) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.Logging(h.logger))
	r.Use(middleware.HTTPMetrics(h.metrics))

	r.Get("/health", h.Health)
	r.Handle("/metrics", promhttp.Handler())
	r.Get("/stats/{short_code}", h.Stats)
	r.With(rateLimiter.Handler).Post("/shorten", h.Shorten)
	r.With(rateLimiter.Handler).Get("/{short_code}", h.Redirect)

	return r
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Shorten(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req models.ShortenRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	resp, err := h.service.ShortenURL(r.Context(), req.URL, req.CustomCode, req.ExpiresInSeconds)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidURL):
			writeError(w, http.StatusBadRequest, "invalid url")
		case errors.Is(err, services.ErrInvalidCustomCode):
			writeError(w, http.StatusBadRequest, "invalid custom_code")
		case errors.Is(err, services.ErrInvalidExpiry):
			writeError(w, http.StatusBadRequest, "invalid expires_in_seconds")
		case errors.Is(err, services.ErrReservedShortCode):
			writeError(w, http.StatusBadRequest, "custom_code is reserved")
		case errors.Is(err, services.ErrCustomCodeConflict):
			writeError(w, http.StatusConflict, "custom_code already exists")
		default:
			h.logger.Error("failed to shorten url", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	h.metrics.IncRedirectRequests()
	defer h.metrics.ObserveRedirectLatency(time.Since(start))

	shortCode := chi.URLParam(r, "short_code")
	targetURL, err := h.service.ResolveAndTrack(r.Context(), shortCode)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrShortCodeExpired):
			writeError(w, http.StatusGone, "short code expired")
		case errors.Is(err, services.ErrShortCodeNotFound):
			writeError(w, http.StatusNotFound, "short code not found")
		default:
			h.logger.Error("failed to resolve short code", zap.Error(err), zap.String("short_code", shortCode))
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	http.Redirect(w, r, targetURL, http.StatusFound)
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	shortCode := chi.URLParam(r, "short_code")
	query, err := parseStatsQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid stats query params")
		return
	}

	resp, err := h.service.GetStats(r.Context(), shortCode, query)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidStatsQuery):
			writeError(w, http.StatusBadRequest, "invalid stats query params")
		case errors.Is(err, services.ErrShortCodeNotFound):
			writeError(w, http.StatusNotFound, "short code not found")
		default:
			h.logger.Error("failed to fetch stats", zap.Error(err), zap.String("short_code", shortCode))
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, models.ErrorResponse{Error: message})
}

func parseStatsQuery(r *http.Request) (models.StatsQuery, error) {
	q := r.URL.Query()
	page := 1
	limit := 10

	if rawPage := strings.TrimSpace(q.Get("page")); rawPage != "" {
		parsed, err := strconv.Atoi(rawPage)
		if err != nil || parsed < 1 {
			return models.StatsQuery{}, errors.New("invalid page")
		}
		page = parsed
	}
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			return models.StatsQuery{}, errors.New("invalid limit")
		}
		limit = parsed
	}

	var fromPtr *time.Time
	if rawFrom := strings.TrimSpace(q.Get("from")); rawFrom != "" {
		from, err := time.Parse(time.RFC3339, rawFrom)
		if err != nil {
			return models.StatsQuery{}, errors.New("invalid from")
		}
		from = from.UTC()
		fromPtr = &from
	}

	var toPtr *time.Time
	if rawTo := strings.TrimSpace(q.Get("to")); rawTo != "" {
		to, err := time.Parse(time.RFC3339, rawTo)
		if err != nil {
			return models.StatsQuery{}, errors.New("invalid to")
		}
		to = to.UTC()
		toPtr = &to
	}

	if fromPtr != nil && toPtr != nil && fromPtr.After(*toPtr) {
		return models.StatsQuery{}, errors.New("from after to")
	}

	return models.StatsQuery{
		Page:  page,
		Limit: limit,
		From:  fromPtr,
		To:    toPtr,
	}, nil
}
