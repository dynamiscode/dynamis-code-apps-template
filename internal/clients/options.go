package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// FieldTypeSingleChoice is a field whose values reference its choice options.
const FieldTypeSingleChoice = "single_choice"

const (
	// maxFieldOptions bounds the choice options of one single-choice field.
	maxFieldOptions     = 100
	maxOptionLabelRunes = 100
)

var (
	ErrOptionNotFound      = errors.New("client choice option not found")
	ErrOptionLimit         = errors.New("choice field option limit reached")
	ErrNotChoiceField      = fmt.Errorf("%w: field is not a single-choice field", ErrInvalidInput)
	ErrOptionLabelRequired = fmt.Errorf("%w: option label is required", ErrInvalidInput)
	ErrInvalidOption       = fmt.Errorf("%w: value must be an option of this field", ErrInvalidInput)
)

// Option is one selectable value of a single-choice field.
type Option struct {
	ID        OptionID  `json:"id"`
	FieldID   FieldID   `json:"fieldId"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"createdAt"`
}

// AddOption adds a choice to a single-choice field at the lowest free slot. It
// refuses an 101st option. Owners and admins only. An archived field takes no
// new options.
func (s *Service) AddOption(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	label string,
	audit identity.AuditContext,
) (Option, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Option{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.WorkspaceUpdate,
	); err != nil {
		return Option{}, identity.ErrForbidden
	}
	labelText, err := validOptionLabel(label)
	if err != nil {
		return Option{}, err
	}
	ref, err := s.fieldRef(ctx, tx, workspaceID, fieldID)
	if err != nil {
		return Option{}, err
	}
	if ref.fieldType != FieldTypeSingleChoice {
		return Option{}, ErrNotChoiceField
	}
	if ref.archived {
		return Option{}, ErrFieldArchived
	}
	slot, err := s.lowestFreeOptionSlot(ctx, tx, workspaceID, fieldID)
	if err != nil {
		return Option{}, err
	}
	now := s.now().UTC()
	option := Option{FieldID: fieldID, Label: labelText, CreatedAt: now}
	var key int64
	if err := s.queryRow(ctx, tx, `
		INSERT INTO client_field_options (workspace_id, field_id, slot, label, created_at)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`, workspaceID, fieldID, slot, labelText, formatTime(now)).Scan(&key); err != nil {
		return Option{}, err
	}
	option.ID = OptionID(key)
	if err := s.recordOptionAudit(ctx, tx, actor, workspaceID, option.ID,
		"client_field_option.created", "client_field_option.create", audit, now, "{}"); err != nil {
		return Option{}, err
	}
	if err := tx.Commit(); err != nil {
		return Option{}, err
	}
	return option, nil
}

// DeleteOption removes a choice. In the same transaction it clears the choice
// from every client that holds it, then deletes the option. Owners and admins
// only. Archived fields can still lose options, so their values can be cleared.
func (s *Service) DeleteOption(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
	optionID OptionID,
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
	var found int
	err = s.queryRow(ctx, tx, `
		SELECT 1 FROM client_field_options
		WHERE workspace_id = ? AND field_id = ? AND id = ?
	`, workspaceID, fieldID, optionID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOptionNotFound
	}
	if err != nil {
		return err
	}
	now := s.now().UTC()
	cleared, err := s.clearSlotValues(ctx, tx, workspaceID, ref.slot, now, "= ?", optionID.String())
	if err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, `
		DELETE FROM client_field_options
		WHERE workspace_id = ? AND field_id = ? AND id = ?
	`, workspaceID, fieldID, optionID); err != nil {
		return err
	}
	if err := s.recordOptionAudit(ctx, tx, actor, workspaceID, optionID,
		"client_field_option.deleted", "client_field_option.delete", audit, now,
		fmt.Sprintf(`{"clearedClients":%d}`, cleared)); err != nil {
		return err
	}
	return tx.Commit()
}

