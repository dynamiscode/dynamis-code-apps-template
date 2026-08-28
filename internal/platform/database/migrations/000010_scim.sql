CREATE TABLE scim_tokens (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_by_user_id TEXT NOT NULL REFERENCES users(id),
    secret_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX scim_tokens_workspace_idx
    ON scim_tokens (workspace_id, created_at);

CREATE TABLE scim_users (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    active BOOLEAN NOT NULL,
    version BIGINT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (workspace_id, external_id),
    UNIQUE (workspace_id, user_id)
);

CREATE INDEX scim_users_workspace_name_idx
    ON scim_users (workspace_id, user_id, external_id);

CREATE TABLE scim_groups (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
    version BIGINT NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (workspace_id, role)
);

INSERT INTO scim_users (
    workspace_id, external_id, user_id, role, active, version, created_at, updated_at
)
SELECT workspace_id, user_id, user_id, role, TRUE, 1, created_at, created_at
FROM workspace_members;

INSERT INTO scim_groups (workspace_id, role, version, created_at)
SELECT w.id, roles.role, 1, w.created_at
FROM workspaces w
JOIN (SELECT 'admin' AS role UNION ALL SELECT 'member' UNION ALL SELECT 'viewer') roles
    ON TRUE;
