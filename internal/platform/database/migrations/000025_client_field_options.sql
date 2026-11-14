-- Choice options of single-choice client fields (ADR 0013, spec #294). An option
-- belongs to one field. Its slot is unique within that field (1 to 100) and
-- orders the choices. A client value stores the option's short ID under the
-- field's slot. Deleting an option clears that value from every client in the
-- same transaction, then deletes the row. Every query also filters by
-- workspace_id, so a field's options never leave its workspace.
CREATE TABLE IF NOT EXISTS client_field_options (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    field_id BIGINT NOT NULL REFERENCES client_fields(id) ON DELETE CASCADE,
    slot INTEGER NOT NULL CHECK (slot BETWEEN 1 AND 100),
    label TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE (field_id, slot)
);

CREATE INDEX IF NOT EXISTS client_field_options_workspace_field_idx
    ON client_field_options (workspace_id, field_id, slot);
