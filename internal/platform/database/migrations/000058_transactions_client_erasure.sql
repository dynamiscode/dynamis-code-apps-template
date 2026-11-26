CREATE TABLE transactions_entries_next (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    direction TEXT NOT NULL CHECK (direction IN ('in', 'out')),
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency TEXT NOT NULL,
    payment_method TEXT NOT NULL,
    occurred_on TEXT NOT NULL,
    client_id BIGINT REFERENCES clients(id) ON DELETE SET NULL,
    note TEXT NOT NULL,
    created_by BIGINT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (workspace_id, payment_method) REFERENCES transactions_payment_methods(workspace_id, name)
);

INSERT INTO transactions_entries_next (id, workspace_id, direction, amount_minor, currency, payment_method, occurred_on, client_id, note, created_by, created_at)
{{history_identity_override}}
SELECT id, workspace_id, direction, amount_minor, currency, payment_method, occurred_on, client_id, note, created_by, created_at FROM transactions_entries;

{{transactions_identity_reset}}
DROP TABLE transactions_entries;
ALTER TABLE transactions_entries_next RENAME TO transactions_entries;
CREATE INDEX transactions_entries_workspace_date_idx ON transactions_entries (workspace_id, occurred_on, id);
CREATE INDEX transactions_entries_workspace_client_idx ON transactions_entries (workspace_id, client_id, id);
