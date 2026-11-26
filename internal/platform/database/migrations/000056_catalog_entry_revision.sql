ALTER TABLE catalog_entries ADD COLUMN revision BIGINT NOT NULL DEFAULT 1;

CREATE TABLE catalog_entry_revision_seeds (
    entry_id BIGINT PRIMARY KEY,
    revision BIGINT NOT NULL
);
INSERT INTO catalog_entry_revision_seeds (entry_id, revision)
SELECT id, revision FROM catalog_entries;
