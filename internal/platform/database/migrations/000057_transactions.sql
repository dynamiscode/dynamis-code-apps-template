CREATE TABLE transactions_payment_methods (
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (workspace_id, name)
);

INSERT INTO transactions_payment_methods (workspace_id, name)
SELECT id, 'check' FROM workspaces UNION ALL
SELECT id, 'transfer' FROM workspaces UNION ALL
SELECT id, 'cash' FROM workspaces UNION ALL
SELECT id, 'card' FROM workspaces;

CREATE TABLE transactions_entries (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    direction TEXT NOT NULL CHECK (direction IN ('in', 'out')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL,
    payment_method TEXT NOT NULL,
    occurred_on TEXT NOT NULL,
    client_id BIGINT REFERENCES clients(id),
    note TEXT NOT NULL,
    created_by BIGINT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (workspace_id, payment_method) REFERENCES transactions_payment_methods(workspace_id, name)
);

CREATE INDEX transactions_entries_workspace_date_idx ON transactions_entries (workspace_id, occurred_on, id);
CREATE INDEX transactions_entries_workspace_client_idx ON transactions_entries (workspace_id, client_id, id);
