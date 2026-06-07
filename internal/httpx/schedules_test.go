package httpx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadScheduleMissingFiles(t *testing.T) {
	item := loadScheduleItem(t.TempDir(), time.Date(2026, 6, 7, 4, 0, 0, 0, time.Local))
	if item.State != "never_run" || item.ID != backupScheduleID || item.NextRunAt == "" {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestLoadScheduleStatusAndHistory(t *testing.T) {
	dir := t.TempDir()
	latest := `{"state":"success","message":"ok","started_at":"2026-06-07T05:00:00Z","completed_at":"2026-06-07T05:10:00Z","archive":"backup.tar.zst","archive_size":123,"sha256":"abc","remote_path":"/safe/backup.tar.zst"}`
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(latest), 0o600); err != nil {
		t.Fatal(err)
	}
	history := latest + "\nnot-json\n" + strings.Replace(latest, `"success"`, `"failed"`, 1) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "history.jsonl"), []byte(history), 0o600); err != nil {
		t.Fatal(err)
	}
	item := loadScheduleItem(dir, time.Now())
	if item.State != "success" || item.ArchiveSize != 123 || len(item.History) != 2 {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestLoadScheduleCorruptLatest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	item := loadScheduleItem(dir, time.Now())
	if item.State != "unknown" || !strings.Contains(item.Message, "损坏") {
		t.Fatalf("unexpected item: %+v", item)
	}
}

func TestNormalizeScheduleState(t *testing.T) {
	cases := map[string]string{
		"success":   "success",
		"succeeded": "success",
		"completed": "success",
		"error":     "failed",
		"custom":    "unknown",
		"":          "unknown",
	}
	for input, want := range cases {
		if got := normalizeScheduleState(input); got != want {
			t.Fatalf("normalizeScheduleState(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestScheduleDoesNotExposeSecretsOrArbitraryPaths(t *testing.T) {
	dir := t.TempDir()
	secret := "super-secret-token"
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(`{"state":"success","message":"ok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token.txt"), []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	item := loadScheduleItem(dir, time.Now())
	if strings.Contains(item.Message, secret) {
		t.Fatal("secret leaked")
	}
	if scheduleStatusDir() == filepath.Join(dir, "token.txt") {
		t.Fatal("status path must be a directory")
	}
}
