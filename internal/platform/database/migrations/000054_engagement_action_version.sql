ALTER TABLE engagements ADD COLUMN action_version BIGINT NOT NULL DEFAULT 0;

UPDATE engagements SET action_version = COALESCE((
    SELECT version FROM engagement_next_actions
    WHERE engagement_next_actions.workspace_id = engagements.workspace_id
      AND engagement_next_actions.engagement_id = engagements.id
), 0);
