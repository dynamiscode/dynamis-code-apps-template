CREATE UNIQUE INDEX engagement_stages_workspace_id_idx ON engagement_stages (workspace_id, id);
CREATE UNIQUE INDEX clients_workspace_key_idx ON clients (workspace_id, id);
CREATE TABLE engagements (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    client_id BIGINT NOT NULL,
    stage_id BIGINT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (workspace_id, client_id) REFERENCES clients (workspace_id, id),
    FOREIGN KEY (workspace_id, stage_id) REFERENCES engagement_stages (workspace_id, id)
);
CREATE INDEX engagements_workspace_id_idx ON engagements (workspace_id, id);
