CREATE TABLE IF NOT EXISTS files (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    object_key TEXT NOT NULL UNIQUE,
    original_name TEXT NOT NULL,
    detected_mime TEXT,
    size BIGINT NOT NULL,
    sha256 TEXT,
    status TEXT NOT NULL CHECK (status IN ('pending', 'ready', 'failed')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS files_workspace_created_idx
    ON files (workspace_id, created_at, id);
