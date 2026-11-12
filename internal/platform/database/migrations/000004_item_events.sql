-- Event keys are never reused (AUTOINCREMENT on SQLite, identity on PostgreSQL),
-- so a change-feed cursor for a pruned event cannot resolve to a newer one.
CREATE TABLE item_events (
    id {{file_id_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    item_id BIGINT NOT NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('created', 'updated', 'deleted')),
    item_version BIGINT NOT NULL,
    occurred_at TEXT NOT NULL
);

CREATE INDEX item_events_workspace_order_idx
    ON item_events (workspace_id, occurred_at, id);
