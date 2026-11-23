CREATE TABLE catalog_entries (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    duration_minutes BIGINT,
    archived_at TEXT
);

CREATE INDEX catalog_entries_workspace_idx ON catalog_entries (workspace_id, id);
