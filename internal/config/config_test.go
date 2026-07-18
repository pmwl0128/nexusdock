package config

import (
	"path/filepath"
	"testing"
)

func TestRequireAuthNeedsAPIToken(t *testing.T) {
	cfg := Config{RequireAuth: true}
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
