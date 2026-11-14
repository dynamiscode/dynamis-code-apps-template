package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/portability"
)

// ExportClients implements portability.ClientSource. It reads the workspace's
// fields, choice options, and clients inside the export transaction, archived
// records included. Stored values are rekeyed from slot keys to field short IDs
// here, where the slot format is owned.
func (s *Service) ExportClients(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	limit int,
) (portability.ClientExport, error) {
	fields, slotIDs, err := s.exportFields(ctx, tx, workspaceID, limit)
	if err != nil {
		return portability.ClientExport{}, err
	}
	options, err := s.exportOptions(ctx, tx, workspaceID, limit)
	if err != nil {
		return portability.ClientExport{}, err
	}
	records, err := s.exportClientRecords(ctx, tx, workspaceID, slotIDs, limit)
	if err != nil {
		return portability.ClientExport{}, err
	}
	return portability.ClientExport{Clients: records, ClientFields: fields, ClientFieldOptions: options}, nil
}

// exportFields returns every field and a map from each slot key to its field
// short ID. Archived fields stay in the map, so their stored values export too.
func (s *Service) exportFields(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	limit int,
) ([]portability.ClientField, map[string]string, error) {
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, workspace_id, label, field_type, slot, archived_at, created_at, updated_at
		FROM client_fields WHERE workspace_id = ? ORDER BY created_at, id LIMIT ?
	`), workspaceID, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	result := make([]portability.ClientField, 0)
	slotIDs := map[string]string{}
	for rows.Next() {
		var value portability.ClientField
		var fieldID FieldID
		var slot int
		var archivedAt sql.NullString
		var createdAt, updatedAt string
		if err := rows.Scan(
			&fieldID, &value.WorkspaceID, &value.Label, &value.Type, &slot,
			&archivedAt, &createdAt, &updatedAt,
		); err != nil {
			return nil, nil, err
		}
		if value.ArchivedAt, err = exportOptionalTime(archivedAt); err != nil {
			return nil, nil, err
		}
		if value.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, nil, err
		}
		if value.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, nil, err
		}
		value.ID = fieldID.String()
		slotIDs[slotKey(slot)] = value.ID
		result = append(result, value)
	}
	return result, slotIDs, rows.Err()
}

func (s *Service) exportOptions(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	limit int,
) ([]portability.ClientFieldOption, error) {
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, field_id, label, created_at
		FROM client_field_options WHERE workspace_id = ? ORDER BY field_id, slot LIMIT ?
	`), workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]portability.ClientFieldOption, 0)
	for rows.Next() {
		var value portability.ClientFieldOption
		var optionID OptionID
		var fieldID FieldID
		var createdAt string
		if err := rows.Scan(&optionID, &fieldID, &value.Label, &createdAt); err != nil {
			return nil, err
		}
		value.ID = optionID.String()
		value.FieldID = fieldID.String()
		if value.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// exportClientRecords returns every client, archived ones included.
func (s *Service) exportClientRecords(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	slotIDs map[string]string,
	limit int,
) ([]portability.ClientRecord, error) {
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, workspace_id, created_by_user_id, display_name, email, phone, notes,
			archived_at, version, custom_values, created_at, updated_at
		FROM clients WHERE workspace_id = ? ORDER BY created_at, id LIMIT ?
	`), workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]portability.ClientRecord, 0)
	for rows.Next() {
		var value portability.ClientRecord
		var clientID ClientID
		var createdByUserID sql.NullInt64
		var archivedAt sql.NullString
		var rawValues, createdAt, updatedAt string
		if err := rows.Scan(
			&clientID, &value.WorkspaceID, &createdByUserID, &value.DisplayName,
			&value.Email, &value.Phone, &value.Notes, &archivedAt, &value.Version,
			&rawValues, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		if createdByUserID.Valid {
			creator := identity.UserID(createdByUserID.Int64)
			value.CreatedByUserID = &creator
		}
		if value.ArchivedAt, err = exportOptionalTime(archivedAt); err != nil {
			return nil, err
		}
		if value.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		if value.UpdatedAt, err = parseTime(updatedAt); err != nil {
			return nil, err
		}
		value.ID = clientID.String()
		if value.CustomValues, err = exportCustomValues(rawValues, slotIDs); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// exportCustomValues decodes a client's custom_values and rekeys them by field
// short ID. A slot with no field left is dropped, so no slot key leaves.
func exportCustomValues(raw string, slotIDs map[string]string) (map[string]string, error) {
	stored := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	values := make(map[string]string, len(stored))
	for slot, value := range stored {
		if fieldID, ok := slotIDs[slot]; ok {
			values[fieldID] = value
		}
	}
	return values, nil
}

func exportOptionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
