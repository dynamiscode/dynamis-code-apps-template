CREATE TABLE webhooks (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    secret_ciphertext TEXT NOT NULL,
    events TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX webhooks_workspace_idx
    ON webhooks (workspace_id, created_at, id);

CREATE TABLE webhook_deliveries (
    id TEXT PRIMARY KEY,
    webhook_id TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL CHECK (status IN ('pending', 'delivered', 'failed')),
    next_attempt_at TEXT,
    last_status_code INTEGER,
    last_error TEXT,
    created_at TEXT NOT NULL,
    delivered_at TEXT,
    UNIQUE (webhook_id, event_id)
);

CREATE INDEX webhook_deliveries_due_idx
    ON webhook_deliveries (status, next_attempt_at, created_at, id);
CREATE INDEX webhook_deliveries_webhook_idx
    ON webhook_deliveries (webhook_id, created_at, id);
