package clients

import (
	"context"
	"database/sql"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// searchTextColumns are the common text fields a search reads.
var searchTextColumns = []string{"display_name", "email", "phone", "notes"}

// searchPredicate matches a search against every common text field and every active custom field. Text,
// number, and date values match by substring. A single-choice value matches when the option's label
// contains the search, because a value stores the option's ID. Yes-or-no values are not searched.
// ponytail: the option list is read once per single-choice field, at most 100 options per field. Upgrade:
// store a searchable label on the value when the slot limit grows.
func (s *Service) searchPredicate(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	search string,
) (string, []any, error) {
	needle := asciiLower(search)
	var parts []string
	var args []any
	for _, column := range searchTextColumns {
		parts = append(parts, database.ContainsExpr(s.driver, database.ASCIILowerExpr(s.driver, column), "?"))
		args = append(args, needle)
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, field_type, slot FROM client_fields
		WHERE workspace_id = ? AND archived_at IS NULL
		ORDER BY id
	`), workspaceID)
	if err != nil {
		return "", nil, err
	}
	type customField struct {
		id        int64
		fieldType string
		slot      int
	}
	var fields []customField
	for rows.Next() {
		var f customField
		if err := rows.Scan(&f.id, &f.fieldType, &f.slot); err != nil {
			rows.Close()
			return "", nil, err
		}
		fields = append(fields, f)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", nil, err
	}
	rows.Close()
	for _, f := range fields {
		value := database.JSONTextExpr(s.driver, "custom_values", slotKey(f.slot))
		switch fieldKind(f.fieldType) {
		case kindText, kindNumber, kindDate:
			parts = append(parts, database.ContainsExpr(s.driver, database.ASCIILowerExpr(s.driver, value), "?"))
			args = append(args, needle)
		case kindChoice:
			ids, err := s.matchingOptionIDs(ctx, tx, workspaceID, FieldID(f.id), needle)
			if err != nil {
				return "", nil, err
			}
			if len(ids) == 0 {
				continue
			}
			placeholders := make([]string, len(ids))
			for i, id := range ids {
				placeholders[i] = "?"
				args = append(args, id)
			}
			parts = append(parts, value+" IN ("+strings.Join(placeholders, ", ")+")")
		}
	}
	return "(" + strings.Join(parts, " OR ") + ")", args, nil
}

// matchingOptionIDs returns the short IDs of a choice field's options whose label contains needle.
func (s *Service) matchingOptionIDs(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	needle string,
) ([]string, error) {
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, label FROM client_field_options
		WHERE workspace_id = ? AND field_id = ?
	`), workspaceID, fieldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id OptionID
		var label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, err
		}
		if strings.Contains(asciiLower(label), needle) {
			ids = append(ids, id.String())
		}
	}
	return ids, rows.Err()
}
