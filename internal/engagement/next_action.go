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

type ActionHistoryEntry struct {
	Text, DueDate, OwnerName, Outcome, ActorName, CreatedAt string
}

type ActivityEntry struct {
	Kind, Text, DueDate, OwnerName, Outcome, ActorName, CreatedAt string
	FromStageID, ToStageID                                        StageID
}

func (s *Service) ListActivity(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, page int) ([]ActivityEntry, bool, error) {
	if page < 1 || page > 1000000 {
		return nil, false, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, false, identity.ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT kind, action_text, due_date, owner_name, outcome, actor_name, created_at, from_stage_id, to_stage_id
		FROM (
			SELECT 'action' AS kind, h.id AS source_id, h.action_text, h.due_date,
				COALESCE(NULLIF(owner.display_name, ''), owner.email, '') AS owner_name, h.outcome,
				COALESCE(NULLIF(actor.display_name, ''), actor.email, '') AS actor_name,
				h.created_at, 0 AS from_stage_id, 0 AS to_stage_id
			FROM engagement_action_history h
			LEFT JOIN users owner ON owner.id = h.owner_id
			LEFT JOIN users actor ON actor.id = h.actor_user_id
			WHERE h.workspace_id = ? AND h.engagement_id = ?
			UNION ALL
			SELECT 'stage' AS kind, h.id AS source_id, '', '', '', '',
				COALESCE(NULLIF(actor.display_name, ''), actor.email, ''),
				h.created_at, h.from_stage_id, h.to_stage_id
			FROM engagement_stage_history h
			LEFT JOIN users actor ON actor.id = h.actor_user_id
			WHERE h.workspace_id = ? AND h.engagement_id = ?
		) activity
		ORDER BY REPLACE(created_at, 'Z', '') DESC, kind DESC, source_id DESC
		LIMIT ? OFFSET ?`), workspaceID, engagementID, workspaceID, engagementID, 51, (page-1)*50)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var activity []ActivityEntry
	for rows.Next() {
		var entry ActivityEntry
		if err := rows.Scan(&entry.Kind, &entry.Text, &entry.DueDate, &entry.OwnerName, &entry.Outcome, &entry.ActorName, &entry.CreatedAt, &entry.FromStageID, &entry.ToStageID); err != nil {
			return nil, false, err
		}
		activity = append(activity, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	hasMore := len(activity) > 50
	if hasMore {
		activity = activity[:50]
	}
	return activity, hasMore, nil
}

func (s *Service) ListActionHistory(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, page int) ([]ActionHistoryEntry, bool, error) {
	if page < 1 || page > 1000000 {
		return nil, false, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, false, identity.ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT h.action_text, h.due_date, COALESCE(NULLIF(owner.display_name, ''), owner.email, ''), h.outcome, COALESCE(NULLIF(actor.display_name, ''), actor.email, ''), h.created_at FROM engagement_action_history h LEFT JOIN users owner ON owner.id = h.owner_id LEFT JOIN users actor ON actor.id = h.actor_user_id WHERE h.workspace_id = ? AND h.engagement_id = ? ORDER BY h.id DESC LIMIT ? OFFSET ?`), workspaceID, engagementID, 51, (page-1)*50)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var history []ActionHistoryEntry
	for rows.Next() {
		var entry ActionHistoryEntry
		if err := rows.Scan(&entry.Text, &entry.DueDate, &entry.OwnerName, &entry.Outcome, &entry.ActorName, &entry.CreatedAt); err != nil {
			return nil, false, err
		}
		history = append(history, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	hasMore := len(history) > 50
	if hasMore {
		history = history[:50]
	}
	return history, hasMore, nil
}

func InvalidNextActionFields(action NextAction) (text, date, owner bool) {
	action.Text = strings.TrimSpace(action.Text)
	text = !utf8.ValidString(action.Text) || utf8.RuneCountInString(action.Text) > 5000 || (action.Text == "" && (action.OwnerID != 0 || action.DueDate != ""))
	if action.DueDate != "" {
		parsed, err := time.Parse("2006-01-02", action.DueDate)
		date = err != nil || parsed.Format("2006-01-02") != action.DueDate
	}
	return
}

func (s *Service) SetNextAction(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, action NextAction, audit identity.AuditContext) error {
	return s.setNextAction(ctx, actor, workspaceID, engagementID, action, nil, audit)
}

func (s *Service) SetNextActionIfVersion(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, action NextAction, expectedVersion int64, audit identity.AuditContext) error {
	return s.setNextAction(ctx, actor, workspaceID, engagementID, action, &expectedVersion, audit)
}

func (s *Service) setNextAction(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, action NextAction, expectedVersion *int64, audit identity.AuditContext) error {
	action.Text = strings.TrimSpace(action.Text)
	text, date, owner := InvalidNextActionFields(action)
	if text || date || owner {
		return ErrInvalidInput
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
	var version int64
	query := `UPDATE engagements SET action_version = CASE WHEN action_version = 9223372036854775807 THEN NULL ELSE action_version + 1 END WHERE workspace_id = ? AND id = ?`
	args := []any{workspaceID, engagementID}
	if expectedVersion != nil {
		query += ` AND action_version = ?`
		args = append(args, *expectedVersion)
	}
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, query+` RETURNING action_version`), args...).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleNextAction
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	outcome := "replaced"
	if action.Text == "" {
		outcome = "cleared"
	}
	_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, due_date, owner_id, outcome, actor_user_id, created_at) SELECT workspace_id, engagement_id, action_text, due_date, owner_id, ?, ?, ? FROM engagement_next_actions WHERE workspace_id = ? AND engagement_id = ? AND (action_text <> ? OR due_date <> ? OR COALESCE(owner_id, 0) <> ?)`), outcome, actor.UserID, now.Format(time.RFC3339Nano), workspaceID, engagementID, action.Text, action.DueDate, action.OwnerID)
	if err != nil {
		return err
	}
	if action.Text == "" {
		_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `DELETE FROM engagement_next_actions WHERE workspace_id = ? AND engagement_id = ?`), workspaceID, engagementID)
	} else {
		var owner any
		if action.OwnerID != 0 {
			if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?`), workspaceID, action.OwnerID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidInput
			} else if err != nil {
				return err
			}
			owner = action.OwnerID
		}
		_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO engagement_next_actions (workspace_id, engagement_id, action_text, due_date, owner_id, version) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (workspace_id, engagement_id) DO UPDATE SET action_text = excluded.action_text, due_date = excluded.due_date, owner_id = excluded.owner_id, version = excluded.version`), workspaceID, engagementID, action.Text, action.DueDate, owner, version)
	}
	if err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.next_action_updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.next_action.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrStaleNextAction indicates that the active action no longer matches the expected version.
var ErrStaleNextAction = errors.New("next action is stale or absent")

func (s *Service) CompleteNextAction(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, expectedVersion int64, audit identity.AuditContext) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return identity.ErrForbidden
	}
	result, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET action_version = CASE WHEN action_version = 9223372036854775807 THEN NULL ELSE action_version + 1 END WHERE workspace_id = ? AND id = ? AND EXISTS (SELECT 1 FROM engagement_next_actions WHERE workspace_id = ? AND engagement_id = ? AND version = ?)`), workspaceID, engagementID, workspaceID, engagementID, expectedVersion)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrStaleNextAction
	}
	var text, dueDate string
	var ownerID sql.NullInt64
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `DELETE FROM engagement_next_actions WHERE workspace_id = ? AND engagement_id = ? AND version = ? RETURNING action_text, due_date, owner_id`), workspaceID, engagementID, expectedVersion).Scan(&text, &dueDate, &ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleNextAction
	}
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, due_date, owner_id, outcome, actor_user_id, created_at) VALUES (?, ?, ?, ?, ?, 'completed', ?, ?)`), workspaceID, engagementID, text, dueDate, ownerID, actor.UserID, now.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.next_action_completed", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.next_action.complete", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

// RescheduleNextAction changes only the due date of the observed active action.
func (s *Service) RescheduleNextAction(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, expectedVersion int64, dueDate string, audit identity.AuditContext) error {
	if dueDate != "" {
		parsed, err := time.Parse("2006-01-02", dueDate)
		if err != nil || parsed.Format("2006-01-02") != dueDate {
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
	var version int64
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET action_version = CASE WHEN action_version = 9223372036854775807 THEN NULL ELSE action_version + 1 END WHERE workspace_id = ? AND id = ? AND EXISTS (SELECT 1 FROM engagement_next_actions WHERE workspace_id = ? AND engagement_id = ? AND version = ?) RETURNING action_version`), workspaceID, engagementID, workspaceID, engagementID, expectedVersion).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleNextAction
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagement_next_actions SET due_date = ?, version = ? WHERE workspace_id = ? AND engagement_id = ? AND version = ?`), dueDate, version, workspaceID, engagementID, expectedVersion)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.next_action_rescheduled", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.next_action.reschedule", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

// ReopenNextAction copies a completed history entry into an empty active slot.
// expectedVersion is the engagement's action counter observed by the caller.
func (s *Service) ReopenNextAction(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, historyID, expectedVersion int64, audit identity.AuditContext) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return identity.ErrForbidden
	}
	var version int64
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET action_version = CASE WHEN action_version = 9223372036854775807 THEN NULL ELSE action_version + 1 END WHERE workspace_id = ? AND id = ? AND action_version = ? AND NOT EXISTS (SELECT 1 FROM engagement_next_actions WHERE workspace_id = ? AND engagement_id = ?) AND EXISTS (SELECT 1 FROM engagement_action_history WHERE workspace_id = ? AND engagement_id = ? AND id = ? AND outcome = 'completed') RETURNING action_version`), workspaceID, engagementID, expectedVersion, workspaceID, engagementID, workspaceID, engagementID, historyID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleNextAction
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO engagement_next_actions (workspace_id, engagement_id, action_text, due_date, owner_id, version) SELECT workspace_id, engagement_id, action_text, due_date, owner_id, ? FROM engagement_action_history WHERE workspace_id = ? AND engagement_id = ? AND id = ? AND outcome = 'completed'`), version, workspaceID, engagementID, historyID); err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO engagement_action_history (workspace_id, engagement_id, action_text, due_date, owner_id, outcome, actor_user_id, created_at) SELECT workspace_id, engagement_id, action_text, due_date, owner_id, 'reopened', ?, ? FROM engagement_action_history WHERE workspace_id = ? AND engagement_id = ? AND id = ? AND outcome = 'completed'`), actor.UserID, now.Format(time.RFC3339Nano), workspaceID, engagementID, historyID); err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.next_action_reopened", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.next_action.reopen", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now}); err != nil {
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
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT e.id, e.client_id, e.stage_id, e.notes, e.name, a.action_text, a.due_date, COALESCE(a.owner_id, 0), a.version, e.action_version, e.quote_status, e.quote_amount, e.quote_date_sent, COALESCE(e.assignee_id, 0), e.agreement_recorded, COALESCE(e.agreement_file_id, 0) FROM engagements e JOIN engagement_stages s ON s.workspace_id = e.workspace_id AND s.id = e.stage_id JOIN engagement_next_actions a ON a.workspace_id = e.workspace_id AND a.engagement_id = e.id WHERE e.workspace_id = ? AND s.kind = 'open' AND a.due_date <> '' AND a.due_date <= ? ORDER BY a.due_date, e.id`), workspaceID, today)
	if err != nil {
		return nil, err
	}
	values := []Engagement{}
	for rows.Next() {
		var value Engagement
		var fileID int64
		if err := rows.Scan(&value.ID, &value.ClientID, &value.StageID, &value.Notes, &value.Name, &value.NextAction.Text, &value.NextAction.DueDate, &value.NextAction.OwnerID, &value.NextAction.Version, &value.ActionVersion, &value.Quote.Status, &value.Quote.Amount, &value.Quote.DateSent, &value.AssigneeID, &value.Agreement.Recorded, &fileID); err != nil {
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
	for i := range values {
		values[i].History, err = s.history(ctx, tx, workspaceID, values[i].ID)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return values, nil
}
