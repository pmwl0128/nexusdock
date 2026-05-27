package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host          string
	Port          int
	StoreDir      string
	AuthToken     string
	AutoSync      bool
	PullInterval  time.Duration
	PushDebounce  time.Duration
	CommitMessage string
	LogLevelName  string
}

func FromEnv() Config {
	return Config{
		Host:          getenv("MEMORYDOCK_HOST", "127.0.0.1"),
		Port:          getenvInt("MEMORYDOCK_PORT", 18777),
		StoreDir:      getenv("MEMORYDOCK_STORE_DIR", "memory"),
		AuthToken:     os.Getenv("MEMORYDOCK_AUTH_TOKEN"),
		AutoSync:      getenvBool("MEMORYDOCK_AUTO_SYNC", false),
		PullInterval:  time.Duration(getenvInt("MEMORYDOCK_PULL_INTERVAL_SECONDS", 120)) * time.Second,
		PushDebounce:  time.Duration(getenvInt("MEMORYDOCK_PUSH_DEBOUNCE_SECONDS", 10)) * time.Second,
		CommitMessage: getenv("MEMORYDOCK_COMMIT_MESSAGE", "memory: 自动同步记忆"),
		LogLevelName:  getenv("MEMORYDOCK_LOG_LEVEL", "info"),
	}
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

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

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
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

func getenvBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
