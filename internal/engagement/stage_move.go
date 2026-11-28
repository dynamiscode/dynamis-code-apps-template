package engagement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

var ErrStaleStages = errors.New("stage order or engagement count changed")
var ErrConfirmationRequired = errors.New("stage kind change requires confirmation")

func stageKindRank(kind string) int {
	switch kind {
	case "open":
		return 0
	case "won":
		return 1
	case "lost":
		return 2
	default:
		return -1
	}
}

func (s *Service) StageEngagementCount(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, stageID StageID) (int, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
		return 0, identity.ErrForbidden
	}
	var count int
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT (SELECT COUNT(*) FROM engagements e WHERE e.workspace_id = s.workspace_id AND e.stage_id = s.id) FROM engagement_stages s WHERE s.workspace_id = ? AND s.id = ? AND s.archived_at IS NULL`), workspaceID, stageID).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrInvalidInput
	}
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Service) MoveStages(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, snapshot []Stage, orderedIDs []StageID, movedID StageID, newKind string, expectedCount int, confirmed bool, audit identity.AuditContext) (result error) {
	defer func() {
		var state interface{ SQLState() string }
		if errors.As(result, &state) && state.SQLState() == "40001" {
			result = ErrStaleStages
		}
	}()
	if stageKindRank(newKind) < 0 || expectedCount < 0 {
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
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT id, name, kind, archived_at IS NOT NULL FROM engagement_stages WHERE workspace_id = ? ORDER BY CASE kind WHEN 'open' THEN 0 WHEN 'won' THEN 1 ELSE 2 END, position`), workspaceID)
	if err != nil {
		return err
	}
	current := make([]Stage, 0, len(snapshot))
	for rows.Next() {
		var stage Stage
		if err := rows.Scan(&stage.ID, &stage.Name, &stage.Kind, &stage.Archived); err != nil {
			rows.Close()
			return err
		}
		current = append(current, stage)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(current) != len(snapshot) {
		return ErrStaleStages
	}
	for i := range current {
		if current[i] != snapshot[i] {
			return ErrStaleStages
		}
	}
	if len(orderedIDs) != len(current) {
		return ErrInvalidInput
	}
	var moved Stage
	others := make([]StageID, 0, len(current))
	byID := make(map[StageID]Stage, len(current))
	for _, stage := range current {
		byID[stage.ID] = stage
		if stage.ID == movedID {
			moved = stage
		} else {
			others = append(others, stage.ID)
		}
	}
	if moved.ID == 0 || moved.Archived {
		return ErrInvalidInput
	}
	j, lastRank, movedCount := 0, -1, 0
	for _, stageID := range orderedIDs {
		stage, ok := byID[stageID]
		if !ok {
			return ErrInvalidInput
		}
		if stageID == movedID {
			movedCount++
			stage.Kind = newKind
		} else {
			if j >= len(others) || others[j] != stageID {
				return ErrInvalidInput
			}
			j++
		}
		rank := stageKindRank(stage.Kind)
		if rank < lastRank {
			return ErrInvalidInput
		}
		lastRank = rank
	}
	if movedCount != 1 || j != len(others) {
		return ErrInvalidInput
	}
	if moved.Kind != newKind {
		if !confirmed {
			return ErrConfirmationRequired
		}
		var count int
		if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT COUNT(*) FROM engagements WHERE workspace_id = ? AND stage_id = ?`), workspaceID, movedID).Scan(&count); err != nil {
			return err
		}
		if count != expectedCount {
			return ErrStaleStages
		}
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagement_stages SET position = -position WHERE workspace_id = ?`), workspaceID); err != nil {
		return err
	}
	for i, stageID := range orderedIDs {
		if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagement_stages SET position = ? WHERE workspace_id = ? AND id = ?`), i+1, workspaceID, stageID); err != nil {
			return err
		}
	}
	if moved.Kind != newKind {
		if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagement_stages SET kind = ? WHERE workspace_id = ? AND id = ?`), newKind, workspaceID, movedID); err != nil {
			return err
		}
	}
	metadata, err := json.Marshal(map[string]any{"stageId": movedID.String(), "oldKind": moved.Kind, "newKind": newKind, "engagementCount": expectedCount})
	if err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement_stage.reordered", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement_stage", TargetKey: int64(movedID), Action: "engagement_stage.reorder", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: string(metadata), CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
}
