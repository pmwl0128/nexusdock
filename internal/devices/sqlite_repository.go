package devices

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

type SQLiteRepository struct {
	db *sql.DB
	mu sync.Mutex
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

func (r *SQLiteRepository) CreateEnrollmentToken(ctx context.Context, token EnrollmentToken) error {
	payload, err := json.Marshal(token)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO device_enrollment_tokens(digest,payload_json,expires_at,used_at) VALUES(?,?,?,?)`, token.Digest, string(payload), token.ExpiresAt.UTC().Format(time.RFC3339Nano), timeText(token.UsedAt))
	if err != nil {
		return domainError(ErrVersionConflict, "enrollment token already exists")
	}
	return nil
}

func (r *SQLiteRepository) CommitEnrollment(ctx context.Context, digest string, now time.Time, build func(EnrollmentToken) (Device, error)) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback()
	var payload string
	if err := tx.QueryRowContext(ctx, `SELECT payload_json FROM device_enrollment_tokens WHERE digest=?`, digest).Scan(&payload); errors.Is(err, sql.ErrNoRows) {
		return Device{}, domainError(ErrEnrollmentTokenInvalid, "enrollment token is invalid")
	} else if err != nil {
		return Device{}, err
	}
	var token EnrollmentToken
	if err := json.Unmarshal([]byte(payload), &token); err != nil {
		return Device{}, err
	}
	if token.UsedAt != nil {
		return Device{}, domainError(ErrEnrollmentTokenUsed, "enrollment token was already used")
	}
	if !now.Before(token.ExpiresAt) {
		return Device{}, domainError(ErrEnrollmentTokenExpired, "enrollment token expired")
	}
	device, err := build(token)
	if err != nil {
		return Device{}, err
	}
	token.UsedAt = ptrTime(now)
	tokenJSON, _ := json.Marshal(token)
	deviceJSON, _ := json.Marshal(device)
	if _, err = tx.ExecContext(ctx, `UPDATE device_enrollment_tokens SET payload_json=?,used_at=? WHERE digest=?`, string(tokenJSON), now.UTC().Format(time.RFC3339Nano), digest); err != nil {
		return Device{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO device_records(id,version,status,payload_json,updated_at) VALUES(?,?,?,?,?)`, device.ID, device.Version, device.Status, string(deviceJSON), device.UpdatedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		return Device{}, domainError(ErrDeviceAlreadyExists, "device %q already exists", device.ID)
	}
	if err = tx.Commit(); err != nil {
		return Device{}, err
	}
	return cloneDevice(device), nil
}

func (r *SQLiteRepository) ConsumeEnrollmentToken(ctx context.Context, digest string, now time.Time) (EnrollmentToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var payload string
	if err := r.db.QueryRowContext(ctx, `SELECT payload_json FROM device_enrollment_tokens WHERE digest=?`, digest).Scan(&payload); errors.Is(err, sql.ErrNoRows) {
		return EnrollmentToken{}, domainError(ErrEnrollmentTokenInvalid, "enrollment token is invalid")
	} else if err != nil {
		return EnrollmentToken{}, err
	}
	var token EnrollmentToken
	if err := json.Unmarshal([]byte(payload), &token); err != nil {
		return EnrollmentToken{}, err
	}
	if token.UsedAt != nil {
		return EnrollmentToken{}, domainError(ErrEnrollmentTokenUsed, "enrollment token was already used")
	}
	if !now.Before(token.ExpiresAt) {
		return EnrollmentToken{}, domainError(ErrEnrollmentTokenExpired, "enrollment token expired")
	}
	token.UsedAt = ptrTime(now)
	updated, _ := json.Marshal(token)
	_, err := r.db.ExecContext(ctx, `UPDATE device_enrollment_tokens SET payload_json=?,used_at=? WHERE digest=?`, string(updated), now.UTC().Format(time.RFC3339Nano), digest)
	return token, err
}

