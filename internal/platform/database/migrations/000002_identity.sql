CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE external_identities (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    email TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (issuer, subject)
);

CREATE INDEX external_identities_user_idx
    ON external_identities (user_id);

CREATE TABLE instance_admins (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL
);

CREATE TABLE workspaces (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE workspace_members (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at TEXT NOT NULL,
    PRIMARY KEY (workspace_id, user_id)
);

CREATE INDEX workspace_members_user_idx
    ON workspace_members (user_id, workspace_id);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    secret_hash TEXT NOT NULL UNIQUE,
    csrf_hash TEXT NOT NULL,
    auth_method TEXT NOT NULL CHECK (auth_method IN ('local', 'oidc')),
    oidc_provider_id TEXT,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    CHECK (
        (auth_method = 'local' AND oidc_provider_id IS NULL) OR
        (auth_method = 'oidc' AND oidc_provider_id IS NOT NULL)
    )
);

CREATE INDEX sessions_user_idx
    ON sessions (user_id, created_at);

CREATE TABLE invitations (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    invited_by_user_id TEXT NOT NULL REFERENCES users(id),
    email TEXT NOT NULL,
    active_email TEXT,
    role TEXT NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
    secret_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    accepted_at TEXT,
    expired_at TEXT,
    revoked_at TEXT,
    UNIQUE (workspace_id, active_email)
);

CREATE INDEX invitations_workspace_idx
    ON invitations (workspace_id, created_at);

CREATE TABLE api_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    secret_hash TEXT NOT NULL UNIQUE,
    scopes TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT,
    last_used_at TEXT,
    revoked_at TEXT
);

CREATE INDEX api_tokens_user_idx
    ON api_tokens (user_id, created_at);
CREATE INDEX api_tokens_workspace_idx
    ON api_tokens (workspace_id, user_id);

CREATE TABLE oidc_transactions (
    state_hash TEXT PRIMARY KEY,
    provider_id TEXT NOT NULL,
    browser_session_hash TEXT NOT NULL,
    pkce_verifier_hash TEXT NOT NULL,
    nonce_hash TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT
);

CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    actor_user_id TEXT,
    auth_method TEXT NOT NULL,
    workspace_id TEXT,
    target_type TEXT NOT NULL,
    target_id TEXT,
    action TEXT NOT NULL,
    outcome TEXT NOT NULL,
    request_id TEXT,
    source_address TEXT,
    metadata TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX audit_events_workspace_idx
    ON audit_events (workspace_id, created_at);
CREATE INDEX audit_events_actor_idx
    ON audit_events (actor_user_id, created_at);

CREATE TABLE bootstrap_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    completed_at TEXT NOT NULL
);
