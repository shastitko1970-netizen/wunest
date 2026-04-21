// WuNest — Web client for LLM roleplay, WuSphere ecosystem.
//
// This binary boots an HTTP server that:
//   - Serves the embedded Vue SPA at /
//   - Exposes /api/* for characters, chats, personas, worlds
//   - Reads the WuApi `wu_session` cookie from `.wusphere.ru` to authenticate users
//   - Proxies /api/chats/:id/stream to WuApi /v1/chat/completions (SSE pass-through)
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shastitko1970-netizen/wunest/internal/config"
	"github.com/shastitko1970-netizen/wunest/internal/db"
	"github.com/shastitko1970-netizen/wunest/internal/server"
	"github.com/shastitko1970-netizen/wunest/internal/wuapi"
)

func main() {
	// --- Config ---
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	// --- Logging ---
	logger := newLogger(cfg)
	slog.SetDefault(logger)
	slog.Info("WuNest starting",
		"env", cfg.Env,
		"port", cfg.HTTPPort,
		"wuapi_base", cfg.WuApiBaseURL,
	)

	// --- Database & cache ---
	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pg, err := db.NewPostgres(rootCtx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("postgres connect failed", "err", err)
		os.Exit(1)
	}
	defer pg.Close()

	rdb, err := db.NewRedis(rootCtx, cfg.RedisURL)
	if err != nil {
		slog.Error("redis connect failed", "err", err)
		os.Exit(1)
	}
	defer rdb.Close()

	// --- WuApi client ---
	wuClient := wuapi.NewClient(wuapi.Config{
		BaseURL:     cfg.WuApiBaseURL,
		InternalURL: cfg.WuApiInternalURL,
		Timeout:     30 * time.Second,
	})

	// --- HTTP server ---
	srv := server.New(server.Deps{
		Config:   cfg,
		Postgres: pg,
		Redis:    rdb,
		WuApi:    wuClient,
		Logger:   logger,
	})

	httpServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout — streaming (SSE) needs long-running writes.
		IdleTimeout: 90 * time.Second,
	}

	// --- Start & wait for signal ---
	go func() {
		slog.Info("http server listening", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "err", err)
			cancel()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	select {
	case s := <-sig:
		slog.Info("signal received, shutting down", "signal", s.String())
	case <-rootCtx.Done():
		slog.Info("context cancelled, shutting down")
	}

	// --- Graceful shutdown ---
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http shutdown failed", "err", err)
	}
	slog.Info("bye")
}

func newLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	if cfg.Env == "development" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}
