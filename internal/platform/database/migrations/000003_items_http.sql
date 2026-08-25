CREATE TABLE items (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_by_user_id TEXT NOT NULL REFERENCES users(id),
    title TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'complete')),
    version BIGINT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX items_workspace_created_idx
    ON items (workspace_id, created_at, id);
CREATE INDEX items_workspace_status_created_idx
    ON items (workspace_id, status, created_at, id);

CREATE TABLE idempotency_records (
    key_hash TEXT NOT NULL,
    principal_id TEXT NOT NULL,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    operation TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    result_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (key_hash, principal_id, workspace_id, operation)
);

CREATE INDEX idempotency_records_expiry_idx
    ON idempotency_records (expires_at);
