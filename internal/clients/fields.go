package clients

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// Custom-field types. Single choice is not in this slice.
const (
	FieldTypeText   = "text"
	FieldTypeNumber = "number"
	FieldTypeDate   = "date"
	FieldTypeYesNo  = "yes_no"
)

const (
	maxFieldLabelRunes = 100
	// maxFieldSlots is the slot count in ADR 0012. Slots are numbered 1 to 50.
	maxFieldSlots = 50
)

var (
	ErrLimit              = errors.New("workspace custom field limit reached")
	ErrFieldNotFound      = errors.New("client field not found")
	ErrFieldLabelRequired = fmt.Errorf("%w: field label is required", ErrInvalidInput)
)

// Field is a workspace-defined custom field. Its slot is internal.
type Field struct {
	ID          FieldID              `json:"id"`
	WorkspaceID identity.WorkspaceID `json:"workspaceId"`
	Label       string               `json:"label"`
	Type        string               `json:"type"`
	ArchivedAt  *time.Time           `json:"archivedAt"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

// FieldInput defines a custom field.
type FieldInput struct {
	Label string
	Type  string
}

// ListFieldsInput selects fields. Forms use the default, which omits archived fields.
type ListFieldsInput struct {
	IncludeArchived bool
}

// DefineField adds a text, number, date, or yes/no field at the lowest free slot.
// Owners and admins only.
// Archived fields keep their slots and count toward the limit.
func (s *Service) DefineField(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	input FieldInput,
	audit identity.AuditContext,
) (Field, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Field{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.WorkspaceUpdate,
	); err != nil {
		return Field{}, identity.ErrForbidden
	}
	label, err := validFieldLabel(input.Label)
	if err != nil {
		return Field{}, err
	}
	if !validFieldType(input.Type) {
		return Field{}, ErrInvalidInput
	}
	var count int
	if err := s.queryRow(ctx, tx,
		"SELECT COUNT(*) FROM client_fields WHERE workspace_id = ?", workspaceID,
	).Scan(&count); err != nil {
		return Field{}, err
	}
	if count >= s.maxFields {
		return Field{}, ErrLimit
	}
	slot, err := s.lowestFreeSlot(ctx, tx, workspaceID)
	if err != nil {
		return Field{}, err
	}
	now := s.now().UTC()
	field := Field{
		WorkspaceID: workspaceID, Label: label, Type: input.Type,
		CreatedAt: now, UpdatedAt: now,
	}
	var key int64
	if err := s.queryRow(ctx, tx, `
		INSERT INTO client_fields (
			workspace_id, created_by_user_id, label, field_type, slot, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, workspaceID, actor.UserID, label, input.Type, slot,
		formatTime(now), formatTime(now)).Scan(&key); err != nil {
		return Field{}, err
	}
	field.ID = FieldID(key)
	if err := s.recordFieldAudit(ctx, tx, actor, workspaceID, field.ID, "client_field.created", "client_field.create", audit, now); err != nil {
		return Field{}, err
	}
	if err := tx.Commit(); err != nil {
		return Field{}, err
	}
	return field, nil
}

// ArchiveField hides a field from forms and keeps its values. Archiving an
// archived field changes nothing. Owners and admins only.
func (s *Service) ArchiveField(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	audit identity.AuditContext,
) (Field, error) {
	return s.setFieldArchived(ctx, actor, workspaceID, fieldID, audit, true)
}

// RestoreField reverses ArchiveField. Owners and admins only.
func (s *Service) RestoreField(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	audit identity.AuditContext,
) (Field, error) {
	return s.setFieldArchived(ctx, actor, workspaceID, fieldID, audit, false)
}

