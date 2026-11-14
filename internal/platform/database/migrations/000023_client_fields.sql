-- Client custom fields (ADR 0012, ADR 0013). A field takes one slot from 1 to 50;
-- its value lives under that slot key in clients.custom_values. The 50 slot
-- indexes at the end are scoped by workspace (ADR 0012).
CREATE TABLE IF NOT EXISTS client_fields (
    id {{integer_pk}},
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    created_by_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    label TEXT NOT NULL,
    field_type TEXT NOT NULL,
    slot INTEGER NOT NULL CHECK (slot BETWEEN 1 AND 50),
    archived_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (workspace_id, slot)
);

CREATE INDEX IF NOT EXISTS client_fields_workspace_id_idx
    ON client_fields (workspace_id, id);

CREATE INDEX IF NOT EXISTS clients_custom_slot_01_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f01}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_02_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f02}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_03_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f03}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_04_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f04}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_05_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f05}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_06_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f06}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_07_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f07}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_08_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f08}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_09_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f09}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_10_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f10}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_11_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f11}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_12_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f12}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_13_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f13}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_14_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f14}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_15_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f15}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_16_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f16}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_17_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f17}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_18_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f18}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_19_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f19}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_20_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f20}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_21_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f21}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_22_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f22}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_23_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f23}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_24_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f24}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_25_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f25}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_26_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f26}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_27_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f27}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_28_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f28}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_29_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f29}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_30_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f30}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_31_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f31}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_32_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f32}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_33_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f33}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_34_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f34}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_35_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f35}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_36_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f36}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_37_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f37}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_38_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f38}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_39_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f39}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_40_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f40}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_41_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f41}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_42_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f42}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_43_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f43}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_44_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f44}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_45_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f45}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_46_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f46}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_47_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f47}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_48_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f48}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_49_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f49}}));
CREATE INDEX IF NOT EXISTS clients_custom_slot_50_idx
    ON clients (workspace_id, ({{json_text_expr:custom_values:f50}}));
