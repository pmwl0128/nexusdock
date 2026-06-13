package artifacts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const fetchColumns = `id,requester_device_id,source_device_id,source_path,archive_requested,status,filename,content_type,storage_path,receiver_public_key,ephemeral_public_key,wrapped_key,wrap_nonce,plain_size,plain_sha256,cipher_size,cipher_sha256,upload_token_digest,upload_token_expires_at,upload_token_used_at,download_token_digest,download_token_expires_at,command_id,listing_json,error_code,error_message,expires_at,created_at,updated_at,mounted_at`

func scanFetch(row rowScanner) (FetchJob, error) {
	var item FetchJob
	var archive int
	var status, uploadExpiry, downloadExpiry, expires, created, updated, listing string
	var uploadUsed, mounted sql.NullString
	if err := row.Scan(
		&item.ID, &item.RequesterDeviceID, &item.SourceDeviceID, &item.SourcePath, &archive, &status,
		&item.Filename, &item.ContentType, &item.StoragePath, &item.ReceiverPublicKey, &item.EphemeralPublicKey,
		&item.WrappedKey, &item.WrapNonce, &item.PlainSize, &item.PlainSHA256, &item.CipherSize, &item.CipherSHA256,
		&item.UploadTokenDigest, &uploadExpiry, &uploadUsed, &item.DownloadTokenDigest, &downloadExpiry,
		&item.CommandID, &listing, &item.ErrorCode, &item.ErrorMessage, &expires, &created, &updated, &mounted,
	); err != nil {
		return FetchJob{}, err
	}
	item.ArchiveRequested = archive != 0
	item.Status = FetchStatus(status)
	var err error
	if item.UploadTokenExpiresAt, err = parseTime(uploadExpiry); err != nil {
		return FetchJob{}, err
	}
	if item.DownloadTokenExpiresAt, err = parseTime(downloadExpiry); err != nil {
		return FetchJob{}, err
	}
	if item.ExpiresAt, err = parseTime(expires); err != nil {
		return FetchJob{}, err
	}
	if item.CreatedAt, err = parseTime(created); err != nil {
		return FetchJob{}, err
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return FetchJob{}, err
	}
	if uploadUsed.Valid {
		value, e := parseTime(uploadUsed.String)
		if e != nil {
			return FetchJob{}, e
		}
		item.UploadTokenUsedAt = &value
	}
	if mounted.Valid {
		value, e := parseTime(mounted.String)
		if e != nil {
			return FetchJob{}, e
		}
		item.MountedAt = &value
	}
	if listing != "" {
		if err := json.Unmarshal([]byte(listing), &item.Listing); err != nil {
			return FetchJob{}, err
		}
	}
	return item, nil
}

func (r *SQLiteRepository) CreateFetch(ctx context.Context, item FetchJob) error {
	listing, err := json.Marshal(item.Listing)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO artifact_fetch_jobs(`+fetchColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		item.ID, item.RequesterDeviceID, item.SourceDeviceID, item.SourcePath, boolInt(item.ArchiveRequested), item.Status,
		item.Filename, item.ContentType, item.StoragePath, item.ReceiverPublicKey, item.EphemeralPublicKey,
		item.WrappedKey, item.WrapNonce, item.PlainSize, item.PlainSHA256, item.CipherSize, item.CipherSHA256,
		item.UploadTokenDigest, formatTime(item.UploadTokenExpiresAt), timeValue(item.UploadTokenUsedAt),
		item.DownloadTokenDigest, formatTime(item.DownloadTokenExpiresAt), item.CommandID, string(listing),
		item.ErrorCode, item.ErrorMessage, formatTime(item.ExpiresAt), formatTime(item.CreatedAt), formatTime(item.UpdatedAt), timeValue(item.MountedAt))
	if err != nil {
		return domainError(ErrConflict, "artifact fetch already exists")
	}
	return nil
}

func (r *SQLiteRepository) GetFetch(ctx context.Context, id string) (FetchJob, error) {
	item, err := scanFetch(r.db.QueryRowContext(ctx, `SELECT `+fetchColumns+` FROM artifact_fetch_jobs WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return FetchJob{}, domainError(ErrFetchNotFound, "artifact fetch %q not found", id)
	}
	return item, err
}

