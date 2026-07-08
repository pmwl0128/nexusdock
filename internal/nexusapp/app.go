package nexusapp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/uvwt/agentdock-nexus/internal/artifacts"
	"github.com/uvwt/agentdock-nexus/internal/auth"
	"github.com/uvwt/agentdock-nexus/internal/commands"
	"github.com/uvwt/agentdock-nexus/internal/config"
	"github.com/uvwt/agentdock-nexus/internal/core"
	"github.com/uvwt/agentdock-nexus/internal/devices"
	"github.com/uvwt/agentdock-nexus/internal/httpx"
	"github.com/uvwt/agentdock-nexus/internal/recall"
	"github.com/uvwt/agentdock-nexus/internal/syncer"
)

func Main(args []string) {
	cfg := config.FromEnv()
	if adminCommandRequested(args) {
		if err := runAdminCommand(context.Background(), cfg, args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel()}))
	slog.SetDefault(logger)

	if err := cfg.ValidateStartup(); err != nil {
		logger.Error("invalid startup configuration", "error", err)
		os.Exit(1)
	}

	store, err := recall.NewStore(cfg.RecallRepoDir)
	if err != nil {
		logger.Error("failed to initialize store", "error", err)
		os.Exit(1)
	}

	syncManager := syncer.NewManager(syncer.Config{
		RepoDir:       cfg.RecallRepoDir,
		AutoSync:      cfg.AutoSync,
		PullInterval:  cfg.PullInterval,
		PushDebounce:  cfg.PushDebounce,
		CommitMessage: cfg.CommitMessage,
	}, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	syncManager.Start(ctx)

	controlDir := cfg.NexusDataDir
	if err := os.MkdirAll(controlDir, 0o700); err != nil {
		logger.Error("failed to create control plane directory", "error", err)
		os.Exit(1)
	}
	controlDBPath := filepath.Join(controlDir, "nexus.db")
	controlDB, err := core.OpenSQLite(ctx, controlDBPath, 4)
	if err != nil {
		logger.Error("failed to open control plane database", "error", err)
		os.Exit(1)
	}
	defer controlDB.Close()
	migrations := core.NewMigrationRunner(controlDB, core.SQLiteBackupHook{SourcePath: controlDBPath, Directory: filepath.Join(controlDir, "backups")})
	if err := migrations.Run(ctx); err != nil {
		logger.Error("failed to migrate control plane database", "error", err)
		os.Exit(1)
	}

	authService := auth.NewService(controlDB)
	migrated, err := authService.EnsureLegacyAdmin(ctx, cfg.Username, cfg.Password, cfg.PasswordHash)
	if err != nil {
		logger.Error("failed to migrate administrator", "error", err)
		os.Exit(1)
	}
	status, err := authService.AdminStatus(ctx)
	if err != nil {
		logger.Error("failed to read administrator status", "error", err)
		os.Exit(1)
	}
	if !status.Initialized {
		logger.Warn("administrator is not initialized; run the local admin init command")
	}
	if migrated {
		logger.Info("migrated legacy administrator credentials")
	}

	deviceService, err := devices.NewService(devices.NewSQLiteRepository(controlDB))
	if err != nil {
		logger.Error("failed to initialize device control plane", "error", err)
		os.Exit(1)
	}
	commandService, err := commands.NewService(commands.NewSQLiteRepository(controlDB), deviceService)
	if err != nil {
		logger.Error("failed to initialize command control plane", "error", err)
		os.Exit(1)
	}
	artifactService, err := artifacts.NewService(
		artifacts.NewSQLiteRepository(controlDB), deviceService, commandService,
		filepath.Join(controlDir, "artifacts"),
	)
	if err != nil {
		logger.Error("failed to initialize artifact relay", "error", err)
		os.Exit(1)
	}
	go artifactService.RunCleanup(ctx, time.Hour)

	embeddingService := recall.NewEmbeddingService(store, recall.EmbeddingConfig{
		Enabled: cfg.EmbeddingEnabled, Endpoint: cfg.EmbeddingEndpoint, Model: cfg.EmbeddingModel,
		IndexPath: cfg.EmbeddingIndexFile, Timeout: cfg.EmbeddingTimeout,
	})

	server := httpx.NewServer(
		cfg,
		store,
		syncManager,
		logger,
		httpx.WithSystemDatabase(controlDB),
		httpx.WithControlPlane(deviceService, commandService),
		httpx.WithWebAuthentication(authService),
		httpx.WithArtifactRelay(artifactService),
		httpx.WithEmbeddingService(embeddingService),
	)
	httpServer := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("nexus starting", "addr", cfg.Addr(), "nexus_data_dir", cfg.NexusDataDir, "recall_repo_dir", cfg.RecallRepoDir, "auto_sync", cfg.AutoSync, "embedding_enabled", cfg.EmbeddingEnabled, "embedding_model", cfg.EmbeddingModel)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	logger.Info("nexus stopped")
}
