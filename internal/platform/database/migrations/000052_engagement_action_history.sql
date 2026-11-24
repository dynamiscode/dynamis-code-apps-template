CREATE TABLE engagement_action_history (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL,
    engagement_id BIGINT NOT NULL,
    action_text TEXT NOT NULL,
    due_date TEXT NOT NULL DEFAULT '',
    owner_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('replaced', 'cleared')),
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (workspace_id, engagement_id) REFERENCES engagements (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX engagement_action_history_engagement_idx ON engagement_action_history (workspace_id, engagement_id, id);
