package engagement

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func (s *Service) Label(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) (string, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return "", identity.ErrForbidden
	}
	var label string
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT label FROM engagement_labels WHERE workspace_id = ?`), workspaceID).Scan(&label)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return label, nil
}

func (s *Service) SetLabel(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, label string, audit identity.AuditContext) error {
	label = strings.TrimSpace(label)
	if !utf8.ValidString(label) || utf8.RuneCountInString(label) > 100 {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
		return identity.ErrForbidden
	}
	_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO engagement_labels (workspace_id, label) VALUES (?, ?) ON CONFLICT (workspace_id) DO UPDATE SET label = excluded.label`), workspaceID, label)
	if err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement_label.updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "workspace", TargetKey: int64(workspaceID), Action: "engagement_label.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
}
