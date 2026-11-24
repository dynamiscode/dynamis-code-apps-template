CREATE TABLE public_intake_links (
    workspace_id BIGINT PRIMARY KEY REFERENCES workspaces(id) ON DELETE CASCADE,
    token_hash TEXT UNIQUE,
    consent_text TEXT NOT NULL
);
