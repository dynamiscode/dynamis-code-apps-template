CREATE TABLE background_jobs (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    deduplication_key TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    available_at TEXT NOT NULL,
    lease_token TEXT,
    leased_until TEXT,
    started_at TEXT,
    completed_at TEXT,
    last_error TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (workspace_id, kind, deduplication_key)
);

INSERT INTO background_jobs (
    id, workspace_id, kind, deduplication_key, payload, status,
    attempt_count, available_at, created_at
)
SELECT 'webhook-delivery-' || d.id, w.workspace_id, 'webhook.delivery', d.id,
    '{"deliveryId":"' || d.id || '"}', 'pending', d.attempt_count,
    COALESCE(d.next_attempt_at, d.created_at), d.created_at
FROM webhook_deliveries d
JOIN webhooks w ON w.id = d.webhook_id
WHERE d.status = 'pending';

CREATE INDEX background_jobs_due_idx
    ON background_jobs (status, available_at, leased_until, created_at, id);

CREATE INDEX background_jobs_history_idx
    ON background_jobs (status, completed_at);
