ALTER TABLE engagement_next_actions ADD COLUMN version BIGINT NOT NULL DEFAULT 1;

CREATE TABLE engagement_action_history_new (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL,
    engagement_id BIGINT NOT NULL,
    action_text TEXT NOT NULL,
    due_date TEXT NOT NULL DEFAULT '',
    owner_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('replaced', 'cleared', 'completed', 'reopened')),
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (workspace_id, engagement_id) REFERENCES engagements (workspace_id, id) ON DELETE CASCADE
);
INSERT INTO engagement_action_history_new (id, workspace_id, engagement_id, action_text, due_date, owner_id, outcome, actor_user_id, created_at)
{{history_identity_override}}
SELECT id, workspace_id, engagement_id, action_text, due_date, owner_id, outcome, actor_user_id, created_at FROM engagement_action_history;
{{history_identity_reset}}
DROP TABLE engagement_action_history;
ALTER TABLE engagement_action_history_new RENAME TO engagement_action_history;
CREATE INDEX engagement_action_history_engagement_idx ON engagement_action_history (workspace_id, engagement_id, id);
