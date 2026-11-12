-- Files take integer keys; their public ID is the ADR 0009 short ID. The
-- table is rebuilt because SQLite and PostgreSQL cannot change a primary key
-- type in place. object_key is copied unchanged, so stored objects stay
-- addressable. Rows keep their creation order in the new key sequence.
CREATE TABLE files_next (
    id {{file_id_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
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

INSERT INTO files_next (workspace_id, owner_user_id, object_key, original_name, detected_mime,
    size, sha256, status, created_at, updated_at)
SELECT workspace_id, owner_user_id, object_key, original_name, detected_mime,
    size, sha256, status, created_at, updated_at
FROM files
ORDER BY created_at, id;

DROP TABLE files;

ALTER TABLE files_next RENAME TO files;

CREATE INDEX IF NOT EXISTS files_workspace_created_idx
    ON files (workspace_id, created_at, id);
