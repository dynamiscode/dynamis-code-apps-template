-- Clients hold the four common fields and a JSON text column for custom values.
-- Custom-field slot indexes arrive with the custom-field migration (ADR 0012).
CREATE TABLE IF NOT EXISTS clients (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    display_name TEXT NOT NULL,
    email TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    archived_at TEXT,
    custom_values TEXT NOT NULL DEFAULT '{}',
    version BIGINT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS clients_workspace_id_idx
    ON clients (workspace_id, id);
