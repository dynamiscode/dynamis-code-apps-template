CREATE TABLE scheduling_bookings (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    resource_id BIGINT NOT NULL REFERENCES scheduling_resources(id),
    entry_id BIGINT NOT NULL REFERENCES catalog_entries(id),
    starts_at TEXT NOT NULL,
    ends_at TEXT NOT NULL,
    status TEXT NOT NULL
);

CREATE INDEX scheduling_bookings_resource_idx ON scheduling_bookings (workspace_id, resource_id, starts_at);
