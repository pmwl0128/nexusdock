package core

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SQLiteBackupHook struct {
	SourcePath string
	Directory  string
	Now        func() time.Time
}

func (h SQLiteBackupHook) BeforeMigrate(ctx context.Context, db *sql.DB, pending []Migration) error {
	if len(pending) == 0 || strings.TrimSpace(h.Directory) == "" || strings.TrimSpace(h.SourcePath) == "" || h.SourcePath == ":memory:" || strings.HasPrefix(h.SourcePath, "file:") {
		return nil
	}
	info, err := os.Stat(h.SourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat database: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	if err := os.MkdirAll(h.Directory, 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	target := filepath.Join(h.Directory, "nexus-"+now().UTC().Format("20060102T150405.000000000Z")+".db")
	escaped := strings.ReplaceAll(target, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return fmt.Errorf("vacuum into %s: %w", target, err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		return fmt.Errorf("chmod backup: %w", err)
	}
	return nil
}
