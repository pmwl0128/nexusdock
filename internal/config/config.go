package config

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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
	Host                  string
	Port                  int
	StoreDir              string
	AuthToken             string
	Username              string
	Password              string
	PasswordHash          string
	AccessFile            string
	RequireAuth           bool
	AuthAllowInsecureHTTP bool
	TrustedProxies        []string
	AutoSync              bool
	PullInterval          time.Duration
	PushDebounce          time.Duration
	CommitMessage         string
	LogLevelName          string
	EmbeddingEnabled      bool
	EmbeddingEndpoint     string
	EmbeddingModel        string
	EmbeddingIndexFile    string
	EmbeddingTimeout      time.Duration
}

func FromEnv() Config {
	storeDir := getenvAny("RECALLDOCK_STORE_DIR", "MEMORYDOCK_STORE_DIR", "memory")
	defaultAccessFile := filepath.Join(storeDir, ".recalldock", "access.json")
	legacyAccessFile := filepath.Join(storeDir, ".memorydock", "access.json")
	if _, err := os.Stat(legacyAccessFile); err == nil {
		defaultAccessFile = legacyAccessFile
	}
	defaultEmbeddingIndexFile := filepath.Join(storeDir, ".recalldock", "embedding-index.json")
	legacyEmbeddingIndexFile := filepath.Join(storeDir, ".memorydock", "embedding-index.json")
	if _, err := os.Stat(legacyEmbeddingIndexFile); err == nil {
		defaultEmbeddingIndexFile = legacyEmbeddingIndexFile
	}
	cfg := Config{
		Host:                  getenvAny("RECALLDOCK_HOST", "MEMORYDOCK_HOST", "127.0.0.1"),
		Port:                  getenvIntAny("RECALLDOCK_PORT", "MEMORYDOCK_PORT", 18777),
		StoreDir:              storeDir,
		AuthToken:             firstNonEmpty(os.Getenv("RECALLDOCK_AUTH_TOKEN"), os.Getenv("MEMORYDOCK_AUTH_TOKEN")),
		Username:              firstNonEmpty(os.Getenv("RECALLDOCK_USERNAME"), os.Getenv("MEMORYDOCK_USERNAME")),
		Password:              firstNonEmpty(os.Getenv("RECALLDOCK_PASSWORD"), os.Getenv("MEMORYDOCK_PASSWORD")),
		PasswordHash:          firstNonEmpty(os.Getenv("RECALLDOCK_PASSWORD_HASH"), os.Getenv("MEMORYDOCK_PASSWORD_HASH")),
		AccessFile:            getenvAny("RECALLDOCK_ACCESS_FILE", "MEMORYDOCK_ACCESS_FILE", defaultAccessFile),
		RequireAuth:           getenvBoolAny("RECALLDOCK_REQUIRE_AUTH", "MEMORYDOCK_REQUIRE_AUTH", false),
		AuthAllowInsecureHTTP: getenvBool("NEXUS_AUTH_ALLOW_INSECURE_HTTP", false),
		TrustedProxies:        splitCSV(getenv("NEXUS_TRUSTED_PROXIES", "127.0.0.1,::1")),
		AutoSync:              getenvBoolAny("RECALLDOCK_AUTO_SYNC", "MEMORYDOCK_AUTO_SYNC", false),
		PullInterval:          time.Duration(getenvIntAny("RECALLDOCK_PULL_INTERVAL_SECONDS", "MEMORYDOCK_PULL_INTERVAL_SECONDS", 120)) * time.Second,
		PushDebounce:          time.Duration(getenvIntAny("RECALLDOCK_PUSH_DEBOUNCE_SECONDS", "MEMORYDOCK_PUSH_DEBOUNCE_SECONDS", 10)) * time.Second,
		CommitMessage:         getenvAny("RECALLDOCK_COMMIT_MESSAGE", "MEMORYDOCK_COMMIT_MESSAGE", "recall: 自动同步召回库"),
		LogLevelName:          getenvAny("RECALLDOCK_LOG_LEVEL", "MEMORYDOCK_LOG_LEVEL", "info"),
		EmbeddingEnabled:      getenvBoolAny("RECALLDOCK_EMBEDDING_ENABLED", "MEMORYDOCK_EMBEDDING_ENABLED", false),
		EmbeddingEndpoint:     firstNonEmpty(os.Getenv("RECALLDOCK_EMBEDDING_ENDPOINT"), os.Getenv("MEMORYDOCK_EMBEDDING_ENDPOINT")),
		EmbeddingModel:        getenvAny("RECALLDOCK_EMBEDDING_MODEL", "MEMORYDOCK_EMBEDDING_MODEL", "BAAI/bge-m3"),
		EmbeddingIndexFile:    getenvAny("RECALLDOCK_EMBEDDING_INDEX_FILE", "MEMORYDOCK_EMBEDDING_INDEX_FILE", defaultEmbeddingIndexFile),
		EmbeddingTimeout:      time.Duration(getenvIntAny("RECALLDOCK_EMBEDDING_TIMEOUT_SECONDS", "MEMORYDOCK_EMBEDDING_TIMEOUT_SECONDS", 30)) * time.Second,
	}
	_ = cfg.LoadAccessFile()
	return cfg
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func getenvAny(primary, legacy, fallback string) string {
	if value := firstNonEmpty(os.Getenv(primary), os.Getenv(legacy)); value != "" {
		return value
	}
	return fallback
}

func getenvIntAny(primary, legacy string, fallback int) int {
	for _, key := range []string{primary, legacy} {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getenvBoolAny(primary, legacy string, fallback bool) bool {
	for _, key := range []string{primary, legacy} {
		value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
		if value == "" {
			continue
		}
		switch value {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
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

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

const passwordHashPrefix = "pbkdf2-sha256"
const passwordHashIterations = 210000
const passwordSaltBytes = 16
const passwordKeyBytes = 32

type AccessFile struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

func (c *Config) AccessEnabled() bool {
	return strings.TrimSpace(c.Username) != "" && (c.Password != "" || c.PasswordHash != "")
}

func (c *Config) CheckPassword(secret string) bool {
	if c.PasswordHash != "" {
		return VerifyPassword(secret, c.PasswordHash)
	}
	return hmac.Equal([]byte(secret), []byte(c.Password))
}

func (c *Config) LoadAccessFile() error {
	if strings.TrimSpace(c.AccessFile) == "" {
		return nil
	}
	data, err := os.ReadFile(c.AccessFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var file AccessFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	if c.Username == "" {
		c.Username = strings.TrimSpace(file.Username)
	}
	if c.Password == "" && c.PasswordHash == "" {
		c.PasswordHash = strings.TrimSpace(file.PasswordHash)
	}
	return nil
}

func (c Config) SaveAccessFile() error {
	if strings.TrimSpace(c.AccessFile) == "" {
		return errors.New("RECALLDOCK_ACCESS_FILE is empty")
	}
	if strings.TrimSpace(c.Username) == "" || strings.TrimSpace(c.PasswordHash) == "" {
		return errors.New("username and password hash are required")
	}
	if err := os.MkdirAll(filepath.Dir(c.AccessFile), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(AccessFile{Username: c.Username, PasswordHash: c.PasswordHash, UpdatedAt: time.Now().Format(time.RFC3339)}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.AccessFile, append(data, '\n'), 0o600)
}

func HashPassword(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", errors.New("secret is required")
	}
	salt := make([]byte, passwordSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2SHA256([]byte(secret), salt, passwordHashIterations, passwordKeyBytes)
	return fmt.Sprintf("%s$%d$%s$%s", passwordHashPrefix, passwordHashIterations, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(secret, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != passwordHashPrefix {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 || iterations > 1000000 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil || len(salt) == 0 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false
	}
	got := pbkdf2SHA256([]byte(secret), salt, iterations, len(want))
	return hmac.Equal(got, want)
}

func pbkdf2SHA256(password, salt []byte, iterations, keyLen int) []byte {
	hLen := sha256.Size
	nBlocks := (keyLen + hLen - 1) / hLen
	out := make([]byte, 0, nBlocks*hLen)
	for block := 1; block <= nBlocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		mac.Write([]byte{byte(block >> 24), byte(block >> 16), byte(block >> 8), byte(block)})
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iterations; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func (c Config) ValidateStartup() error {
	if !c.RequireAuth {
		return nil
	}
	if strings.TrimSpace(c.AuthToken) == "" {
		return errors.New("MEMORYDOCK_REQUIRE_AUTH=true requires MEMORYDOCK_AUTH_TOKEN for programmatic API access")
	}
	return nil
}
