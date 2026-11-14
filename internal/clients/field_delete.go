package clients

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// ErrFieldNotArchived refuses permanent deletion of a field that is still active.
var ErrFieldNotArchived = errors.New("client field must be archived before it is deleted")

// DeleteField permanently removes an archived field. In one serializable
// transaction it clears the field's value from every client that holds one,
// deletes the field's choice options, and deletes the field row. The slot is
// freed only when the transaction commits. Owners and admins only.
func (s *Service) DeleteField(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	fieldID FieldID,
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
	if err := ValidateFieldTransition(ref.status(), FieldStatusDeleted); err != nil {
		return err
	}
	now := s.now().UTC()
	cleared, err := s.clearSlotValues(ctx, tx, workspaceID, ref.slot, now, "IS NOT NULL")
	if err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, `
		DELETE FROM client_field_options WHERE workspace_id = ? AND field_id = ?
	`, workspaceID, fieldID); err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, `
		DELETE FROM client_fields WHERE workspace_id = ? AND id = ?
	`, workspaceID, fieldID); err != nil {
		return err
	}
	if err := s.recordAudit(ctx, tx, actor, workspaceID, audit, auditRecord{
		eventType: "client_field.deleted", action: "client_field.delete", targetType: "client_field",
		targetKey: int64(fieldID), metadata: fmt.Sprintf(`{"clearedClients":%d}`, cleared), at: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}
