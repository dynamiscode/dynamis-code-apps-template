CREATE TABLE engagement_labels (
    workspace_id BIGINT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    label TEXT NOT NULL
);
