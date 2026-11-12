-- DEFAULT 1 marks existing rows as sealed with config.WebhookKeyVersion (1).
ALTER TABLE webhooks ADD COLUMN secret_key_version INTEGER NOT NULL DEFAULT 1;
