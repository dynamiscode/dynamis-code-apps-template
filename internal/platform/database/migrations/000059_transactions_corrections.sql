ALTER TABLE transactions_entries ADD COLUMN voided_by BIGINT;
ALTER TABLE transactions_entries ADD COLUMN voided_at TEXT;
ALTER TABLE transactions_entries ADD COLUMN void_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE transactions_entries ADD COLUMN replaces_entry_id BIGINT REFERENCES transactions_entries(id);
ALTER TABLE transactions_entries ADD COLUMN refund_of_entry_id BIGINT REFERENCES transactions_entries(id);

CREATE INDEX transactions_entries_refunds_idx ON transactions_entries (workspace_id, refund_of_entry_id);
CREATE UNIQUE INDEX transactions_entries_replacement_idx ON transactions_entries (workspace_id, replaces_entry_id) WHERE replaces_entry_id IS NOT NULL;
