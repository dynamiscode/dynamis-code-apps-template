-- Version 10 was SCIM on the pre-merge branch and background jobs on main.
-- Version 12 was files on the pre-merge branch and SCIM on main.
-- Recreate whichever schema a previously applied version skipped.
CREATE TABLE IF NOT EXISTS scim_tokens (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_by_user_id TEXT NOT NULL REFERENCES users(id),
    secret_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX IF NOT EXISTS scim_tokens_workspace_idx
    ON scim_tokens (workspace_id, created_at);

CREATE TABLE IF NOT EXISTS scim_users (
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

CREATE INDEX IF NOT EXISTS scim_users_workspace_name_idx
    ON scim_users (workspace_id, user_id, external_id);

CREATE TABLE IF NOT EXISTS scim_groups (
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
FROM workspace_members
WHERE NOT EXISTS (
    SELECT 1
    FROM scim_users existing
    WHERE existing.workspace_id = workspace_members.workspace_id
      AND existing.external_id = workspace_members.user_id
);

INSERT INTO scim_groups (workspace_id, role, version, created_at)
SELECT w.id, roles.role, 1, w.created_at
FROM workspaces w
JOIN (SELECT 'admin' AS role UNION ALL SELECT 'member' UNION ALL SELECT 'viewer') roles
    ON TRUE
WHERE NOT EXISTS (
    SELECT 1
    FROM scim_groups existing
    WHERE existing.workspace_id = w.id
      AND existing.role = roles.role
);

CREATE TABLE IF NOT EXISTS background_jobs (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    deduplication_key TEXT NOT NULL,
    payload TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    available_at TEXT NOT NULL,
    lease_token TEXT,
    leased_until TEXT,
    started_at TEXT,
    completed_at TEXT,
    last_error TEXT,
    created_at TEXT NOT NULL,
    UNIQUE (workspace_id, kind, deduplication_key)
);

INSERT INTO background_jobs (
    id, workspace_id, kind, deduplication_key, payload, status,
    attempt_count, available_at, created_at
)
SELECT 'webhook-delivery-' || d.id, w.workspace_id, 'webhook.delivery', d.id,
    '{"deliveryId":"' || d.id || '"}', 'pending', d.attempt_count,
    COALESCE(d.next_attempt_at, d.created_at), d.created_at
FROM webhook_deliveries d
JOIN webhooks w ON w.id = d.webhook_id
WHERE d.status = 'pending'
  AND NOT EXISTS (
      SELECT 1
      FROM background_jobs existing
      WHERE existing.workspace_id = w.workspace_id
        AND existing.kind = 'webhook.delivery'
        AND existing.deduplication_key = d.id
  );

CREATE INDEX IF NOT EXISTS background_jobs_due_idx
    ON background_jobs (status, available_at, leased_until, created_at, id);

CREATE INDEX IF NOT EXISTS background_jobs_history_idx
    ON background_jobs (status, completed_at);
