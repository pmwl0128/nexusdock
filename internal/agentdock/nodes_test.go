package agentdock

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvwt/nexusdock/internal/core"
)

func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := core.OpenSQLite(context.Background(), ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE agentdock_nodes (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		endpoint TEXT NOT NULL UNIQUE,
		enabled INTEGER NOT NULL,
		timeout_seconds INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE agentdock_node_secrets (
		node_id TEXT PRIMARY KEY,
		token_ciphertext BLOB NOT NULL,
		updated_at TEXT NOT NULL,
		FOREIGN KEY (node_id) REFERENCES agentdock_nodes(id) ON DELETE CASCADE
	);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	store, err := NewStoreWithKey(db, make([]byte, 32))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store, db
}

func TestStoreLifecycleEncryptsToken(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()
	enabled := true
	node, err := store.Create(ctx, CreateInput{
		ID: "dockmini", Name: "DockMini", Endpoint: "http://host.docker.internal:18766/",
		Token: "runtime-secret", Enabled: &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.Endpoint != "http://host.docker.internal:18766" || node.TimeoutSeconds != 8 || !node.TokenConfigured {
		t.Fatalf("unexpected node: %#v", node)
	}

	var ciphertext []byte
	if err := db.QueryRow(`SELECT token_ciphertext FROM agentdock_node_secrets WHERE node_id = 'dockmini'`).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ciphertext), "runtime-secret") {
		t.Fatal("database contains plaintext AgentDock token")
	}
	credentials, err := store.Credentials(ctx, "dockmini")
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Token != "runtime-secret" || credentials.Node.ID != "dockmini" {
		t.Fatalf("unexpected credentials: node=%#v token_matches=%v", credentials.Node, credentials.Token == "runtime-secret")
	}

	name := "Mac mini"
	timeout := 15
	newToken := "replacement-secret"
	updated, err := store.Update(ctx, "dockmini", UpdateInput{Name: &name, TimeoutSeconds: &timeout, Token: &newToken})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != name || updated.TimeoutSeconds != timeout {
		t.Fatalf("unexpected update: %#v", updated)
	}
	credentials, err = store.Credentials(ctx, "dockmini")
	if err != nil || credentials.Token != newToken {
		t.Fatalf("updated credentials unavailable: token_matches=%v err=%v", credentials.Token == newToken, err)
	}

	if err := store.Delete(ctx, "dockmini"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "dockmini"); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("Get after delete error = %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agentdock_node_secrets`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("secret row not deleted: count=%d err=%v", count, err)
	}
}

func TestStoreRejectsInvalidAndDuplicateNodes(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()
	valid := CreateInput{ID: "dockmini", Name: "DockMini", Endpoint: "https://dockmini.example.com", Token: "secret"}
	if _, err := store.Create(ctx, valid); err != nil {
		t.Fatal(err)
	}
	duplicate := valid
	duplicate.ID = "dockair"
	if _, err := store.Create(ctx, duplicate); !errors.Is(err, ErrNodeExists) {
		t.Fatalf("duplicate endpoint error = %v", err)
	}
	for _, input := range []CreateInput{
		{ID: "Dock Mini", Name: "bad", Endpoint: "https://example.com", Token: "secret"},
		{ID: "bad", Name: "bad", Endpoint: "https://example.com/mcp", Token: "secret"},
		{ID: "bad", Name: "bad", Endpoint: "file:///tmp/agentdock", Token: "secret"},
		{ID: "bad", Name: "bad", Endpoint: "https://example.com", Token: ""},
	} {
		if _, err := store.Create(ctx, input); err == nil {
			t.Fatalf("invalid input accepted: %#v", input)
		}
	}
}

func TestDisabledNodeCannotProvideCredentials(t *testing.T) {
	store, _ := newTestStore(t)
	enabled := false
	if _, err := store.Create(context.Background(), CreateInput{ID: "offline", Name: "Offline", Endpoint: "https://offline.example.com", Token: "secret", Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Credentials(context.Background(), "offline"); !errors.Is(err, ErrNodeDisabled) {
		t.Fatalf("credentials error = %v", err)
	}
}

func TestNewStoreCreatesPrivatePersistentKey(t *testing.T) {
	dir := t.TempDir()
	db, err := core.OpenSQLite(context.Background(), ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := NewStore(db, dir); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "secrets", "agentdock-nodes.key")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %o, want 600", info.Mode().Perm())
	}
	data, err := os.ReadFile(keyPath)
	if err != nil || len(data) != 32 {
		t.Fatalf("key length = %d err=%v", len(data), err)
	}
}

func TestNewStoreRepairsExistingKeyPermissions(t *testing.T) {
	dir := t.TempDir()
	keyDir := filepath.Join(dir, "secrets")
	keyPath := filepath.Join(keyDir, "agentdock-nodes.key")
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, make([]byte, 32), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := core.OpenSQLite(context.Background(), ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := NewStore(db, dir); err != nil {
		t.Fatal(err)
	}

	dirInfo, err := os.Stat(keyDir)
	if err != nil {
		t.Fatal(err)
	}
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 || keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("permissions dir=%o key=%o, want 700/600", dirInfo.Mode().Perm(), keyInfo.Mode().Perm())
	}
}
