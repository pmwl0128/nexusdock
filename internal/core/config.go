package core

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host            string
	Port            int
	DatabasePath    string
	BackupDir       string
	LogLevelName    string
	ShutdownTimeout time.Duration
	WorkerInterval  time.Duration
	MaxOpenConns    int
}

func ConfigFromEnv() Config {
	dbPath := envString("AGENTDOCK_NEXUS_DB", "data/nexus.db")
	return Config{
		Host:            envString("AGENTDOCK_NEXUS_HOST", "127.0.0.1"),
		Port:            envInt("AGENTDOCK_NEXUS_PORT", 18777),
		DatabasePath:    dbPath,
		BackupDir:       envString("AGENTDOCK_NEXUS_BACKUP_DIR", filepath.Join(filepath.Dir(dbPath), "backups")),
		LogLevelName:    envString("AGENTDOCK_NEXUS_LOG_LEVEL", "info"),
		ShutdownTimeout: time.Duration(envInt("AGENTDOCK_NEXUS_SHUTDOWN_SECONDS", 15)) * time.Second,
		WorkerInterval:  time.Duration(envInt("AGENTDOCK_NEXUS_WORKER_INTERVAL_SECONDS", 60)) * time.Second,
		MaxOpenConns:    envInt("AGENTDOCK_NEXUS_DB_MAX_OPEN_CONNS", 8),
	}
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("AGENTDOCK_NEXUS_HOST must not be empty")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("AGENTDOCK_NEXUS_PORT must be between 1 and 65535")
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		return errors.New("AGENTDOCK_NEXUS_DB must not be empty")
	}
	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}
	if c.WorkerInterval <= 0 {
		return errors.New("worker interval must be positive")
	}
	if c.MaxOpenConns < 1 {
		return errors.New("database max open connections must be positive")
	}
	return nil
}

func (c Config) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }

func (c Config) LogLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(c.LogLevelName)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
