ALTER TABLE public_intake_links ADD COLUMN encrypted_token TEXT;
ALTER TABLE public_intake_links ADD COLUMN token_key_version INTEGER;
