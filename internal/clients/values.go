package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// maxFieldValueRunes bounds one text value after trimming.
const maxFieldValueRunes = 500

var (
	ErrFieldArchived   = errors.New("client field is archived")
	ErrFieldTypeLocked = errors.New("client field type is locked by stored values")
	ErrValueRequired   = fmt.Errorf("%w: value is required; clear the value to remove it", ErrInvalidInput)
)

// FieldValue is one active field's value on a client. Value is nil when unset.
type FieldValue struct {
	FieldID FieldID `json:"fieldId"`
	Label   string  `json:"label"`
	Type    string  `json:"type"`
	Value   *string `json:"value"`
}

// SetValue stores a value under the field's slot. Members and above write. The
// client's version must match, and the write bumps it.
func (s *Service) SetValue(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	version int64,
	fieldID FieldID,
	value string,
	audit identity.AuditContext,
) (Client, error) {
	return s.writeValue(ctx, actor, workspaceID, clientID, version, fieldID, audit, "client.value.set",
		func(tx *sql.Tx, ref fieldRef) (*string, error) {
			normalized, err := s.validFieldValue(ctx, tx, workspaceID, fieldID, ref.fieldType, value)
			if err != nil {
				return nil, err
			}
			return &normalized, nil
		})
}

// ClearValue removes the field's value from the client. Same rules as SetValue.
func (s *Service) ClearValue(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	version int64,
	fieldID FieldID,
	audit identity.AuditContext,
) (Client, error) {
	return s.writeValue(ctx, actor, workspaceID, clientID, version, fieldID, audit, "client.value.clear",
		func(*sql.Tx, fieldRef) (*string, error) { return nil, nil })
}

// ListValues returns the client's values for the workspace's active fields in
// creation order. Any role may read.
func (s *Service) ListValues(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
) ([]FieldValue, error) {
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
	stored, err := s.rawValues(ctx, tx, workspaceID, clientID)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, label, field_type, slot FROM client_fields
		WHERE workspace_id = ? AND archived_at IS NULL
		ORDER BY id`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []FieldValue{}
	for rows.Next() {
		var value FieldValue
		var slot int
		if err := rows.Scan(&value.FieldID, &value.Label, &value.Type, &slot); err != nil {
			return nil, err
		}
		if raw, ok := stored[slotKey(slot)]; ok {
			value.Value = &raw
		}
		values = append(values, value)
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
	return values, nil
}

// setFieldType changes a field's type. Owners and admins only. The type locks
// once any client holds a value under the field's slot; the lock is derived from
// stored values, so clearing the last value lifts it. No surface calls this yet.
func (s *Service) setFieldType(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	fieldType string,
	audit identity.AuditContext,
) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.WorkspaceUpdate,
	); err != nil {
		return identity.ErrForbidden
	}
	ref, err := s.fieldRef(ctx, tx, workspaceID, fieldID)
	if err != nil {
		return err
	}
	inUse, err := s.slotInUse(ctx, tx, workspaceID, ref.slot)
	if err != nil {
		return err
	}
	if inUse {
		return ErrFieldTypeLocked
	}
	if !validFieldType(fieldType) {
		return ErrInvalidInput
	}
	now := s.now().UTC()
	if _, err := s.exec(ctx, tx, `
		UPDATE client_fields SET field_type = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ?
	`, fieldType, formatTime(now), workspaceID, fieldID); err != nil {
		return err
	}
	if err := s.recordFieldAudit(ctx, tx, actor, workspaceID, fieldID,
		"client_field.type_changed", "client_field.change_type", audit, now); err != nil {
		return err
	}
	return tx.Commit()
}

// writeValue applies next to the stored value under the field's slot. next
// receives the transaction and the field, returns the new value, or nil to
// remove it.
func (s *Service) writeValue(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	version int64,
	fieldID FieldID,
	audit identity.AuditContext,
	action string,
	next func(tx *sql.Tx, ref fieldRef) (*string, error),
) (Client, error) {
	if version < 1 {
		return Client{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return Client{}, identity.ErrForbidden
	}
	ref, err := s.fieldRef(ctx, tx, workspaceID, fieldID)
	if err != nil {
		return Client{}, err
	}
	if ref.archived {
		return Client{}, ErrFieldArchived
	}
	value, err := next(tx, ref)
	if err != nil {
		return Client{}, err
	}
	client, err := s.get(ctx, tx, workspaceID, clientID)
	if err != nil {
		return Client{}, err
	}
	if client.Version != version {
		return Client{}, ErrPreconditionFailed
	}
	stored, err := s.rawValues(ctx, tx, workspaceID, clientID)
	if err != nil {
		return Client{}, err
	}
	if value != nil {
		stored[slotKey(ref.slot)] = *value
	} else {
		delete(stored, slotKey(ref.slot))
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return Client{}, err
	}
	client.Version++
	client.UpdatedAt = s.now().UTC()
	result, err := s.exec(ctx, tx, `
		UPDATE clients SET custom_values = ?, version = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ? AND version = ?
	`, string(encoded), client.Version, formatTime(client.UpdatedAt), workspaceID, clientID, version)
	if err != nil {
		return Client{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Client{}, ErrPreconditionFailed
	}
	if err := s.recordChange(ctx, tx, workspaceID, client.ID, "updated", client.Version, client.UpdatedAt); err != nil {
		return Client{}, err
	}
	if err := s.publishEvent(ctx, tx, workspaceID, "client.updated", client, client.UpdatedAt); err != nil {
		return Client{}, err
	}
	// The audit metadata names the field, never the value.
	if err := s.recordAudit(ctx, tx, actor, workspaceID, audit, auditRecord{
		eventType: "client.updated", action: action, targetType: "client",
		targetKey: int64(clientID), metadata: fmt.Sprintf(`{"field":%q,"version":%d}`, fieldID.String(), client.Version),
		at: client.UpdatedAt,
	}); err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	return client, nil
}

// validFieldValue is the validation hook for value writes. It returns the stored
// text form of the value: text as typed, a number as canonical decimal, a date
// as YYYY-MM-DD, yes/no as true or false, and a choice as its option's short ID
// (ADR 0012 value representation). A choice must name an option of this field,
// checked in tx.
func (s *Service) validFieldValue(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	fieldType, value string,
) (string, error) {
	if !validFieldType(fieldType) {
		return "", ErrInvalidInput
	}
	value, err := validText(value, maxFieldValueRunes)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", ErrValueRequired
	}
	switch fieldType {
	case FieldTypeNumber:
		return validNumber(value)
	case FieldTypeDate:
		return validDate(value)
	case FieldTypeYesNo:
		return validYesNo(value)
	case FieldTypeSingleChoice:
		return s.validChoice(ctx, tx, workspaceID, fieldID, value)
	}
	return value, nil
}

// numberPattern takes plain decimals with an optional exponent. It rejects NaN,
// Inf, hex floats, and separators, which ParseFloat would otherwise accept.
var numberPattern = regexp.MustCompile(`^[+-]?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// validNumber stores a finite number as canonical decimal text. Precision is
// float64's, about 15 significant digits. ponytail: float64 ceiling, use an
// exact decimal type if a workspace needs more.
func validNumber(value string) (string, error) {
	if !numberPattern.MatchString(value) {
		return "", ErrInvalidInput
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "", ErrInvalidInput
	}
	if number == 0 {
		number = 0 // drop the sign of -0
	}
	return strconv.FormatFloat(number, 'f', -1, 64), nil
}

// validDate accepts only ISO YYYY-MM-DD calendar dates. time.Parse rejects a day
// past the month's end, such as 2026-02-30.
func validDate(value string) (string, error) {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return "", ErrInvalidInput
	}
	return date.Format("2006-01-02"), nil
}

