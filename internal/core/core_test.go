package core

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSchemaIsIdempotentAndPersistent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nexus.db")
	db, err := OpenSQLite(ctx, path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatalf("second ensure failed: %v", err)
	}
	for _, table := range []string{"agentdock_devices", "agentdock_pairing_codes", "agentdock_tool_contracts", "runtime_ai_settings", "runtime_ai_setting_secrets"} {
		var name string
		if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
			t.Fatalf("%s missing: %v", table, err)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE tasks(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	var removedTable string
	err = db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='tasks'`).Scan(&removedTable)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unused table should be removed, err=%v table=%q", err, removedTable)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id, username, created_at, updated_at) VALUES('u1', 'alice', 'now', 'now')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenSQLite(ctx, path, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var username string
	if err := db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = 'u1'`).Scan(&username); err != nil {
		t.Fatal(err)
	}
	if username != "alice" {
		t.Fatalf("username = %q, want alice", username)
	}
}

func TestTxManagerRollsBackOnError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := OpenSQLite(ctx, ":memory:", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE values_test(value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("stop")
	err = NewTxManager(db).WithinTx(ctx, nil, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO values_test(value) VALUES('x')`); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM values_test`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want rollback to 0", count)
	}
}

func TestSQLiteBackupHookCreatesPrivateBackup(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "nexus.db")
	db, err := OpenSQLite(ctx, path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE existing(value TEXT); INSERT INTO existing VALUES('kept')`); err != nil {
		t.Fatal(err)
	}
	backupDir := filepath.Join(dir, "backups")
	hook := SQLiteBackupHook{SourcePath: path, Directory: backupDir}
	if err := hook.Backup(ctx, db); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("backup count = %d, want 1", len(entries))
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("backup mode = %o, want 600", info.Mode().Perm())
	}
}
