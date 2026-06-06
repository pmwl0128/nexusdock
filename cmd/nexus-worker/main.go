package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/uvwt/memorydock/internal/auth"
	"github.com/uvwt/memorydock/internal/core"
)

func main() {
	if err := runWorker(); err != nil {
		slog.Error("nexus worker stopped", "error", err)
		os.Exit(1)
	}
}

func runWorker() error {
	cfg := core.ConfigFromEnv()
	if err := cfg.Validate(); err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()}))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := core.OpenSQLite(ctx, cfg.DatabasePath, cfg.MaxOpenConns)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := core.NewMigrationRunner(db, core.SQLiteBackupHook{SourcePath: cfg.DatabasePath, Directory: cfg.BackupDir}).Run(ctx); err != nil {
		return err
	}
	authService := auth.NewService(db)
	ticker := time.NewTicker(cfg.WorkerInterval)
	defer ticker.Stop()
	logger.Info("nexus worker started", "interval", cfg.WorkerInterval.String())
	for {
		select {
		case <-ctx.Done():
			logger.Info("nexus worker stopped")
			return nil
		case <-ticker.C:
			count, err := authService.CleanupExpired(ctx)
			if err != nil {
				logger.Error("cleanup expired tokens", "error", err)
				continue
			}
			if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
				logger.Error("checkpoint sqlite wal", "error", err)
				continue
			}
			logger.Info("worker maintenance completed", "expired_tokens_deleted", count)
		}
	}
}
