package recall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewWriteValidatesWithoutMutatingStore(t *testing.T) {
	store := newTestStore(t)
	path := "recall/docs/inbox/preview.md"
	preview, err := store.PreviewWrite(WriteRequest{Path: path, Content: "# Preview", Confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Path != path || !strings.Contains(preview.ProposedContent, "# Preview") {
		t.Fatalf("preview = %#v", preview)
	}
	if _, err := os.Stat(filepath.Join(store.Root(), filepath.FromSlash(path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("PreviewWrite created a file: %v", err)
	}

	if _, err := store.Write(WriteRequest{Path: path, Content: "# Existing", Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PreviewWrite(WriteRequest{Path: path, Content: "# Replacement", Confirmed: true}); !errors.Is(err, ErrFileExists) {
		t.Fatalf("preview did not enforce overwrite rules: %v", err)
	}
	preview, err = store.PreviewWrite(WriteRequest{Path: path, Content: "# Replacement", Confirmed: true, Overwrite: true})
	if err != nil || !preview.Overwrite {
		t.Fatalf("overwrite preview = %#v err=%v", preview, err)
	}
	current, err := store.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(current.Content, "Replacement") {
		t.Fatalf("overwrite preview mutated store: %q", current.Content)
	}
}
