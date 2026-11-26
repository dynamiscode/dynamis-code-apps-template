package engagement

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
var ErrInvalidName = fmt.Errorf("%w: engagement name is invalid", ErrInvalidInput)
var ErrInvalidStage = fmt.Errorf("%w: engagement stage is invalid", ErrInvalidInput)
var ErrInvalidNotes = fmt.Errorf("%w: engagement notes are invalid", ErrInvalidInput)
var ErrNotesConflict = errors.New("engagement notes changed")

func NotesRevision(notes string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(notes)))
}

type ID int64

func (v ID) String() string               { return id.EncodeKey(id.TypeEngagement, int64(v)) }
func (v ID) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func ParseID(value string) ID             { key, _ := id.DecodeKey(id.TypeEngagement, value); return ID(key) }

type Input struct {
	Name     string           `json:"name"`
	ClientID clients.ClientID `json:"clientId"`
	StageID  StageID          `json:"-"`
	Notes    string           `json:"notes"`
}
type Engagement struct {
	Name          string           `json:"name"`
	ID            ID               `json:"id"`
	ClientID      clients.ClientID `json:"clientId"`
	StageID       StageID          `json:"stageId"`
	Notes         string           `json:"notes"`
	History       []MoveHistory    `json:"history"`
	NextAction    NextAction       `json:"nextAction"`
	ActionVersion int64            `json:"-"`
	Quote         Quote            `json:"quote"`
	AssigneeID    identity.UserID  `json:"assigneeId"`
	Agreement     Agreement        `json:"agreement"`
}

type MoveHistory struct {
	ActorName   string          `json:"-"`
	ActorUserID identity.UserID `json:"actorUserId"`
	CreatedAt   string          `json:"createdAt"`
	FromStageID StageID         `json:"fromStageId"`
	ToStageID   StageID         `json:"toStageId"`
}

func (s *Service) Move(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, stageID StageID, audit identity.AuditContext) error {
	return s.MoveMany(ctx, actor, workspaceID, []ID{engagementID}, stageID, audit)
}

// MoveMany moves every selected engagement in one transaction, or changes none.
func (s *Service) MoveMany(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, ids []ID, stageID StageID, audit identity.AuditContext) error {
	if len(ids) == 0 || len(ids) > 100 {
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
	for i, id := range ids {
		if id <= 0 {
			return ErrInvalidInput
		}
		for _, previous := range ids[:i] {
			if previous == id {
				return ErrInvalidInput
			}
		}
		if err := s.moveInTx(ctx, tx, actor, workspaceID, id, stageID, audit); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) moveInTx(ctx context.Context, tx *sql.Tx, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, stageID StageID, audit identity.AuditContext) error {
	var from StageID
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT stage_id FROM engagements WHERE workspace_id = ? AND id = ?`), workspaceID, engagementID).Scan(&from); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var target StageID
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT id FROM engagement_stages WHERE workspace_id = ? AND id = ? AND archived_at IS NULL`), workspaceID, stageID).Scan(&target); errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidInput
	} else if err != nil {
		return err
	}
	if from == target {
		return nil
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET stage_id = ? WHERE workspace_id = ? AND id = ?`), target, workspaceID, engagementID); err != nil {
		return err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO engagement_stage_history (workspace_id, engagement_id, actor_user_id, from_stage_id, to_stage_id, created_at) VALUES (?, ?, ?, ?, ?, ?)`), workspaceID, engagementID, actor.UserID, from, target, now.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	metadata, err := json.Marshal(struct {
		From string `json:"from"`
		To   string `json:"to"`
	}{from.String(), target.String()})
	if err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.moved", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.move", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: string(metadata), CreatedAt: now}); err != nil {
		return err
	}
	return nil
}

