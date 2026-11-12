CREATE TABLE oauth_clients (
    id {{integer_pk}},
    client_id TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    redirect_uris TEXT NOT NULL,
    created_at TEXT NOT NULL,
    deleted_at TEXT
);

CREATE TABLE oauth_grants (
    id {{integer_pk}},
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    client_id BIGINT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    scopes TEXT NOT NULL,
    auth_level INTEGER NOT NULL CHECK (auth_level IN (1, 2)),
    created_at TEXT NOT NULL,
    revoked_at TEXT,
    UNIQUE (user_id, workspace_id, client_id)
);

CREATE INDEX oauth_grants_session_idx ON oauth_grants (session_id);

CREATE TABLE oauth_tokens (
    id {{integer_pk}},
    grant_id BIGINT NOT NULL REFERENCES oauth_grants(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('code', 'access', 'refresh')),
    secret_hash TEXT NOT NULL UNIQUE,
    client_id BIGINT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    redirect_uri TEXT,
    code_challenge TEXT,
    resource TEXT,
    parent_id BIGINT,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT,
    revoked_at TEXT
);

CREATE INDEX oauth_tokens_grant_idx ON oauth_tokens (grant_id);
CREATE INDEX oauth_tokens_expiry_idx ON oauth_tokens (expires_at);
