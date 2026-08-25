CREATE TABLE item_events (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    item_id TEXT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('created', 'updated', 'deleted')),
    item_version BIGINT NOT NULL,
    occurred_at TEXT NOT NULL
);

CREATE INDEX item_events_workspace_order_idx
    ON item_events (workspace_id, occurred_at, id);
