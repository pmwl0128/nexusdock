package commands

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
)

type SQLiteRepository struct {
	db *sql.DB
	mu sync.Mutex
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

func (r *SQLiteRepository) Enqueue(ctx context.Context, command Command) (Command, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, err := r.findByIdempotency(ctx, command.DeviceID, command.IdempotencyKey); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Command{}, false, err
	}
	payload, err := json.Marshal(command)
	if err != nil {
		return Command{}, false, err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO device_commands_v1(id,device_id,idempotency_key,status,priority,not_before,expires_at,lease_expires_at,version,payload_json,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		command.ID, command.DeviceID, command.IdempotencyKey, command.Status, command.Priority,
		command.NotBefore.UTC().Format(time.RFC3339Nano), command.ExpiresAt.UTC().Format(time.RFC3339Nano), nullableTime(command.LeaseExpiresAt),
		command.Version, string(payload), command.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if existing, lookupErr := r.findByIdempotency(ctx, command.DeviceID, command.IdempotencyKey); lookupErr == nil {
			return existing, false, nil
		}
		return Command{}, false, err
	}
	return cloneCommand(command), true, nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id string) (Command, error) {
	var payload string
	if err := r.db.QueryRowContext(ctx, `SELECT payload_json FROM device_commands_v1 WHERE id=?`, id).Scan(&payload); errors.Is(err, sql.ErrNoRows) {
		return Command{}, commandError(ErrCommandNotFound, "command %q not found", id)
	} else if err != nil {
		return Command{}, err
	}
	return decodeCommand(payload)
}

func (r *SQLiteRepository) ListByDevice(ctx context.Context, deviceID string) ([]Command, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT payload_json FROM device_commands_v1 WHERE device_id=? ORDER BY not_before,id`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Command
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		command, err := decodeCommand(payload)
		if err != nil {
			return nil, err
		}
		result = append(result, command)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, rows.Err()
}

func (r *SQLiteRepository) Update(ctx context.Context, command Command, expectedVersion int64) (Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	command.Version = expectedVersion + 1
	payload, err := json.Marshal(command)
	if err != nil {
		return Command{}, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE device_commands_v1 SET status=?,priority=?,not_before=?,expires_at=?,lease_expires_at=?,version=?,payload_json=?,updated_at=? WHERE id=? AND version=?`,
		command.Status, command.Priority, command.NotBefore.UTC().Format(time.RFC3339Nano), command.ExpiresAt.UTC().Format(time.RFC3339Nano), nullableTime(command.LeaseExpiresAt), command.Version, string(payload), command.UpdatedAt.UTC().Format(time.RFC3339Nano), command.ID, expectedVersion)
	if err != nil {
		return Command{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, e := r.Get(ctx, command.ID); e != nil {
			return Command{}, e
		}
		return Command{}, commandError(ErrVersionConflict, "command %q changed concurrently", command.ID)
	}
	return cloneCommand(command), nil
}

func (r *SQLiteRepository) LeaseNext(ctx context.Context, deviceID string, now time.Time, leaseDuration time.Duration) (Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	commands, err := r.listAllByDevice(ctx, deviceID)
	if err != nil {
		return Command{}, err
	}
	var candidate *Command
	for i := range commands {
		command := commands[i]
		if command.Status.Terminal() || now.Before(command.NotBefore) {
			continue
		}
		if !now.Before(command.ExpiresAt) {
			command.Status = StatusExpired
			command.LeaseID = ""
			command.LeaseExpiresAt = nil
			command.CompletedAt = ptrTime(now)
			command.UpdatedAt = now
			if _, err := r.updateUnlocked(ctx, command, command.Version); err != nil {
				return Command{}, err
			}
			continue
		}
		leaseable := command.Status == StatusQueued || ((command.Status == StatusLeased || command.Status == StatusRunning) && command.LeaseExpiresAt != nil && !now.Before(*command.LeaseExpiresAt))
		if leaseable && command.Attempts >= command.MaxAttempts {
			command.Status = StatusFailed
			command.LeaseID = ""
			command.LeaseExpiresAt = nil
			command.Result = &Result{Success: false, ErrorCode: "MAX_ATTEMPTS_EXCEEDED", Error: "command lease expired after maximum attempts", FinishedAt: now}
			command.CompletedAt = ptrTime(now)
			command.UpdatedAt = now
			if _, err := r.updateUnlocked(ctx, command, command.Version); err != nil {
				return Command{}, err
			}
			continue
		}
		if !leaseable {
			continue
		}
		if candidate == nil || command.Priority > candidate.Priority || (command.Priority == candidate.Priority && command.CreatedAt.Before(candidate.CreatedAt)) {
			copyValue := command
			candidate = &copyValue
		}
	}
	if candidate == nil {
		return Command{}, commandError(ErrCommandNotLeaseable, "no leaseable command for device %q", deviceID)
	}
	expected := candidate.Version
	candidate.Status = StatusLeased
	candidate.LeaseID = newLeaseID()
	candidate.LeaseExpiresAt = ptrTime(now.Add(leaseDuration))
	candidate.Attempts++
	candidate.UpdatedAt = now
	return r.updateUnlocked(ctx, *candidate, expected)
}

func (r *SQLiteRepository) findByIdempotency(ctx context.Context, deviceID, key string) (Command, error) {
	var payload string
	err := r.db.QueryRowContext(ctx, `SELECT payload_json FROM device_commands_v1 WHERE device_id=? AND idempotency_key=?`, deviceID, key).Scan(&payload)
	if err != nil {
		return Command{}, err
	}
	return decodeCommand(payload)
}

func (r *SQLiteRepository) listAllByDevice(ctx context.Context, deviceID string) ([]Command, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT payload_json FROM device_commands_v1 WHERE device_id=?`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Command
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		command, err := decodeCommand(payload)
		if err != nil {
			return nil, err
		}
		result = append(result, command)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) updateUnlocked(ctx context.Context, command Command, expectedVersion int64) (Command, error) {
	command.Version = expectedVersion + 1
	payload, err := json.Marshal(command)
	if err != nil {
		return Command{}, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE device_commands_v1 SET status=?,priority=?,not_before=?,expires_at=?,lease_expires_at=?,version=?,payload_json=?,updated_at=? WHERE id=? AND version=?`, command.Status, command.Priority, command.NotBefore.UTC().Format(time.RFC3339Nano), command.ExpiresAt.UTC().Format(time.RFC3339Nano), nullableTime(command.LeaseExpiresAt), command.Version, string(payload), command.UpdatedAt.UTC().Format(time.RFC3339Nano), command.ID, expectedVersion)
	if err != nil {
		return Command{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Command{}, commandError(ErrVersionConflict, "command %q changed concurrently", command.ID)
	}
	return cloneCommand(command), nil
}

func decodeCommand(payload string) (Command, error) {
	var command Command
	err := json.Unmarshal([]byte(payload), &command)
	return command, err
}
func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
