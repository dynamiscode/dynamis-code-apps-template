CREATE TABLE public_links (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    item_id BIGINT NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX public_links_workspace_idx
    ON public_links (workspace_id, item_id, created_at, id);

CREATE INDEX public_links_expiry_idx
    ON public_links (expires_at, revoked_at);
