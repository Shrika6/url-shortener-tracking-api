package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/shrika/url-shortener-tracking-api/internal/middleware"
	"github.com/shrika/url-shortener-tracking-api/internal/models"
	"github.com/shrika/url-shortener-tracking-api/internal/services"
	"go.uber.org/zap"
)

type Handler struct {
	service services.URLService
	logger  *zap.Logger
}

func New(service services.URLService, logger *zap.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) Router(rateLimiter *middleware.RateLimiter) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.Logging(h.logger))
	r.Use(rateLimiter.Handler)

	r.Get("/health", h.Health)
	r.Post("/shorten", h.Shorten)
	r.Get("/stats/{short_code}", h.Stats)
	r.Get("/{short_code}", h.Redirect)

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

	resp, err := h.service.ShortenURL(r.Context(), req.URL, req.CustomCode)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidURL):
			writeError(w, http.StatusBadRequest, "invalid url")
		case errors.Is(err, services.ErrInvalidCustomCode):
			writeError(w, http.StatusBadRequest, "invalid custom_code")
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
	shortCode := chi.URLParam(r, "short_code")
	targetURL, err := h.service.ResolveAndTrack(r.Context(), shortCode)
	if err != nil {
		switch {
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
	resp, err := h.service.GetStats(r.Context(), shortCode)
	if err != nil {
		switch {
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
