package core

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	migrationfiles "github.com/uvwt/nexusdock/migrations"
)

type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

type BackupHook interface {
	BeforeMigrate(context.Context, *sql.DB, []Migration) error
}

type MigrationRunner struct {
	db   *sql.DB
	hook BackupHook
	now  func() time.Time
}

func NewMigrationRunner(db *sql.DB, hook BackupHook) *MigrationRunner {
	return &MigrationRunner{db: db, hook: hook, now: time.Now}
}

func (r *MigrationRunner) Run(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
        version INTEGER PRIMARY KEY,
        name TEXT NOT NULL,
        checksum TEXT NOT NULL,
        applied_at TEXT NOT NULL
    )`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	applied, err := r.applied(ctx)
	if err != nil {
		return err
	}
	var pending []Migration
	for _, migration := range migrations {
		if checksum, ok := applied[migration.Version]; ok {
			if checksum != migration.Checksum {
				return NewError(CodeDBConflict, fmt.Sprintf("migration %d checksum changed", migration.Version), nil)
			}
			continue
		}
		pending = append(pending, migration)
	}
	if len(pending) == 0 {
		return nil
	}
	if r.hook != nil {
		if err := r.hook.BeforeMigrate(ctx, r.db, pending); err != nil {
			return fmt.Errorf("backup before migration: %w", err)
		}
	}
	for _, migration := range pending {
		migration := migration
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", migration.Version, err)
		}
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES(?, ?, ?, ?)`,
			migration.Version, migration.Name, migration.Checksum, r.now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.Version, err)
		}
	}
	return nil
}

func (r *MigrationRunner) CurrentVersion(ctx context.Context) (int, error) {
	var version int
	err := r.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func (r *MigrationRunner) applied(ctx context.Context) (map[int]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()
	result := make(map[int]string)
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		result[version] = checksum
	}
	return result, rows.Err()
}

func loadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationfiles.Files, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	result := make([]Migration, 0, len(entries))
	seen := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if previous, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %d in %q and %q", version, previous, entry.Name())
		}
		data, err := migrationfiles.Files.ReadFile(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		sum := sha256.Sum256(data)
		result = append(result, Migration{
			Version:  version,
			Name:     entry.Name(),
			SQL:      string(data),
			Checksum: hex.EncodeToString(sum[:]),
		})
		seen[version] = entry.Name()
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	return result, nil
}
