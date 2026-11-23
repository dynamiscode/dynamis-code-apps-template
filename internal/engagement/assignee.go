package engagement

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func (s *Service) SetAssignee(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, assignee identity.UserID, audit identity.AuditContext) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return identity.ErrForbidden
	}
	var exists int
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT 1 FROM engagements WHERE workspace_id = ? AND id = ?`), workspaceID, engagementID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var value any
	if assignee != 0 {
		if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?`), workspaceID, assignee).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidInput
		} else if err != nil {
			return err
		}
		value = assignee
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET assignee_id = ? WHERE workspace_id = ? AND id = ?`), value, workspaceID, engagementID); err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.assignee_updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.assignee.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
}