// validYesNo accepts exactly true or false.
func validYesNo(value string) (string, error) {
	if value != "true" && value != "false" {
		return "", ErrInvalidInput
	}
	return value, nil
}

// validChoice accepts only the short ID of an option of this field. Any other
// value, including an option of another field or a label, is ErrInvalidOption.
func (s *Service) validChoice(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	value string,
) (string, error) {
	optionID, err := ParseOptionID(value)
	if err != nil {
		return "", ErrInvalidOption
	}
	var found int
	err = s.queryRow(ctx, tx, `
		SELECT 1 FROM client_field_options
		WHERE workspace_id = ? AND field_id = ? AND id = ?
	`, workspaceID, fieldID, optionID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidOption
	}
	if err != nil {
		return "", err
	}
	return optionID.String(), nil
}

// fieldRef is the internal view of a field's slot, type, and archive state.
type fieldRef struct {
	slot      int
	fieldType string
	archived  bool
}

// slotKey is the key a field's value takes in clients.custom_values.
func slotKey(slot int) string {
	return fmt.Sprintf("f%02d", slot)
}

func (s *Service) fieldRef(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
) (fieldRef, error) {
	if fieldID == 0 {
		return fieldRef{}, ErrFieldNotFound
	}
	var ref fieldRef
	var archivedAt sql.NullString
	err := s.queryRow(ctx, queryer, `
		SELECT slot, field_type, archived_at FROM client_fields
		WHERE workspace_id = ? AND id = ?
	`, workspaceID, fieldID).Scan(&ref.slot, &ref.fieldType, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fieldRef{}, ErrFieldNotFound
	}
	if err != nil {
		return fieldRef{}, err
	}
	ref.archived = archivedAt.Valid
	return ref, nil
}

// slotInUse reports whether any client in the workspace holds a value under the
// slot. It repeats the slot index expression, so the lookup uses that index.
func (s *Service) slotInUse(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	slot int,
) (bool, error) {
	expr := database.JSONTextExpr(s.driver, "custom_values", slotKey(slot))
	var found int
	err := s.queryRow(ctx, tx, fmt.Sprintf(
		"SELECT 1 FROM clients WHERE workspace_id = ? AND %s IS NOT NULL LIMIT 1", expr,
	), workspaceID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// rawValues decodes the client's custom_values into slot keys. A missing client
// is ErrNotFound.
func (s *Service) rawValues(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	workspaceID identity.WorkspaceID,
	clientID ClientID,
) (map[string]string, error) {
	if clientID == 0 {
		return nil, ErrNotFound
	}
	var raw string
	err := s.queryRow(ctx, queryer, `
		SELECT custom_values FROM clients WHERE workspace_id = ? AND id = ?
	`, workspaceID, clientID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}
