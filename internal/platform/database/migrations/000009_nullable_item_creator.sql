CREATE TABLE items_account_deletion (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'complete')),
    version BIGINT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO items_account_deletion (
    id, workspace_id, created_by_user_id, title, status,
    version, created_at, updated_at
)
SELECT id, workspace_id, created_by_user_id, title, status,
    version, created_at, updated_at
FROM items;

DROP TABLE items;
ALTER TABLE items_account_deletion RENAME TO items;

CREATE INDEX items_workspace_created_idx
    ON items (workspace_id, created_at, id);
CREATE INDEX items_workspace_status_created_idx
    ON items (workspace_id, status, created_at, id);
