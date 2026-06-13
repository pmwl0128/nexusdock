package artifacts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type SQLiteRepository struct{ db *sql.DB }

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository { return &SQLiteRepository{db: db} }

const artifactColumns = `id,source_kind,source_id,filename,content_type,status,storage_path,cipher_size,cipher_sha256,plain_size,plain_sha256,ephemeral_public_key,upload_token_digest,upload_token_expires_at,upload_token_used_at,expires_at,dispatch_requested,delete_after_all_delivered,conflict_policy,extract_requested,logical_target,created_at,updated_at`
const deliveryColumns = `id,artifact_id,target_device_id,status,wrapped_key,wrap_nonce,download_token_digest,download_token_expires_at,command_id,local_path,error_code,error_message,created_at,updated_at,completed_at`

type rowScanner interface{ Scan(...any) error }

func scanArtifact(row rowScanner) (Artifact, error) {
	var item Artifact
	var status string
	var uploadUsed sql.NullString
	var uploadExpires, expires, created, updated string
	var dispatch, deleteAfter, extract int
	err := row.Scan(&item.ID, &item.SourceKind, &item.SourceID, &item.Filename, &item.ContentType, &status, &item.StoragePath,
		&item.CipherSize, &item.CipherSHA256, &item.PlainSize, &item.PlainSHA256, &item.EphemeralPublicKey,
		&item.UploadTokenDigest, &uploadExpires, &uploadUsed, &expires, &dispatch, &deleteAfter, &item.ConflictPolicy,
		&extract, &item.LogicalTarget, &created, &updated)
	if err != nil {
		return Artifact{}, err
	}
	item.Status = ArtifactStatus(status)
	item.DispatchRequested = dispatch != 0
	item.DeleteAfterAllDelivered = deleteAfter != 0
	item.ExtractRequested = extract != 0
	if item.UploadTokenExpiresAt, err = parseTime(uploadExpires); err != nil {
		return Artifact{}, err
	}
	if item.ExpiresAt, err = parseTime(expires); err != nil {
		return Artifact{}, err
	}
	if item.CreatedAt, err = parseTime(created); err != nil {
		return Artifact{}, err
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return Artifact{}, err
	}
	if uploadUsed.Valid {
		value, parseErr := parseTime(uploadUsed.String)
		if parseErr != nil {
			return Artifact{}, parseErr
		}
		item.UploadTokenUsedAt = &value
	}
	return item, nil
}

func scanDelivery(row rowScanner) (Delivery, error) {
	var item Delivery
	var status string
	var tokenExpiry, completed sql.NullString
	var created, updated string
	err := row.Scan(&item.ID, &item.ArtifactID, &item.TargetDeviceID, &status, &item.WrappedKey, &item.WrapNonce,
		&item.DownloadTokenDigest, &tokenExpiry, &item.CommandID, &item.LocalPath, &item.ErrorCode, &item.ErrorMessage,
		&created, &updated, &completed)
	if err != nil {
		return Delivery{}, err
	}
	item.Status = DeliveryStatus(status)
	var parseErr error
	if item.CreatedAt, parseErr = parseTime(created); parseErr != nil {
		return Delivery{}, parseErr
	}
	if item.UpdatedAt, parseErr = parseTime(updated); parseErr != nil {
		return Delivery{}, parseErr
	}
	if tokenExpiry.Valid {
		value, err := parseTime(tokenExpiry.String)
		if err != nil {
			return Delivery{}, err
		}
		item.DownloadTokenExpiresAt = &value
	}
	if completed.Valid {
		value, err := parseTime(completed.String)
		if err != nil {
			return Delivery{}, err
		}
		item.CompletedAt = &value
	}
	return item, nil
}

