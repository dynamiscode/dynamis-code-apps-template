-- DEFAULT 'smtp' marks existing rows as SMTP settings. Resend rows reuse the
-- encrypted_password column for the API key and leave host, port, and username empty.
ALTER TABLE workspace_email_settings ADD COLUMN provider TEXT NOT NULL DEFAULT 'smtp';
ALTER TABLE workspace_email_settings ADD COLUMN sender_domain_status TEXT NOT NULL DEFAULT '';
