CREATE TABLE catalog_prices (
    id {{integer_pk}},
    entry_id BIGINT NOT NULL REFERENCES catalog_entries(id) ON DELETE CASCADE,
    mode TEXT NOT NULL,
    amount TEXT,
    currency TEXT NOT NULL,
    variant TEXT NOT NULL,
    eligibility TEXT NOT NULL
);

CREATE INDEX catalog_prices_entry_idx ON catalog_prices (entry_id, id);
