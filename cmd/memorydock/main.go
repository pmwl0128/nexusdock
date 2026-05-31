package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/uvwt/memorydock/internal/config"
	"github.com/uvwt/memorydock/internal/httpx"
	"github.com/uvwt/memorydock/internal/memory"
	"github.com/uvwt/memorydock/internal/syncer"
)

func main() {
	cfg := config.FromEnv()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel()}))
	slog.SetDefault(logger)

	if err := cfg.ValidateStartup(); err != nil {
		logger.Error("invalid startup configuration", "error", err)
		os.Exit(1)
	}

	store, err := memory.NewStore(cfg.StoreDir)
	if err != nil {
		logger.Error("failed to initialize store", "error", err)
		os.Exit(1)
	}

	syncManager := syncer.NewManager(syncer.Config{
		RepoDir:       cfg.StoreDir,
		AutoSync:      cfg.AutoSync,
		PullInterval:  cfg.PullInterval,
		PushDebounce:  cfg.PushDebounce,
		CommitMessage: cfg.CommitMessage,
	}, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	syncManager.Start(ctx)

	server := httpx.NewServer(cfg, store, syncManager, logger)
	httpServer := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("memorydock starting", "addr", cfg.Addr(), "store_dir", cfg.StoreDir, "auto_sync", cfg.AutoSync)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	logger.Info("memorydock stopped")
}