func (s *Service) history(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID, engagementID ID) ([]MoveHistory, error) {
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT h.actor_user_id, h.created_at, h.from_stage_id, h.to_stage_id, COALESCE(NULLIF(u.display_name, ''), u.email, '') FROM engagement_stage_history h LEFT JOIN users u ON u.id = h.actor_user_id WHERE h.workspace_id = ? AND h.engagement_id = ? ORDER BY h.id`), workspaceID, engagementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := []MoveHistory{}
	for rows.Next() {
		var entry MoveHistory
		if err := rows.Scan(&entry.ActorUserID, &entry.CreatedAt, &entry.FromStageID, &entry.ToStageID, &entry.ActorName); err != nil {
			return nil, err
		}
		history = append(history, entry)
	}
	return history, rows.Err()
}

type Quote struct {
	Status   string `json:"status"`
	Amount   string `json:"amount"`
	DateSent string `json:"dateSent"`
}

type NextAction struct {
	Text    string          `json:"text"`
	DueDate string          `json:"dueDate"`
	OwnerID identity.UserID `json:"ownerId"`
	Version int64           `json:"version"`
}

func validName(name string) bool {
	return name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 200
}

func (s *Service) Create(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, input Input, audit identity.AuditContext) (Engagement, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) {
		return Engagement{}, ErrInvalidName
	}
	if input.ClientID == 0 {
		return Engagement{}, ErrInvalidInput
	}
	if !utf8.ValidString(input.Notes) || utf8.RuneCountInString(input.Notes) > 5000 {
		return Engagement{}, ErrInvalidNotes
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Engagement{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return Engagement{}, identity.ErrForbidden
	}
	value, err := s.createInTx(ctx, tx, actor, workspaceID, input, audit)
	if err != nil {
		return Engagement{}, err
	}
	if err := tx.Commit(); err != nil {
		return Engagement{}, err
	}
	return value, nil
}

// CreateIntakeInTx creates an engagement only for an active workspace link.
func (s *Service) CreateIntakeInTx(ctx context.Context, tx *sql.Tx, token string, workspaceID identity.WorkspaceID, clientID clients.ClientID, audit identity.AuditContext) (Engagement, error) {
	if err := s.auth.AuthorizeIntakeInTx(ctx, tx, token, workspaceID); err != nil {
		return Engagement{}, err
	}
	client, err := clients.NewService(s.db, s.driver, s.auth, 0).GetInTx(ctx, tx, workspaceID, clientID)
	if err != nil {
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
	return s.createInTx(ctx, tx, identity.Principal{AuthMethod: "public_intake"}, workspaceID, Input{Name: client.DisplayName, ClientID: clientID, StageID: stageID}, audit)
}

func (s *Service) createInTx(ctx context.Context, tx *sql.Tx, actor identity.Principal, workspaceID identity.WorkspaceID, input Input, audit identity.AuditContext) (Engagement, error) {
	clientService := clients.NewService(s.db, s.driver, s.auth, 0)
	client, err := clientService.GetInTx(ctx, tx, workspaceID, input.ClientID)
	if errors.Is(err, clients.ErrNotFound) {
		return Engagement{}, ErrInvalidInput
	} else if err != nil {
		return Engagement{}, err
	}
	if client.ArchivedAt != nil {
		return Engagement{}, ErrInvalidInput
	}
	var stageID StageID
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT id FROM engagement_stages WHERE workspace_id = ? AND id = ? AND kind = 'open' AND archived_at IS NULL`), workspaceID, input.StageID).Scan(&stageID)
	if errors.Is(err, sql.ErrNoRows) {
		return Engagement{}, ErrInvalidStage
	}
	if err != nil {
		return Engagement{}, err
	}
	value := Engagement{Name: input.Name, ClientID: input.ClientID, StageID: stageID, Notes: input.Notes, History: []MoveHistory{}, Quote: Quote{Status: "not_sent"}}
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `INSERT INTO engagements (workspace_id, client_id, stage_id, notes, name) VALUES (?, ?, ?, ?, ?) RETURNING id`), workspaceID, value.ClientID, value.StageID, value.Notes, value.Name).Scan(&value.ID)
	if err != nil {
		return Engagement{}, err
	}
	err = s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(value.ID), Action: "engagement.create", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()})
	if err != nil {
		return Engagement{}, err
	}
	return value, nil
}

func (s *Service) SetName(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, name string, audit identity.AuditContext) error {
	name = strings.TrimSpace(name)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return identity.ErrForbidden
	}
	var current string
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT name FROM engagements WHERE workspace_id = ? AND id = ?`), workspaceID, engagementID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if name == current {
		return nil
	}
	if !validName(name) {
		return ErrInvalidName
	}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET name = ? WHERE workspace_id = ? AND id = ?`), name, workspaceID, engagementID); err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.name_updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.name.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) SetNotes(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, notes string, revision *string, audit identity.AuditContext) (err error) {
	defer func() {
		var state interface{ SQLState() string }
		if errors.As(err, &state) && state.SQLState() == "40001" {
			err = ErrNotesConflict
		}
	}()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return identity.ErrForbidden
	}
	var current string
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT notes FROM engagements WHERE workspace_id = ? AND id = ?`), workspaceID, engagementID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if revision != nil && *revision != NotesRevision(current) {
		return ErrNotesConflict
	}
	if notes == current {
		return nil
	}
	if !utf8.ValidString(notes) || utf8.RuneCountInString(notes) > 5000 {
		return ErrInvalidNotes
	}
	result, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET notes = ? WHERE workspace_id = ? AND id = ? AND notes = ?`), notes, workspaceID, engagementID, current)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotesConflict
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.notes_updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.notes.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
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
	var fileID int64
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT e.id, e.client_id, e.stage_id, e.notes, e.name, COALESCE(a.action_text, ''), COALESCE(a.due_date, ''), COALESCE(a.owner_id, 0), COALESCE(a.version, 0), e.action_version, e.quote_status, e.quote_amount, e.quote_date_sent, COALESCE(e.assignee_id, 0), e.agreement_recorded, COALESCE(e.agreement_file_id, 0) FROM engagements e LEFT JOIN engagement_next_actions a ON a.workspace_id = e.workspace_id AND a.engagement_id = e.id WHERE e.workspace_id = ? AND e.id = ?`), workspaceID, engagementID).Scan(&value.ID, &value.ClientID, &value.StageID, &value.Notes, &value.Name, &value.NextAction.Text, &value.NextAction.DueDate, &value.NextAction.OwnerID, &value.NextAction.Version, &value.ActionVersion, &value.Quote.Status, &value.Quote.Amount, &value.Quote.DateSent, &value.AssigneeID, &value.Agreement.Recorded, &fileID)
	value.Agreement.FileID = id.EncodeKey(id.TypeFile, fileID)
	if errors.Is(err, sql.ErrNoRows) {
		return Engagement{}, ErrNotFound
	}
	if err != nil {
		return Engagement{}, err
	}
	value.History, err = s.history(ctx, tx, workspaceID, engagementID)
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
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT e.id, e.client_id, e.stage_id, e.notes, e.name, COALESCE(a.action_text, ''), COALESCE(a.due_date, ''), COALESCE(a.owner_id, 0), COALESCE(a.version, 0), e.action_version, e.quote_status, e.quote_amount, e.quote_date_sent, COALESCE(e.assignee_id, 0), e.agreement_recorded, COALESCE(e.agreement_file_id, 0) FROM engagements e LEFT JOIN engagement_next_actions a ON a.workspace_id = e.workspace_id AND a.engagement_id = e.id WHERE e.workspace_id = ? AND e.client_id = ? ORDER BY e.id DESC`), workspaceID, clientID)
	if err != nil {
		return nil, err
	}
	var values []Engagement
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
