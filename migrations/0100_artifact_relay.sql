CREATE TABLE IF NOT EXISTS artifact_records (
    id TEXT PRIMARY KEY,
    source_kind TEXT NOT NULL,
    source_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    status TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    cipher_size INTEGER NOT NULL DEFAULT 0,
    cipher_sha256 TEXT NOT NULL DEFAULT '',
    plain_size INTEGER NOT NULL DEFAULT 0,
    plain_sha256 TEXT NOT NULL DEFAULT '',
    ephemeral_public_key TEXT NOT NULL DEFAULT '',
    upload_token_digest TEXT NOT NULL,
    upload_token_expires_at TEXT NOT NULL,
    upload_token_used_at TEXT,
    expires_at TEXT NOT NULL,
    dispatch_requested INTEGER NOT NULL,
    delete_after_all_delivered INTEGER NOT NULL,
    conflict_policy TEXT NOT NULL,
    extract_requested INTEGER NOT NULL,
    logical_target TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_artifact_records_status_expires
    ON artifact_records(status, expires_at);

CREATE TABLE IF NOT EXISTS artifact_deliveries (
    id TEXT PRIMARY KEY,
    artifact_id TEXT NOT NULL,
    target_device_id TEXT NOT NULL,
    status TEXT NOT NULL,
    wrapped_key TEXT NOT NULL DEFAULT '',
    wrap_nonce TEXT NOT NULL DEFAULT '',
    download_token_digest TEXT NOT NULL DEFAULT '',
    download_token_expires_at TEXT,
    command_id TEXT NOT NULL DEFAULT '',
    local_path TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT,
    UNIQUE(artifact_id, target_device_id),
    FOREIGN KEY (artifact_id) REFERENCES artifact_records(id) ON DELETE CASCADE,
    FOREIGN KEY (target_device_id) REFERENCES device_records(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_artifact_deliveries_artifact_status
    ON artifact_deliveries(artifact_id, status);
CREATE INDEX IF NOT EXISTS idx_artifact_deliveries_target_status
    ON artifact_deliveries(target_device_id, status, updated_at);
