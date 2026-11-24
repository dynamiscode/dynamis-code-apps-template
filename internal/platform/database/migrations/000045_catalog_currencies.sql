CREATE TABLE catalog_currencies (
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    decimals BIGINT NOT NULL CHECK (decimals >= 0 AND decimals <= 18),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (workspace_id, code)
);

CREATE UNIQUE INDEX catalog_currencies_default_idx ON catalog_currencies (workspace_id) WHERE is_default = TRUE;

INSERT INTO catalog_currencies (workspace_id, code, decimals, is_default)
SELECT id, 'USD', 2, TRUE FROM workspaces;

INSERT INTO catalog_currencies (workspace_id, code, decimals, is_default)
SELECT DISTINCT e.workspace_id, p.currency, 18, FALSE
FROM catalog_entries e JOIN catalog_prices p ON p.entry_id = e.id
WHERE p.currency <> 'USD';
