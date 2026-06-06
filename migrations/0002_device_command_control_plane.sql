CREATE TABLE IF NOT EXISTS device_enrollment_tokens (
    digest TEXT PRIMARY KEY,
    payload_json TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at TEXT
);
CREATE INDEX IF NOT EXISTS idx_device_enrollment_tokens_expires_at
    ON device_enrollment_tokens(expires_at);

CREATE TABLE IF NOT EXISTS device_records (
    id TEXT PRIMARY KEY,
    version INTEGER NOT NULL,
    status TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_device_records_status
    ON device_records(status);

CREATE TABLE IF NOT EXISTS device_heartbeats (
    device_id TEXT PRIMARY KEY,
    payload_json TEXT NOT NULL,
    received_at TEXT NOT NULL,
    FOREIGN KEY (device_id) REFERENCES device_records(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS device_commands_v1 (
    id TEXT PRIMARY KEY,
    device_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    status TEXT NOT NULL,
    priority INTEGER NOT NULL,
    not_before TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    lease_expires_at TEXT,
    version INTEGER NOT NULL,
    payload_json TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(device_id, idempotency_key),
    FOREIGN KEY (device_id) REFERENCES device_records(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_device_commands_v1_device_status
    ON device_commands_v1(device_id, status, priority DESC, not_before, expires_at);
