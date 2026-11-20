-- Saved views: a named, prefiltered way of looking at a list. A personal view belongs to its creator. A shared view
-- belongs to the workspace. The state holds the list query and the column settings a view applies.
CREATE TABLE IF NOT EXISTS list_views (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    list_key TEXT NOT NULL,
    name TEXT NOT NULL,
    shared BOOLEAN NOT NULL,
    state TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS list_views_workspace_list_idx
    ON list_views (workspace_id, list_key, shared, id);

-- A member's default view per list. Without a row, the list opens in its Active view.
CREATE TABLE IF NOT EXISTS list_view_defaults (
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    list_key TEXT NOT NULL,
    view_id BIGINT NOT NULL REFERENCES list_views(id) ON DELETE CASCADE,
    PRIMARY KEY (workspace_id, user_id, list_key)
);
