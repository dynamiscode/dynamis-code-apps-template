CREATE TABLE engagement_stages (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('open', 'won', 'lost')),
    position BIGINT NOT NULL,
    archived_at TEXT,
    UNIQUE (workspace_id, position)
);
