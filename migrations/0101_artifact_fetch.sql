CREATE TABLE IF NOT EXISTS artifact_fetch_jobs (
    id TEXT PRIMARY KEY,
    requester_device_id TEXT NOT NULL,
    source_device_id TEXT NOT NULL,
    source_path TEXT NOT NULL,
    archive_requested INTEGER NOT NULL,
    status TEXT NOT NULL,
    filename TEXT NOT NULL DEFAULT '',
    content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    storage_path TEXT NOT NULL,
    receiver_public_key TEXT NOT NULL,
    ephemeral_public_key TEXT NOT NULL DEFAULT '',
    wrapped_key TEXT NOT NULL DEFAULT '',
    wrap_nonce TEXT NOT NULL DEFAULT '',
    plain_size INTEGER NOT NULL DEFAULT 0,
    plain_sha256 TEXT NOT NULL DEFAULT '',
    cipher_size INTEGER NOT NULL DEFAULT 0,
    cipher_sha256 TEXT NOT NULL DEFAULT '',
    upload_token_digest TEXT NOT NULL,
    upload_token_expires_at TEXT NOT NULL,
    upload_token_used_at TEXT,
    download_token_digest TEXT NOT NULL,
    download_token_expires_at TEXT NOT NULL,
    command_id TEXT NOT NULL DEFAULT '',
    listing_json TEXT NOT NULL DEFAULT '[]',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    mounted_at TEXT,
    FOREIGN KEY (requester_device_id) REFERENCES device_records(id) ON DELETE RESTRICT,
    FOREIGN KEY (source_device_id) REFERENCES device_records(id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_artifact_fetch_requester_status
    ON artifact_fetch_jobs(requester_device_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_artifact_fetch_source_status
    ON artifact_fetch_jobs(source_device_id, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_artifact_fetch_expiry
    ON artifact_fetch_jobs(status, expires_at);
