package recall

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateNotesAreOutsideOrdinaryRecall(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	privatePath := filepath.Join(root, "private-notes", "notes", "recovery", "secret.md")
	if err := os.MkdirAll(filepath.Dir(privatePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privatePath, []byte("---\ntitle: Hidden recovery\nsummary: hidden metadata\n---\n\nBODY_ONLY_PRIVATE_MARKER\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ordinaryPath := filepath.Join(root, "recall", "docs", "inbox", "ordinary.md")
	if err := os.MkdirAll(filepath.Dir(ordinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ordinaryPath, []byte("# Ordinary\n\nordinary marker\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := store.List("", 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Path == "private-notes" || len(entry.Path) >= len("private-notes/") && entry.Path[:len("private-notes/")] == "private-notes/" {
			t.Fatalf("private note leaked through recall list: %#v", entry)
		}
	}
	results, err := store.Search("BODY_ONLY_PRIVATE_MARKER", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("private note leaked through recall search: %#v", results)
	}
	if _, err := store.Read("private-notes/notes/recovery/secret.md"); !errors.Is(err, ErrDisallowedPath) {
		t.Fatalf("private note recall read error = %v, want ErrDisallowedPath", err)
	}
}
