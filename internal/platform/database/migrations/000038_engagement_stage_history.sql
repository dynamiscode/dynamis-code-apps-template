CREATE UNIQUE INDEX engagements_workspace_key_idx ON engagements (workspace_id, id);

CREATE TABLE engagement_stage_history (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    engagement_id BIGINT NOT NULL,
    actor_user_id BIGINT NOT NULL,
    from_stage_id BIGINT NOT NULL,
    to_stage_id BIGINT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (workspace_id, engagement_id) REFERENCES engagements (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, from_stage_id) REFERENCES engagement_stages (workspace_id, id),
    FOREIGN KEY (workspace_id, to_stage_id) REFERENCES engagement_stages (workspace_id, id)
);
CREATE INDEX engagement_stage_history_engagement_idx ON engagement_stage_history (workspace_id, engagement_id, id);
