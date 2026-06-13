package httpx

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadBackupMissingFiles(t *testing.T) {
	item := loadBackupStatus(t.TempDir(), time.Date(2026, 6, 7, 4, 0, 0, 0, time.Local))
	if item.State != "never_run" || item.ID != backupStatusID || item.NextRunAt == "" {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestLoadBackupStatusAndHistory(t *testing.T) {
	dir := t.TempDir()
	latest := `{"state":"success","message":"ok","started_at":"2026-06-07T05:00:00Z","completed_at":"2026-06-07T05:10:00Z","archive":"backup.tar.zst","archive_size":123,"sha256":"abc","remote_path":"/safe/backup.tar.zst"}`
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(latest), 0o600); err != nil {
		t.Fatal(err)
	}
	history := latest + "\nnot-json\n" + strings.Replace(latest, `"success"`, `"failed"`, 1) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(history), 0o600); err != nil {
		t.Fatal(err)
	}
	item := loadBackupStatus(dir, time.Now())
	if item.State != "success" || item.ArchiveSize != 123 || len(item.History) != 2 {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestLoadBackupCorruptLatest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	item := loadBackupStatus(dir, time.Now())
	if item.State != "unknown" || !strings.Contains(item.Message, "损坏") {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestNormalizeBackupState(t *testing.T) {
	cases := map[string]string{
		"success":   "success",
		"succeeded": "success",
		"completed": "success",
		"error":     "failed",
		"custom":    "unknown",
		"":          "unknown",
	}
	for input, want := range cases {
		if got := normalizeBackupState(input); got != want {
			t.Fatalf("normalizeBackupState(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBackupDoesNotExposeSecretsOrArbitraryPaths(t *testing.T) {
	dir := t.TempDir()
	secret := "super-secret-token"
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(`{"state":"success","message":"ok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token.txt"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	item := loadBackupStatus(dir, time.Now())
	if strings.Contains(item.Message, secret) {
		t.Fatal("secret leaked")
	}
	if backupStatusDir() == filepath.Join(dir, "token.txt") {
		t.Fatal("status path must be a directory")
	}
}

func TestGetBackupStatusReturnsSingleObject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NEXUS_BACKUP_STATUS_DIR", dir)
	request := httptest.NewRequest(http.MethodGet, "/v1/backup/status", nil)
	response := httptest.NewRecorder()
	server := &Server{}
	server.getBackupStatus(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var item backupStatus
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		t.Fatalf("decode backup status: %v", err)
	}
	if item.ID != backupStatusID || item.State != "never_run" {
		t.Fatalf("unexpected backup status: %+v", item)
	}
}

func TestRetiredScheduleRouteIsNotRegistered(t *testing.T) {
	server := &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	request := httptest.NewRequest(http.MethodGet, "/v1/schedules", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
}
