package engagement

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var ErrNoOpenStage = errors.New("workspace has no open stage")
var ErrNotFound = errors.New("engagement not found")

type ID int64

func (v ID) String() string               { return id.EncodeKey(id.TypeEngagement, int64(v)) }
func (v ID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func ParseID(value string) ID             { key, _ := id.DecodeKey(id.TypeEngagement, value); return ID(key) }

type Input struct {
	ClientID clients.ClientID `json:"clientId"`
	StageID  StageID          `json:"-"`
	Notes    string           `json:"notes"`
}
type Engagement struct {
	ID       ID               `json:"id"`
	ClientID clients.ClientID `json:"clientId"`
	StageID  StageID          `json:"stageId"`
	Notes    string           `json:"notes"`
}

func (s *Service) Create(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, input Input, audit identity.AuditContext) (Engagement, error) {
	input.Notes = strings.TrimSpace(input.Notes)
	if input.ClientID == 0 || !utf8.ValidString(input.Notes) || utf8.RuneCountInString(input.Notes) > 5000 {
		return Engagement{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Engagement{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return Engagement{}, identity.ErrForbidden
	}
	clientService := clients.NewService(s.db, s.driver, s.auth, 0)
	if _, err := clientService.GetInTx(ctx, tx, workspaceID, input.ClientID); errors.Is(err, clients.ErrNotFound) {
		return Engagement{}, ErrInvalidInput
	} else if err != nil {
		return Engagement{}, err
	}
	var stageID StageID
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT id FROM engagement_stages WHERE workspace_id = ? AND kind = 'open' AND archived_at IS NULL ORDER BY position LIMIT 1`), workspaceID).Scan(&stageID)
	if errors.Is(err, sql.ErrNoRows) {
		return Engagement{}, ErrNoOpenStage
	}
	if err != nil {
		return Engagement{}, err
	}
	if input.StageID != 0 {
		err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT id FROM engagement_stages WHERE workspace_id = ? AND id = ? AND kind = 'open' AND archived_at IS NULL`), workspaceID, input.StageID).Scan(&stageID)
		if errors.Is(err, sql.ErrNoRows) {
			return Engagement{}, ErrInvalidInput
		}
		if err != nil {
			return Engagement{}, err
		}
	}
	value := Engagement{ClientID: input.ClientID, StageID: stageID, Notes: input.Notes}
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `INSERT INTO engagements (workspace_id, client_id, stage_id, notes) VALUES (?, ?, ?, ?) RETURNING id`), workspaceID, value.ClientID, value.StageID, value.Notes).Scan(&value.ID)
	if err != nil {
		return Engagement{}, err
	}
	err = s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(value.ID), Action: "engagement.create", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()})
	if err != nil {
		return Engagement{}, err
	}
	if err := tx.Commit(); err != nil {
		return Engagement{}, err
	}
	return value, nil
}

func (s *Service) Get(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID) (Engagement, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return Engagement{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return Engagement{}, identity.ErrForbidden
	}
	var value Engagement
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT id, client_id, stage_id, notes FROM engagements WHERE workspace_id = ? AND id = ?`), workspaceID, engagementID).Scan(&value.ID, &value.ClientID, &value.StageID, &value.Notes)
	if errors.Is(err, sql.ErrNoRows) {
		return Engagement{}, ErrNotFound
	}
	if err != nil {
		return Engagement{}, err
	}
	if err := tx.Commit(); err != nil {
		return Engagement{}, err
	}
	return value, nil
}

func (s *Service) ListForClient(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, clientID clients.ClientID) ([]Engagement, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, identity.ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT id, client_id, stage_id, notes FROM engagements WHERE workspace_id = ? AND client_id = ? ORDER BY id DESC`), workspaceID, clientID)
	if err != nil {
		return nil, err
	}
	var values []Engagement
	for rows.Next() {
		var value Engagement
		if err := rows.Scan(&value.ID, &value.ClientID, &value.StageID, &value.Notes); err != nil {
			rows.Close()
			return nil, err
		}
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
