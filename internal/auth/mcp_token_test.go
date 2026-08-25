package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPTokenStorePersistsAndResets(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewMCPTokenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	first := store.Token()
	if len(first) != 64 {
		t.Fatalf("token length=%d want=64", len(first))
	}

	path := filepath.Join(dataDir, "secrets", "mcp-access-token")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("token mode=%o want=600", got)
	}

	reopened, err := NewMCPTokenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Token() != first {
		t.Fatal("reopened store did not keep token")
	}

	second, err := reopened.Reset()
	if err != nil {
		t.Fatal(err)
	}
	if second == first || reopened.Token() != second {
		t.Fatal("reset did not replace token")
	}

	reopenedAgain, err := NewMCPTokenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if reopenedAgain.Token() != second {
		t.Fatal("reset token did not persist")
	}
}
