-- Client change feed for realtime list refresh (ADR 0013). Event keys are never
-- reused, so a cursor for a pruned event cannot resolve to a newer one. The event
-- type has no CHECK constraint so later lifecycle events add no table rebuild.
CREATE TABLE client_events (
    id {{file_id_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    client_id BIGINT NOT NULL,
    event_type TEXT NOT NULL,
    client_version BIGINT NOT NULL,
    occurred_at TEXT NOT NULL
);

CREATE INDEX client_events_workspace_order_idx
    ON client_events (workspace_id, occurred_at, id);
