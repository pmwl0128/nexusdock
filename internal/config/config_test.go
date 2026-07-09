package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashPasswordVerifyPassword(t *testing.T) {
	hash, err := HashPassword("strong-secret")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "strong-secret" {
		t.Fatalf("password hash must not store plaintext")
	}
	if !VerifyPassword("strong-secret", hash) {
		t.Fatalf("expected password to verify")
	}
	if VerifyPassword("wrong-secret", hash) {
		t.Fatalf("wrong password verified")
	}
}

func TestAccessFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Username: "admin", PasswordHash: hash, AccessFile: filepath.Join(dir, "nexus", "access.json")}
	if err := cfg.SaveAccessFile(); err != nil {
		t.Fatalf("SaveAccessFile: %v", err)
	}
	data, err := os.ReadFile(cfg.AccessFile)
	if err != nil {
		t.Fatalf("read access file: %v", err)
	}
	if string(data) == "secret" || string(data) == hash {
		t.Fatalf("access file content sanity check failed")
	}
	loaded := Config{AccessFile: cfg.AccessFile}
	if err := loaded.LoadAccessFile(); err != nil {
		t.Fatalf("LoadAccessFile: %v", err)
	}
	if loaded.Username != "admin" || !loaded.CheckPassword("secret") {
		t.Fatalf("loaded config does not authenticate")
	}
}

func TestRequireAuthNeedsAPIToken(t *testing.T) {
	cfg := Config{RequireAuth: true, Username: "admin", Password: "not-default"}
	if err := cfg.ValidateStartup(); err == nil {
		t.Fatalf("expected missing bearer token to be rejected")
	}
	cfg.AuthToken = "token"
	if err := cfg.ValidateStartup(); err != nil {
		t.Fatalf("valid require auth config rejected: %v", err)
	}
}

func TestFromEnvUsesNexusDataDirAndRecallRepoDir(t *testing.T) {
	t.Setenv("NEXUS_DATA_DIR", "/tmp/nexus-data")
	t.Setenv("RECALL_REPO_DIR", "/tmp/recall-repo")

	cfg := FromEnv()

	if cfg.NexusDataDir != "/tmp/nexus-data" {
		t.Fatalf("NexusDataDir = %q", cfg.NexusDataDir)
	}
	if cfg.RecallRepoDir != "/tmp/recall-repo" {
		t.Fatalf("RecallRepoDir = %q", cfg.RecallRepoDir)
	}
	if cfg.StoreDir != cfg.RecallRepoDir {
		t.Fatalf("StoreDir should remain deprecated alias for RecallRepoDir, got %q", cfg.StoreDir)
	}
}

func TestFromEnvUsesNexusAndRecallVariables(t *testing.T) {
	t.Setenv("NEXUS_HOST", "0.0.0.0")
	t.Setenv("NEXUS_PORT", "18000")
	t.Setenv("NEXUS_AUTH_TOKEN", "nexus-token")
	t.Setenv("NEXUS_REQUIRE_AUTH", "true")
	t.Setenv("RECALL_AUTO_SYNC", "true")
	t.Setenv("RECALL_EMBEDDING_INDEX_FILE", "/tmp/recall-index.json")

	cfg := FromEnv()

	if cfg.Host != "0.0.0.0" || cfg.Port != 18000 {
		t.Fatalf("Nexus host/port should be used, got %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.AuthToken != "nexus-token" || !cfg.RequireAuth {
		t.Fatalf("Nexus auth settings should be used, token=%q require=%v", cfg.AuthToken, cfg.RequireAuth)
	}
	if !cfg.AutoSync {
		t.Fatalf("Recall auto sync should be enabled")
	}
	if cfg.EmbeddingIndexFile != "/tmp/recall-index.json" {
		t.Fatalf("Recall embedding index should be used, got %q", cfg.EmbeddingIndexFile)
	}
}

func TestFromEnvDefaultsRecallEmbeddingIndexUnderRecallDirectory(t *testing.T) {
	t.Setenv("RECALL_REPO_DIR", "/tmp/recall-repo")

	cfg := FromEnv()

	want := filepath.Join("/tmp/recall-repo", ".recall", "embedding-index.json")
	if cfg.EmbeddingIndexFile != want {
		t.Fatalf("EmbeddingIndexFile = %q, want %q", cfg.EmbeddingIndexFile, want)
	}
}
