package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shrika/url-shortener-tracking-api/internal/cache"
	"github.com/shrika/url-shortener-tracking-api/internal/config"
	"github.com/shrika/url-shortener-tracking-api/internal/handlers"
	"github.com/shrika/url-shortener-tracking-api/internal/logger"
	"github.com/shrika/url-shortener-tracking-api/internal/middleware"
	"github.com/shrika/url-shortener-tracking-api/internal/repositories"
	"github.com/shrika/url-shortener-tracking-api/internal/services"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	log, err := logger.New()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = log.Sync()
	}()

	ctx := context.Background()

	dbPool, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		log.Fatal("failed to connect postgres", zap.Error(err))
	}
	defer dbPool.Close()
	if err := dbPool.Ping(ctx); err != nil {
		log.Fatal("failed to ping postgres", zap.Error(err))
	}

	redisClient, err := cache.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatal("failed to create redis client", zap.Error(err))
	}
	defer func() {
		_ = redisClient.Close()
	}()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatal("failed to ping redis", zap.Error(err))
	}

	repo := repositories.NewPostgresURLRepository(dbPool)
	redisCache := cache.NewRedisCache(redisClient)
	service := services.NewShortenerService(
		repo,
		redisCache,
		log,
		cfg.BaseURL,
		cfg.CacheTTL,
		cfg.CodeLength,
		cfg.ClickFlushInterval,
		cfg.ClickFlushBatchSize,
	)

	rateLimiter := middleware.NewRateLimiter(redisClient, cfg.RateLimitRequests, cfg.RateLimitWindow, log)
	h := handlers.New(service, log)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      h.Router(rateLimiter),
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}

	syncCtx, cancelSync := context.WithCancel(context.Background())
	defer cancelSync()
	go service.RunClickSync(syncCtx)

	serverErrCh := make(chan error, 1)
	go func() {
		log.Info("http server starting", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Info("shutdown signal received", zap.String("signal", sig.String()))
	case err := <-serverErrCh:
		log.Error("server crashed", zap.Error(err))
	}

	cancelSync()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := service.FlushAllPendingClicks(shutdownCtx); err != nil {
		log.Warn("failed to flush pending clicks", zap.Error(err))
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", zap.Error(err))
	}

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelWait()
	for {
		if waitCtx.Err() != nil {
			break
		}
		if dbPool.Stat().AcquiredConns() == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	log.Info("shutdown complete")
}
