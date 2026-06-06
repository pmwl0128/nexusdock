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

	"github.com/uvwt/agentdock-nexus/internal/audit"
	"github.com/uvwt/agentdock-nexus/internal/auth"
	"github.com/uvwt/agentdock-nexus/internal/core"
	"github.com/uvwt/agentdock-nexus/internal/runs"
)

func main() {
	if err := runServer(); err != nil {
		slog.Error("nexus server stopped", "error", err)
		os.Exit(1)
	}
}

func runServer() error {
	cfg := core.ConfigFromEnv()
	if err := cfg.Validate(); err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()}))
	slog.SetDefault(logger)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	db, err := core.OpenSQLite(ctx, cfg.DatabasePath, cfg.MaxOpenConns)
	if err != nil {
		return err
	}
	defer db.Close()
	migrations := core.NewMigrationRunner(db, core.SQLiteBackupHook{SourcePath: cfg.DatabasePath, Directory: cfg.BackupDir})
	if err := migrations.Run(ctx); err != nil {
		return err
	}
	events := core.NewEventBus()
	defer events.Close()
	authService := auth.NewService(db)
	if err := authService.EnsureBootstrapSystemToken(ctx, os.Getenv("AGENTDOCK_NEXUS_BOOTSTRAP_TOKEN")); err != nil {
		return err
	}
	application := newApp(db, migrations, authService, audit.NewService(db), runs.NewService(db, events), logger)
	server := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           application.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("nexus server listening", "addr", cfg.Addr())
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
