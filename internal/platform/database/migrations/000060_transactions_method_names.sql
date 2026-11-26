ALTER TABLE transactions_payment_methods ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
UPDATE transactions_payment_methods SET display_name = name;
