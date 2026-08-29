ALTER TABLE sessions ADD COLUMN auth_level INTEGER NOT NULL DEFAULT 1;
ALTER TABLE sessions ADD COLUMN fresh_at TEXT;
UPDATE sessions SET fresh_at = created_at WHERE fresh_at IS NULL;

CREATE TABLE mfa_challenges (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL CHECK (purpose IN ('login', 'passkey_enrollment', 'totp_enrollment')),
    session_id TEXT REFERENCES sessions(id) ON DELETE CASCADE,
    auth_method TEXT NOT NULL DEFAULT 'local',
    oidc_provider_id TEXT,
    webauthn_session_json TEXT,
    encrypted_secret TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT
);

CREATE INDEX mfa_challenges_user_idx ON mfa_challenges (user_id, created_at);

CREATE TABLE mfa_totp (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    encrypted_secret TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE mfa_recovery_codes (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    used_at TEXT
);

CREATE INDEX mfa_recovery_codes_user_idx ON mfa_recovery_codes (user_id, used_at);

CREATE TABLE mfa_passkeys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id TEXT NOT NULL UNIQUE,
    credential_json TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_used_at TEXT,
    revoked_at TEXT
);

CREATE INDEX mfa_passkeys_user_idx ON mfa_passkeys (user_id, revoked_at, created_at);
