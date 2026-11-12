-- Postmark keeps its Account Token here, sealed with the same keyring as the
-- Server Token in encrypted_password. Empty for SMTP and Resend rows.
ALTER TABLE workspace_email_settings ADD COLUMN encrypted_account_token TEXT NOT NULL DEFAULT '';
ALTER TABLE workspace_email_settings ADD COLUMN account_secret_key_version INTEGER NOT NULL DEFAULT 0;