func (r *SQLiteRepository) Create(ctx context.Context, artifact Artifact, deliveries []Delivery) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO artifact_records(`+artifactColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		artifact.ID, artifact.SourceKind, artifact.SourceID, artifact.Filename, artifact.ContentType, artifact.Status, artifact.StoragePath,
		artifact.CipherSize, artifact.CipherSHA256, artifact.PlainSize, artifact.PlainSHA256, artifact.EphemeralPublicKey,
		artifact.UploadTokenDigest, formatTime(artifact.UploadTokenExpiresAt), timeValue(artifact.UploadTokenUsedAt), formatTime(artifact.ExpiresAt),
		boolInt(artifact.DispatchRequested), boolInt(artifact.DeleteAfterAllDelivered), artifact.ConflictPolicy, boolInt(artifact.ExtractRequested), artifact.LogicalTarget,
		formatTime(artifact.CreatedAt), formatTime(artifact.UpdatedAt))
	if err != nil {
		return domainError(ErrConflict, "artifact already exists")
	}
	for _, delivery := range deliveries {
		_, err = tx.ExecContext(ctx, `INSERT INTO artifact_deliveries(`+deliveryColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			delivery.ID, delivery.ArtifactID, delivery.TargetDeviceID, delivery.Status, delivery.WrappedKey, delivery.WrapNonce,
			delivery.DownloadTokenDigest, timeValue(delivery.DownloadTokenExpiresAt), delivery.CommandID, delivery.LocalPath,
			delivery.ErrorCode, delivery.ErrorMessage, formatTime(delivery.CreatedAt), formatTime(delivery.UpdatedAt), timeValue(delivery.CompletedAt))
		if err != nil {
			return domainError(ErrConflict, "duplicate artifact delivery")
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) ListArtifacts(ctx context.Context, limit int) ([]Artifact, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+artifactColumns+` FROM artifact_records ORDER BY created_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Artifact{}
	for rows.Next() {
		item, err := scanArtifact(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLiteRepository) GetArtifact(ctx context.Context, id string) (Artifact, error) {
	item, err := scanArtifact(r.db.QueryRowContext(ctx, `SELECT `+artifactColumns+` FROM artifact_records WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Artifact{}, domainError(ErrNotFound, "artifact %q not found", id)
	}
	return item, err
}

func (r *SQLiteRepository) ListDeliveries(ctx context.Context, artifactID string) ([]Delivery, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+deliveryColumns+` FROM artifact_deliveries WHERE artifact_id=? ORDER BY created_at,id`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Delivery{}
	for rows.Next() {
		item, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *SQLiteRepository) GetDelivery(ctx context.Context, id string) (Delivery, error) {
	item, err := scanDelivery(r.db.QueryRowContext(ctx, `SELECT `+deliveryColumns+` FROM artifact_deliveries WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Delivery{}, domainError(ErrDeliveryNotFound, "delivery %q not found", id)
	}
	return item, err
}

func (r *SQLiteRepository) ClaimUpload(ctx context.Context, id, digest string, now time.Time) (Artifact, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE artifact_records SET status=?,updated_at=? WHERE id=? AND status=? AND upload_token_digest=? AND upload_token_expires_at>?`,
		ArtifactUploading, formatTime(now), id, ArtifactPending, digest, formatTime(now))
	if err != nil {
		return Artifact{}, err
	}
	count, _ := res.RowsAffected()
	if count == 1 {
		return r.GetArtifact(ctx, id)
	}
	item, getErr := r.GetArtifact(ctx, id)
	if getErr != nil {
		return Artifact{}, getErr
	}
	switch {
	case item.UploadTokenDigest != digest:
		return Artifact{}, domainError(ErrUploadTokenInvalid, "upload token is invalid")
	case !now.Before(item.UploadTokenExpiresAt):
		return Artifact{}, domainError(ErrUploadTokenExpired, "upload token expired")
	case item.UploadTokenUsedAt != nil || item.Status == ArtifactUploaded:
		return Artifact{}, domainError(ErrUploadAlreadyUsed, "upload token was already used")
	default:
		return Artifact{}, domainError(ErrInvalidState, "artifact cannot begin upload from status %s", item.Status)
	}
}

func (r *SQLiteRepository) AbortUpload(ctx context.Context, id string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE artifact_records SET status=?,updated_at=? WHERE id=? AND status=?`, ArtifactPending, formatTime(now), id, ArtifactUploading)
	return err
}

func (r *SQLiteRepository) FinalizeUpload(ctx context.Context, artifact Artifact, deliveries []Delivery) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE artifact_records SET status=?,cipher_size=?,cipher_sha256=?,plain_size=?,plain_sha256=?,ephemeral_public_key=?,upload_token_used_at=?,updated_at=? WHERE id=? AND status=?`,
		ArtifactUploaded, artifact.CipherSize, artifact.CipherSHA256, artifact.PlainSize, artifact.PlainSHA256,
		artifact.EphemeralPublicKey, timeValue(artifact.UploadTokenUsedAt), formatTime(artifact.UpdatedAt), artifact.ID, ArtifactUploading)
	if err != nil {
		return err
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return domainError(ErrInvalidState, "artifact upload is not claimed")
	}
	for _, delivery := range deliveries {
		res, err = tx.ExecContext(ctx, `UPDATE artifact_deliveries SET wrapped_key=?,wrap_nonce=?,updated_at=? WHERE id=? AND artifact_id=? AND status=?`,
			delivery.WrappedKey, delivery.WrapNonce, formatTime(delivery.UpdatedAt), delivery.ID, artifact.ID, DeliveryPending)
		if err != nil {
			return err
		}
		count, _ = res.RowsAffected()
		if count != 1 {
			return domainError(ErrValidation, "manifest delivery %q is invalid", delivery.ID)
		}
	}
	return tx.Commit()
}

func (r *SQLiteRepository) SetDeliveryQueued(ctx context.Context, id, tokenDigest, commandID string, tokenExpires time.Time, status DeliveryStatus, now time.Time) (Delivery, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE artifact_deliveries SET status=?,download_token_digest=?,download_token_expires_at=?,command_id=?,error_code='',error_message='',updated_at=? WHERE id=? AND status IN (?,?,?)`,
		status, tokenDigest, formatTime(tokenExpires), commandID, formatTime(now), id, DeliveryPending, DeliveryFailed, DeliveryQueued)
	if err != nil {
		return Delivery{}, err
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return Delivery{}, domainError(ErrInvalidState, "delivery %q cannot be dispatched", id)
	}
	return r.GetDelivery(ctx, id)
}

func (r *SQLiteRepository) SetDeliveryResult(ctx context.Context, id string, request DeliveryResultRequest, now time.Time) (Delivery, error) {
	var completed any
	if request.Status.Terminal() {
		completed = formatTime(now)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE artifact_deliveries SET status=?,local_path=?,error_code=?,error_message=?,completed_at=?,updated_at=? WHERE id=? AND status IN (?,?,?,?)`,
		request.Status, request.LocalPath, request.ErrorCode, request.ErrorMessage, completed, formatTime(now), id,
		DeliveryPending, DeliveryQueued, DeliveryDownloading, DeliveryFailed)
	if err != nil {
		return Delivery{}, err
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return Delivery{}, domainError(ErrInvalidState, "delivery %q cannot accept result", id)
	}
	return r.GetDelivery(ctx, id)
}

func (r *SQLiteRepository) MarkDeliveryDownloading(ctx context.Context, id string, now time.Time) (Delivery, error) {
	_, err := r.db.ExecContext(ctx, `UPDATE artifact_deliveries SET status=?,updated_at=? WHERE id=? AND status=?`, DeliveryDownloading, formatTime(now), id, DeliveryQueued)
	if err != nil {
		return Delivery{}, err
	}
	return r.GetDelivery(ctx, id)
}

func (r *SQLiteRepository) ExpireBefore(ctx context.Context, now time.Time) ([]Artifact, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT `+artifactColumns+` FROM artifact_records WHERE status IN (?,?,?) AND expires_at<=?`, ArtifactPending, ArtifactUploading, ArtifactUploaded, formatTime(now))
	if err != nil {
		return nil, err
	}
	items := []Artifact{}
	for rows.Next() {
		item, err := scanArtifact(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx, `UPDATE artifact_records SET status=?,updated_at=? WHERE id=?`, ArtifactExpired, formatTime(now), item.ID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE artifact_deliveries SET status=?,completed_at=?,updated_at=? WHERE artifact_id=? AND status NOT IN (?,?,?,?)`,
			DeliveryExpired, formatTime(now), formatTime(now), item.ID, DeliveryCompleted, DeliveryFailed, DeliveryExpired, DeliveryCancelled); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *SQLiteRepository) MarkArtifactDeleted(ctx context.Context, id string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE artifact_records SET status=?,updated_at=? WHERE id=?`, ArtifactDeleted, formatTime(now), id)
	return err
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse artifact time: %w", err)
	}
	return parsed, nil
}
func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
func timeValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
