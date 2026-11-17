CREATE TABLE invitation_return_handoffs (
    token_hash TEXT PRIMARY KEY,
    binding_hash TEXT NOT NULL,
    invitation_id BIGINT NOT NULL REFERENCES invitations(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT
);

CREATE INDEX invitation_return_handoffs_expiry_idx
    ON invitation_return_handoffs (expires_at);

CREATE INDEX invitation_return_handoffs_invitation_idx
    ON invitation_return_handoffs (invitation_id);
