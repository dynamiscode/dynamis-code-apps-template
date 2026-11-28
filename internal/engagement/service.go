package engagement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrInvalidInput = errors.New("invalid stage input")

type StageID int64

func (s StageID) String() string               { return id.EncodeKey(id.TypeEngagementStage, int64(s)) }
func (s StageID) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

type Stage struct {
	ID       StageID `json:"id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Archived bool    `json:"archived"`
}

func ParseStageID(value string) StageID {
	key, _ := id.DecodeKey(id.TypeEngagementStage, value)
	return StageID(key)
}

type StageInput struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type Service struct {
	db     *sql.DB
	driver config.DatabaseDriver
	auth   *identity.Service
}

func NewService(db *sql.DB, driver config.DatabaseDriver, auth *identity.Service) *Service {
	return &Service{db: db, driver: driver, auth: auth}
}

func (s *Service) CreateStage(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, input StageInput, audit identity.AuditContext) (Stage, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || !utf8.ValidString(input.Name) || utf8.RuneCountInString(input.Name) > 200 || (input.Kind != "open" && input.Kind != "won" && input.Kind != "lost") {
		return Stage{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Stage{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
		return Stage{}, identity.ErrForbidden
	}
	stage := Stage{Name: input.Name, Kind: input.Kind}
	query := `INSERT INTO engagement_stages (workspace_id, name, kind, position) VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM engagement_stages WHERE workspace_id = ?)) RETURNING id`
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, query), workspaceID, stage.Name, stage.Kind, workspaceID).Scan(&stage.ID); err != nil {
		return Stage{}, err
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagement_stages SET position = -position WHERE workspace_id = ?`), workspaceID); err != nil {
		return Stage{}, err
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `WITH ordered AS MATERIALIZED (
		SELECT id, ROW_NUMBER() OVER (ORDER BY CASE kind WHEN 'open' THEN 0 WHEN 'won' THEN 1 ELSE 2 END, -position) AS new_position
		FROM engagement_stages WHERE workspace_id = ?
	) UPDATE engagement_stages SET position = (SELECT new_position FROM ordered WHERE ordered.id = engagement_stages.id) WHERE workspace_id = ?`), workspaceID, workspaceID); err != nil {
		return Stage{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement_stage.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement_stage", TargetKey: int64(stage.ID), Action: "engagement_stage.create", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return Stage{}, err
	}
	if err := tx.Commit(); err != nil {
		return Stage{}, err
	}
	return stage, nil
}

func (s *Service) ListStages(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) ([]Stage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, identity.ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT id, name, kind, archived_at IS NOT NULL FROM engagement_stages WHERE workspace_id = ? ORDER BY CASE kind WHEN 'open' THEN 0 WHEN 'won' THEN 1 ELSE 2 END, position`), workspaceID)
	if err != nil {
		return nil, err
	}
	stages := []Stage{}
	for rows.Next() {
		var stage Stage
		if err := rows.Scan(&stage.ID, &stage.Name, &stage.Kind, &stage.Archived); err != nil {
			rows.Close()
			return nil, err
		}
		stages = append(stages, stage)
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
	return stages, nil
}

func (s *Service) ArchiveStage(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, stageID StageID, audit identity.AuditContext) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
		return identity.ErrForbidden
	}
	result, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagement_stages SET archived_at = ? WHERE workspace_id = ? AND id = ? AND archived_at IS NULL`), time.Now().UTC().Format(time.RFC3339Nano), workspaceID, stageID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInvalidInput
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement_stage.archived", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement_stage", TargetKey: int64(stageID), Action: "engagement_stage.archive", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
}
