-- Explicit column visibility deviations, kept separately for each member,
-- workspace, and list. Declared list defaults remain in application code.
CREATE TABLE list_column_preferences (
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    list_key TEXT NOT NULL,
    column_key TEXT NOT NULL,
    visible BOOLEAN NOT NULL,
    PRIMARY KEY (workspace_id, user_id, list_key, column_key)
);

CREATE INDEX list_column_preferences_workspace_list_column_idx
    ON list_column_preferences (workspace_id, list_key, column_key);
