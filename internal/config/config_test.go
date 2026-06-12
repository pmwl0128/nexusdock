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
	cfg := Config{Username: "admin", PasswordHash: hash, AccessFile: filepath.Join(dir, ".memorydock", "access.json")}
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
