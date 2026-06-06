CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_user_id TEXT,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS devices (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_user_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    labels_json TEXT NOT NULL DEFAULT '{}',
    last_seen_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'inbox',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS skills (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS auth_tokens (
    id TEXT PRIMARY KEY,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('user', 'agent', 'device', 'system')),
    subject_id TEXT NOT NULL,
    token_kind TEXT NOT NULL CHECK (token_kind IN ('session', 'agent_token', 'device_token', 'system_token')),
    token_hash TEXT NOT NULL UNIQUE,
    scopes_json TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT,
    revoked_at TEXT,
    revoked_by_type TEXT,
    revoked_by_id TEXT
);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_subject ON auth_tokens(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_active ON auth_tokens(token_hash, revoked_at, expires_at);

CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled')),
    actor_type TEXT NOT NULL CHECK (actor_type IN ('user', 'agent', 'device', 'system')),
    actor_id TEXT NOT NULL,
    device_id TEXT,
    skill_id TEXT,
    task_id TEXT,
    idempotency_key TEXT,
    input_json TEXT NOT NULL DEFAULT '{}',
    output_json TEXT,
    error_code TEXT,
    error_message TEXT,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE SET NULL,
    FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE SET NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_idempotency
    ON runs(actor_type, actor_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_runs_status_created ON runs(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_device_created ON runs(device_id, created_at DESC);

CREATE TABLE IF NOT EXISTS run_steps (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'skipped')),
    input_json TEXT NOT NULL DEFAULT '{}',
    output_json TEXT,
    error_code TEXT,
    error_message TEXT,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (run_id, sequence),
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_run_steps_run ON run_steps(run_id, sequence);

CREATE TABLE IF NOT EXISTS run_evidence (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    step_id TEXT,
    kind TEXT NOT NULL,
    uri TEXT,
    media_type TEXT,
    digest TEXT,
    payload_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
    FOREIGN KEY (step_id) REFERENCES run_steps(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_run_evidence_run ON run_evidence(run_id, created_at);

CREATE TABLE IF NOT EXISTS run_verifications (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('passed', 'failed', 'skipped')),
    summary TEXT NOT NULL DEFAULT '',
    evidence_id TEXT,
    created_at TEXT NOT NULL,
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
    FOREIGN KEY (evidence_id) REFERENCES run_evidence(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_run_verifications_run ON run_verifications(run_id, created_at);

CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT PRIMARY KEY,
    occurred_at TEXT NOT NULL,
    actor_type TEXT NOT NULL CHECK (actor_type IN ('user', 'agent', 'device', 'system')),
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    object_type TEXT NOT NULL,
    object_id TEXT NOT NULL,
    result TEXT NOT NULL,
    risk TEXT NOT NULL DEFAULT 'low',
    approval TEXT NOT NULL DEFAULT 'not_required',
    run_id TEXT,
    request_id TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_audit_events_time ON audit_events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_object ON audit_events(object_type, object_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events(actor_type, actor_id, occurred_at DESC);

CREATE TRIGGER IF NOT EXISTS audit_events_no_update
BEFORE UPDATE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events are append-only');
END;

CREATE TRIGGER IF NOT EXISTS audit_events_no_delete
BEFORE DELETE ON audit_events
BEGIN
    SELECT RAISE(ABORT, 'audit_events are append-only');
END;
