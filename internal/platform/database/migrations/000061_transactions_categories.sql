CREATE TABLE transactions_expense_categories (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    archived BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (workspace_id, id)
);

ALTER TABLE transactions_entries ADD COLUMN category_id BIGINT REFERENCES transactions_expense_categories(id);
CREATE INDEX transactions_expense_categories_workspace_idx ON transactions_expense_categories (workspace_id, id);
