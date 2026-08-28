ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC';
ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT 'system';
ALTER TABLE users ADD COLUMN email_verified_at TEXT;

CREATE TABLE email_verifications (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT
);

CREATE INDEX email_verifications_user_idx
    ON email_verifications (user_id, created_at);

CREATE TABLE password_resets (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT
);

CREATE INDEX password_resets_user_idx
    ON password_resets (user_id, created_at);

CREATE TABLE user_notification_preferences (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, notification_type)
);

CREATE TABLE workspace_notification_preferences (
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (workspace_id, user_id, notification_type)
);

CREATE TABLE notifications (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    workspace_id TEXT REFERENCES workspaces(id) ON DELETE CASCADE,
    notification_type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TEXT NOT NULL,
    read_at TEXT
);

CREATE INDEX notifications_user_created_idx
    ON notifications (user_id, created_at, id);

CREATE INDEX notifications_workspace_created_idx
    ON notifications (workspace_id, created_at, id);