func (r *SQLiteRepository) SetFetchCommand(ctx context.Context, id, commandID string, status FetchStatus, now time.Time) (FetchJob, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE artifact_fetch_jobs SET command_id=?,status=?,updated_at=? WHERE id=? AND status=?`, commandID, status, formatTime(now), id, FetchPending)
	if err != nil {
		return FetchJob{}, err
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return FetchJob{}, domainError(ErrInvalidState, "fetch %q cannot be queued", id)
	}
	return r.GetFetch(ctx, id)
}

func (r *SQLiteRepository) ClaimFetchUpload(ctx context.Context, id, sourceDeviceID, digest string, now time.Time) (FetchJob, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE artifact_fetch_jobs SET status=?,updated_at=? WHERE id=? AND source_device_id=? AND status IN (?,?) AND upload_token_digest=? AND upload_token_expires_at>?`,
		FetchUploading, formatTime(now), id, sourceDeviceID, FetchQueued, FetchListing, digest, formatTime(now))
	if err != nil {
		return FetchJob{}, err
	}
	count, _ := res.RowsAffected()
	if count == 1 {
		return r.GetFetch(ctx, id)
	}
	item, getErr := r.GetFetch(ctx, id)
	if getErr != nil {
		return FetchJob{}, getErr
	}
	if item.SourceDeviceID != sourceDeviceID {
		return FetchJob{}, domainError(ErrFetchDeviceMismatch, "fetch does not belong to source device")
	}
	if item.UploadTokenDigest != digest {
		return FetchJob{}, domainError(ErrFetchTokenInvalid, "fetch upload token is invalid")
	}
	if !now.Before(item.UploadTokenExpiresAt) {
		return FetchJob{}, domainError(ErrFetchTokenExpired, "fetch upload token expired")
	}
	return FetchJob{}, domainError(ErrInvalidState, "fetch cannot begin upload from status %s", item.Status)
}

func (r *SQLiteRepository) AbortFetchUpload(ctx context.Context, id string, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE artifact_fetch_jobs SET status=?,updated_at=? WHERE id=? AND status=?`, FetchQueued, formatTime(now), id, FetchUploading)
	return err
}

func (r *SQLiteRepository) CompleteFetchUpload(ctx context.Context, item FetchJob) error {
	res, err := r.db.ExecContext(ctx, `UPDATE artifact_fetch_jobs SET status=?,filename=?,content_type=?,ephemeral_public_key=?,wrapped_key=?,wrap_nonce=?,plain_size=?,plain_sha256=?,cipher_size=?,cipher_sha256=?,upload_token_used_at=?,updated_at=? WHERE id=? AND status=?`,
		FetchReady, item.Filename, item.ContentType, item.EphemeralPublicKey, item.WrappedKey, item.WrapNonce,
		item.PlainSize, item.PlainSHA256, item.CipherSize, item.CipherSHA256, timeValue(item.UploadTokenUsedAt), formatTime(item.UpdatedAt), item.ID, FetchUploading)
	if err != nil {
		return err
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return domainError(ErrInvalidState, "fetch upload is not claimed")
	}
	return nil
}

func (r *SQLiteRepository) SetFetchResult(ctx context.Context, id, sourceDeviceID string, request FetchResultRequest, now time.Time) (FetchJob, error) {
	listing, err := json.Marshal(request.Listing)
	if err != nil {
		return FetchJob{}, err
	}
	res, err := r.db.ExecContext(ctx, `UPDATE artifact_fetch_jobs SET status=?,listing_json=?,error_code=?,error_message=?,updated_at=? WHERE id=? AND source_device_id=? AND status IN (?,?,?)`,
		request.Status, string(listing), request.ErrorCode, request.ErrorMessage, formatTime(now), id, sourceDeviceID, FetchQueued, FetchListing, FetchUploading)
	if err != nil {
		return FetchJob{}, err
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return FetchJob{}, domainError(ErrInvalidState, "fetch %q cannot accept result", id)
	}
	return r.GetFetch(ctx, id)
}

func (r *SQLiteRepository) MarkFetchMounted(ctx context.Context, id, requesterDeviceID string, now time.Time) (FetchJob, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE artifact_fetch_jobs SET status=?,mounted_at=?,updated_at=? WHERE id=? AND requester_device_id=? AND status=?`,
		FetchMounted, formatTime(now), formatTime(now), id, requesterDeviceID, FetchReady)
	if err != nil {
		return FetchJob{}, err
	}
	count, _ := res.RowsAffected()
	if count != 1 {
		return FetchJob{}, domainError(ErrInvalidState, "fetch %q cannot be mounted", id)
	}
	return r.GetFetch(ctx, id)
}

func (r *SQLiteRepository) ExpireFetches(ctx context.Context, now time.Time) ([]FetchJob, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+fetchColumns+` FROM artifact_fetch_jobs WHERE status NOT IN (?,?,?) AND expires_at<=?`, FetchMounted, FetchFailed, FetchExpired, formatTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []FetchJob{}
	for rows.Next() {
		item, err := scanFetch(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, item := range items {
		if _, err := r.db.ExecContext(ctx, `UPDATE artifact_fetch_jobs SET status=?,updated_at=? WHERE id=?`, FetchExpired, formatTime(now), item.ID); err != nil {
			return nil, err
		}
	}
	return items, nil
}
