CREATE TABLE workspace_currencies (
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    decimals BIGINT NOT NULL CHECK (decimals >= 0 AND decimals <= 18),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (workspace_id, code)
);

CREATE UNIQUE INDEX workspace_currencies_default_idx ON workspace_currencies (workspace_id) WHERE is_default = TRUE;