func (s *Service) setFieldArchived(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	audit identity.AuditContext,
	archive bool,
) (Field, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Field{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.WorkspaceUpdate,
	); err != nil {
		return Field{}, identity.ErrForbidden
	}
	field, err := s.getField(ctx, tx, workspaceID, fieldID)
	if err != nil {
		return Field{}, err
	}
	if (field.ArchivedAt != nil) == archive {
		return field, nil
	}
	now := s.now().UTC()
	var archivedAt any
	action, eventType := "client_field.restore", "client_field.restored"
	field.ArchivedAt = nil
	if archive {
		archivedAt = formatTime(now)
		action, eventType = "client_field.archive", "client_field.archived"
		field.ArchivedAt = &now
	}
	field.UpdatedAt = now
	if _, err := s.exec(ctx, tx, `
		UPDATE client_fields SET archived_at = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ?
	`, archivedAt, formatTime(now), workspaceID, fieldID); err != nil {
		return Field{}, err
	}
	if err := s.recordFieldAudit(ctx, tx, actor, workspaceID, fieldID, eventType, action, audit, now); err != nil {
		return Field{}, err
	}
	if err := tx.Commit(); err != nil {
		return Field{}, err
	}
	return field, nil
}

// ListFields returns the workspace's fields in creation order. Any role may read.
func (s *Service) ListFields(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	input ListFieldsInput,
) ([]Field, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesRead,
	); err != nil {
		return nil, identity.ErrForbidden
	}
	query := `
		SELECT id, workspace_id, label, field_type, archived_at, created_at, updated_at
		FROM client_fields
		WHERE workspace_id = ?`
	if !input.IncludeArchived {
		query += " AND archived_at IS NULL"
	}
	query += " ORDER BY id"
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, query), workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	fields := []Field{}
	for rows.Next() {
		field, err := scanField(rows)
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return fields, nil
}

func (s *Service) getField(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
) (Field, error) {
	if fieldID == 0 {
		return Field{}, ErrFieldNotFound
	}
	field, err := scanField(s.queryRow(ctx, queryer, `
		SELECT id, workspace_id, label, field_type, archived_at, created_at, updated_at
		FROM client_fields
		WHERE workspace_id = ? AND id = ?
	`, workspaceID, fieldID))
	if errors.Is(err, sql.ErrNoRows) {
		return Field{}, ErrFieldNotFound
	}
	return field, err
}

// lowestFreeSlot returns the smallest slot in 1 to 50 that no field in the
// workspace holds. Archived fields hold their slots, so they are skipped too.
func (s *Service) lowestFreeSlot(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID) (int, error) {
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver,
		"SELECT slot FROM client_fields WHERE workspace_id = ?"), workspaceID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	used := make(map[int]bool)
	for rows.Next() {
		var slot int
		if err := rows.Scan(&slot); err != nil {
			return 0, err
		}
		used[slot] = true
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for slot := 1; slot <= maxFieldSlots; slot++ {
		if !used[slot] {
			return slot, nil
		}
	}
	return 0, ErrLimit
}

func (s *Service) recordFieldAudit(
	ctx context.Context,
	tx *sql.Tx,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	eventType, action string,
	audit identity.AuditContext,
	now time.Time,
) error {
	return s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: eventType, ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "client_field", TargetKey: int64(fieldID), Action: action,
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	})
}

// validFieldType reports whether a custom field may take the type. Single choice
// is not in this slice.
func validFieldType(fieldType string) bool {
	switch fieldType {
	case FieldTypeText, FieldTypeNumber, FieldTypeDate, FieldTypeYesNo:
		return true
	}
	return false
}

// validFieldLabel trims the label and keeps the rest as typed.
func validFieldLabel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrFieldLabelRequired
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxFieldLabelRunes {
		return "", ErrInvalidInput
	}
	return value, nil
}

func scanField(row scanner) (Field, error) {
	var field Field
	var archivedAt sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(
		&field.ID, &field.WorkspaceID, &field.Label, &field.Type,
		&archivedAt, &createdAt, &updatedAt,
	); err != nil {
		return Field{}, err
	}
	var err error
	if field.CreatedAt, err = parseTime(createdAt); err != nil {
		return Field{}, err
	}
	if field.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return Field{}, err
	}
	if archivedAt.Valid {
		value, err := parseTime(archivedAt.String)
		if err != nil {
			return Field{}, err
		}
		field.ArchivedAt = &value
	}
	return field, nil
}