func (r *SQLiteRepository) CreateDevice(ctx context.Context, device Device) error {
	payload, _ := json.Marshal(device)
	_, err := r.db.ExecContext(ctx, `INSERT INTO device_records(id,version,status,payload_json,updated_at) VALUES(?,?,?,?,?)`, device.ID, device.Version, device.Status, string(payload), device.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return domainError(ErrDeviceAlreadyExists, "device %q already exists", device.ID)
	}
	return nil
}

func (r *SQLiteRepository) GetDevice(ctx context.Context, id string) (Device, error) {
	var payload string
	if err := r.db.QueryRowContext(ctx, `SELECT payload_json FROM device_records WHERE id=?`, id).Scan(&payload); errors.Is(err, sql.ErrNoRows) {
		return Device{}, domainError(ErrDeviceNotFound, "device %q not found", id)
	} else if err != nil {
		return Device{}, err
	}
	var device Device
	if err := json.Unmarshal([]byte(payload), &device); err != nil {
		return Device{}, err
	}
	return device, nil
}

func (r *SQLiteRepository) ListDevices(ctx context.Context) ([]Device, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT payload_json FROM device_records ORDER BY updated_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Device
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var d Device
		if err := json.Unmarshal([]byte(payload), &d); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) UpdateDevice(ctx context.Context, device Device, expectedVersion int64) (Device, error) {
	device.Version = expectedVersion + 1
	payload, _ := json.Marshal(device)
	res, err := r.db.ExecContext(ctx, `UPDATE device_records SET version=?,status=?,payload_json=?,updated_at=? WHERE id=? AND version=?`, device.Version, device.Status, string(payload), device.UpdatedAt.UTC().Format(time.RFC3339Nano), device.ID, expectedVersion)
	if err != nil {
		return Device{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, e := r.GetDevice(ctx, device.ID); e != nil {
			return Device{}, e
		}
		return Device{}, domainError(ErrVersionConflict, "device %q changed concurrently", device.ID)
	}
	return device, nil
}

func (r *SQLiteRepository) RecordHeartbeat(ctx context.Context, id string, expectedVersion int64, heartbeat Heartbeat) (Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	device, err := r.GetDevice(ctx, id)
	if err != nil {
		return Device{}, err
	}
	if device.Version != expectedVersion {
		return Device{}, domainError(ErrVersionConflict, "device %q changed concurrently", id)
	}
	device.AgentDockVersion = heartbeat.AgentDockVersion
	device.Capabilities = cloneCapabilities(heartbeat.Capabilities)
	device.LastSeen = ptrTime(heartbeat.ReceivedAt)
	device.Status = StatusOnline
	device.UpdatedAt = heartbeat.ReceivedAt
	device.Version++
	dj, _ := json.Marshal(device)
	hj, _ := json.Marshal(heartbeat)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE device_records SET version=?,status=?,payload_json=?,updated_at=? WHERE id=? AND version=?`, device.Version, device.Status, string(dj), device.UpdatedAt.UTC().Format(time.RFC3339Nano), id, expectedVersion)
	if err != nil {
		return Device{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Device{}, domainError(ErrVersionConflict, "device %q changed concurrently", id)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO device_heartbeats(device_id,payload_json,received_at) VALUES(?,?,?) ON CONFLICT(device_id) DO UPDATE SET payload_json=excluded.payload_json,received_at=excluded.received_at`, id, string(hj), heartbeat.ReceivedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return Device{}, err
	}
	if err = tx.Commit(); err != nil {
		return Device{}, err
	}
	return device, nil
}

func (r *SQLiteRepository) LatestHeartbeat(ctx context.Context, id string) (Heartbeat, bool, error) {
	var payload string
	err := r.db.QueryRowContext(ctx, `SELECT payload_json FROM device_heartbeats WHERE device_id=?`, id).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		if _, getErr := r.GetDevice(ctx, id); getErr != nil {
			return Heartbeat{}, false, getErr
		}
		return Heartbeat{}, false, nil
	}
	if err != nil {
		return Heartbeat{}, false, err
	}
	var hb Heartbeat
	if err := json.Unmarshal([]byte(payload), &hb); err != nil {
		return Heartbeat{}, false, fmt.Errorf("decode heartbeat: %w", err)
	}
	return hb, true, nil
}

func timeText(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}