// ListOptions returns a field's choices in slot order. Any role may read. The
// list is the same for archived fields, so a form uses it only for active ones.
func (s *Service) ListOptions(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
) ([]Option, error) {
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
	if _, err := s.fieldRef(ctx, tx, workspaceID, fieldID); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, field_id, label, created_at FROM client_field_options
		WHERE workspace_id = ? AND field_id = ?
		ORDER BY slot`), workspaceID, fieldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	options := []Option{}
	for rows.Next() {
		var option Option
		var createdAt string
		if err := rows.Scan(&option.ID, &option.FieldID, &option.Label, &createdAt); err != nil {
			return nil, err
		}
		if option.CreatedAt, err = parseTime(createdAt); err != nil {
			return nil, err
		}
		options = append(options, option)
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
	return options, nil
}

// clearSlotValues removes the field's value from every client whose value under
// the slot matches condition (SQL after the slot expression, with args). Each
// cleared client's version bumps and its change is recorded for the list feed.
// It returns how many clients were cleared. The lookup repeats the slot
// expression, so it uses the slot index.
func (s *Service) clearSlotValues(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	slot int,
	now time.Time,
	condition string,
	args ...any,
) (int, error) {
	expr := database.JSONTextExpr(s.driver, "custom_values", slotKey(slot))
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, fmt.Sprintf(
		"SELECT id, version, custom_values FROM clients WHERE workspace_id = ? AND %s %s", expr, condition,
	)), append([]any{workspaceID}, args...)...)
	if err != nil {
		return 0, err
	}
	type holder struct {
		id      ClientID
		version int64
		raw     string
	}
	var holders []holder
	for rows.Next() {
		var h holder
		if err := rows.Scan(&h.id, &h.version, &h.raw); err != nil {
			rows.Close()
			return 0, err
		}
		holders = append(holders, h)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, h := range holders {
		values := map[string]string{}
		if err := json.Unmarshal([]byte(h.raw), &values); err != nil {
			return 0, err
		}
		delete(values, slotKey(slot))
		encoded, err := json.Marshal(values)
		if err != nil {
			return 0, err
		}
		version := h.version + 1
		if _, err := s.exec(ctx, tx, `
			UPDATE clients SET custom_values = ?, version = ?, updated_at = ?
			WHERE workspace_id = ? AND id = ?
		`, string(encoded), version, formatTime(now), workspaceID, h.id); err != nil {
			return 0, err
		}
		if err := s.recordChange(ctx, tx, workspaceID, h.id, "updated", version, now); err != nil {
			return 0, err
		}
	}
	return len(holders), nil
}

// lowestFreeOptionSlot returns the smallest slot in 1 to 100 that no option of
// the field holds. It returns ErrOptionLimit when the field has 100 options.
func (s *Service) lowestFreeOptionSlot(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
) (int, error) {
	used, err := s.usedSlots(ctx, tx,
		"SELECT slot FROM client_field_options WHERE workspace_id = ? AND field_id = ?", workspaceID, fieldID)
	if err != nil {
		return 0, err
	}
	slot, ok := firstFreeSlot(used, maxFieldOptions)
	if !ok {
		return 0, ErrOptionLimit
	}
	return slot, nil
}

func (s *Service) recordOptionAudit(
	ctx context.Context,
	tx *sql.Tx,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	optionID OptionID,
	eventType, action string,
	audit identity.AuditContext,
	now time.Time,
	metadata string,
) error {
	return s.recordAudit(ctx, tx, actor, workspaceID, audit, auditRecord{
		eventType: eventType, action: action, targetType: "client_field_option",
		targetKey: int64(optionID), metadata: metadata, at: now,
	})
}

// validOptionLabel trims the label and keeps the rest as typed.
func validOptionLabel(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrOptionLabelRequired
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxOptionLabelRunes {
		return "", ErrInvalidInput
	}
	return value, nil
}
