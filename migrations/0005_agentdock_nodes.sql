CREATE TABLE agentdock_nodes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    endpoint TEXT NOT NULL UNIQUE,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    timeout_seconds INTEGER NOT NULL DEFAULT 8 CHECK (timeout_seconds BETWEEN 1 AND 300),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE agentdock_node_secrets (
    node_id TEXT PRIMARY KEY,
    token_ciphertext BLOB NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (node_id) REFERENCES agentdock_nodes(id) ON DELETE CASCADE
);
