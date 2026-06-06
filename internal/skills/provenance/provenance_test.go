package provenance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDigestDirectoryStableAndSensitive(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := DigestDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DigestDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("digest not stable: %s != %s", first, second)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := DigestDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("digest did not change")
	}
}

func TestSanitizeURI(t *testing.T) {
	got := SanitizeURI("https://user:secret@example.com/repo.git?token=abc#fragment")
	if got != "https://example.com/repo.git" {
		t.Fatalf("SanitizeURI() = %q", got)
	}
	record := Record{SourceType: SourceGit, SourceURI: got, Digest: string(make([]byte, 64)), ImportedAt: time.Now()}
	if err := record.Validate(); err == nil {
		t.Fatal("expected invalid hex digest")
	}
}
