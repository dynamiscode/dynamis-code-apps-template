CREATE TABLE client_timeline_entries (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    client_id BIGINT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    author_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    entry_type TEXT NOT NULL,
    channel TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL,
    corrects_id BIGINT REFERENCES client_timeline_entries(id),
    created_at TEXT NOT NULL
);

CREATE INDEX client_timeline_entries_order_idx
    ON client_timeline_entries (workspace_id, client_id, created_at, id);
