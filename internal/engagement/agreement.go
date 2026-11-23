package engagement

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type Agreement struct {
	Recorded bool   `json:"recorded"`
	FileID   string `json:"fileId"`
}

func (s *Service) SetAgreement(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, agreement Agreement, audit identity.AuditContext) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return identity.ErrForbidden
	}
	var fileID any
	if agreement.FileID != "" {
		key, err := files.ParseFileID(agreement.FileID)
		if err != nil {
			return ErrInvalidInput
		}
		fileService := files.NewService(s.db, s.driver, s.auth, nil, 0, 0, 0, "")
		if _, err := fileService.GetInTx(ctx, tx, workspaceID, key); errors.Is(err, files.ErrNotFound) {
			return ErrInvalidInput
		} else if err != nil {
			return err
		}
		fileID = key
	}
	result, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET agreement_recorded = ?, agreement_file_id = ? WHERE workspace_id = ? AND id = ?`), agreement.Recorded, fileID, workspaceID, engagementID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.agreement_updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.agreement.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
}
