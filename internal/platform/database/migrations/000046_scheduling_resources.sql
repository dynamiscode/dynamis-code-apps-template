ALTER TABLE workspaces ADD COLUMN time_zone TEXT NOT NULL DEFAULT 'UTC';

CREATE TABLE scheduling_resources (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX scheduling_resources_workspace_idx ON scheduling_resources (workspace_id, id);
