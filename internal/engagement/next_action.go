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
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func (s *Service) SetNextAction(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, action NextAction, audit identity.AuditContext) error {
	action.Text = strings.TrimSpace(action.Text)
	if !utf8.ValidString(action.Text) || utf8.RuneCountInString(action.Text) > 5000 || (action.Text == "" && (action.OwnerID != 0 || action.DueDate != "")) || (action.Text != "" && action.OwnerID == 0) {
		return ErrInvalidInput
	}
	if action.DueDate != "" {
		date, err := time.Parse("2006-01-02", action.DueDate)
		if err != nil || date.Format("2006-01-02") != action.DueDate {
			return ErrInvalidInput
		}
	}
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
	if action.Text == "" {
		_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `DELETE FROM engagement_next_actions WHERE workspace_id = ? AND engagement_id = ?`), workspaceID, engagementID)
	} else {
		if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?`), workspaceID, action.OwnerID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidInput
		} else if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO engagement_next_actions (workspace_id, engagement_id, action_text, due_date, owner_id) VALUES (?, ?, ?, ?, ?) ON CONFLICT (workspace_id, engagement_id) DO UPDATE SET action_text = excluded.action_text, due_date = excluded.due_date, owner_id = excluded.owner_id`), workspaceID, engagementID, action.Text, action.DueDate, action.OwnerID)
	}
	if err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.next_action_updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.next_action.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ListDue(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, today string) ([]Engagement, error) {
	if _, err := time.Parse("2006-01-02", today); err != nil {
		return nil, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, identity.ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT e.id, e.client_id, e.stage_id, e.notes, a.action_text, a.due_date, COALESCE(a.owner_id, 0), e.quote_status, e.quote_amount, e.quote_date_sent, COALESCE(e.assignee_id, 0), e.agreement_recorded, COALESCE(e.agreement_file_id, 0) FROM engagements e JOIN engagement_stages s ON s.workspace_id = e.workspace_id AND s.id = e.stage_id JOIN engagement_next_actions a ON a.workspace_id = e.workspace_id AND a.engagement_id = e.id WHERE e.workspace_id = ? AND s.kind = 'open' AND a.due_date <> '' AND a.due_date <= ? ORDER BY a.due_date, e.id`), workspaceID, today)
	if err != nil {
		return nil, err
	}
	values := []Engagement{}
	for rows.Next() {
		var value Engagement
		var fileID int64
		if err := rows.Scan(&value.ID, &value.ClientID, &value.StageID, &value.Notes, &value.NextAction.Text, &value.NextAction.DueDate, &value.NextAction.OwnerID, &value.Quote.Status, &value.Quote.Amount, &value.Quote.DateSent, &value.AssigneeID, &value.Agreement.Recorded, &fileID); err != nil {
			rows.Close()
			return nil, err
		}
		value.Agreement.FileID = id.EncodeKey(id.TypeFile, fileID)
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
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
