ALTER TABLE oidc_transactions ADD COLUMN purpose TEXT NOT NULL DEFAULT 'login';
ALTER TABLE oidc_transactions ADD COLUMN user_id TEXT REFERENCES users(id) ON DELETE CASCADE;
