-- The order of each member's list columns, kept separately for each workspace and list. A list's first column
-- (its name) is always first and has no row here. Declared list defaults remain in application code.
CREATE TABLE list_column_orders (
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    list_key TEXT NOT NULL,
    column_key TEXT NOT NULL,
    sort_position INTEGER NOT NULL,
    PRIMARY KEY (workspace_id, user_id, list_key, column_key)
);

CREATE INDEX list_column_orders_workspace_list_position_idx
    ON list_column_orders (workspace_id, list_key, sort_position);
