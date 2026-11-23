CREATE UNIQUE INDEX engagements_workspace_key_idx ON engagements (workspace_id, id);
CREATE TABLE engagement_next_actions (
    workspace_id BIGINT NOT NULL,
    engagement_id BIGINT NOT NULL,
    action_text TEXT NOT NULL,
    due_date TEXT NOT NULL DEFAULT '',
    owner_id BIGINT NOT NULL REFERENCES users(id),
    PRIMARY KEY (workspace_id, engagement_id),
    FOREIGN KEY (workspace_id, engagement_id) REFERENCES engagements (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX engagement_next_actions_due_idx ON engagement_next_actions (workspace_id, due_date);
