ALTER TABLE engagements ADD COLUMN quote_status TEXT NOT NULL DEFAULT 'not_sent' CHECK (quote_status IN ('not_sent', 'sent', 'pending', 'accepted', 'declined'));
ALTER TABLE engagements ADD COLUMN quote_amount TEXT NOT NULL DEFAULT '';
ALTER TABLE engagements ADD COLUMN quote_date_sent TEXT NOT NULL DEFAULT '';
