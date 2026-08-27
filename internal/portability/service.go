package portability

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

var ErrLimit = errors.New("workspace export limit reached")

const FormatVersion = "dynamis-code.workspace/v1"

type Export struct {
	FormatVersion string       `json:"formatVersion"`
	ExportedAt    time.Time    `json:"exportedAt"`
	Workspace     Workspace    `json:"workspace"`
	Members       []Member     `json:"members"`
	Items         []items.Item `json:"items"`
	AuditEvents   []AuditEvent `json:"auditEvents"`
	Excluded      []string     `json:"excluded"`
}

type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Locale    string    `json:"locale"`
	CreatedAt time.Time `json:"createdAt"`
}

type Member struct {
	UserID    string        `json:"userId"`
	Email     string        `json:"email"`
	Role      identity.Role `json:"role"`
	CreatedAt time.Time     `json:"createdAt"`
}

type AuditEvent struct {
	ID            string          `json:"id"`
	EventType     string          `json:"eventType"`
	ActorUserID   string          `json:"actorUserId,omitempty"`
	AuthMethod    string          `json:"authMethod"`
	WorkspaceID   string          `json:"workspaceId,omitempty"`
	TargetType    string          `json:"targetType"`
	TargetID      string          `json:"targetId,omitempty"`
	Action        string          `json:"action"`
	Outcome       string          `json:"outcome"`
	RequestID     string          `json:"requestId,omitempty"`
	SourceAddress string          `json:"sourceAddress,omitempty"`
	Metadata      json.RawMessage `json:"metadata"`
	CreatedAt     time.Time       `json:"createdAt"`
}

type Service struct {
	db         *sql.DB
	driver     config.DatabaseDriver
	identity   *identity.Service
	maxRecords int
	maxBytes   int
	now        func() time.Time
}

func NewService(
	db *sql.DB,
	driver config.DatabaseDriver,
	identityService *identity.Service,
	maxRecords int,
	maxBytes int,
) *Service {
	return &Service{
		db: db, driver: driver, identity: identityService,
		maxRecords: maxRecords, maxBytes: maxBytes, now: time.Now,
	}
}

func (s *Service) Export(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	audit identity.AuditContext,
) ([]byte, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := s.identity.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.WorkspaceExport,
	); err != nil {
		return nil, identity.ErrForbidden
	}
	result := Export{
		FormatVersion: FormatVersion, ExportedAt: s.now().UTC(),
		Members: []Member{}, Items: []items.Item{}, AuditEvents: []AuditEvent{},
		Excluded: []string{
			"credentials", "sessions", "apiTokens", "externalIdentities",
			"invitations", "oidcTransactions", "idempotencyRecords", "realtimeReplay",
			"notifications", "notificationPreferences",
		},
	}
	var createdAt string
	if err := s.queryRow(ctx, tx,
		"SELECT id, name, locale, created_at FROM workspaces WHERE id = ?", workspaceID,
	).Scan(&result.Workspace.ID, &result.Workspace.Name, &result.Workspace.Locale, &createdAt); err != nil {
		return nil, err
	}
	result.Workspace.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	if result.Members, err = s.members(ctx, tx, workspaceID); err != nil {
		return nil, err
	}
	if result.Items, err = s.itemRecords(ctx, tx, workspaceID); err != nil {
		return nil, err
	}
	if result.AuditEvents, err = s.auditEvents(ctx, tx, workspaceID); err != nil {
		return nil, err
	}
	records := 1 + len(result.Members) + len(result.Items) + len(result.AuditEvents)
	encoded, encodeErr := json.Marshal(result)
	if records > s.maxRecords || (encodeErr == nil && len(encoded) > s.maxBytes) {
		if err := s.recordAudit(ctx, tx, actor, workspaceID, "failure", audit, result.ExportedAt); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, ErrLimit
	}
	if encodeErr != nil {
		return nil, encodeErr
	}
	if err := s.recordAudit(ctx, tx, actor, workspaceID, "success", audit, result.ExportedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return encoded, nil
}

func (s *Service) members(ctx context.Context, tx *sql.Tx, workspaceID string) ([]Member, error) {
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT m.user_id, u.email, m.role, m.created_at
		FROM workspace_members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = ? ORDER BY m.created_at, m.user_id LIMIT ?
	`), workspaceID, s.maxRecords+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Member, 0)
	for rows.Next() {
		var value Member
		var createdAt string
		if err := rows.Scan(&value.UserID, &value.Email, &value.Role, &createdAt); err != nil {
			return nil, err
		}
		value.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Service) itemRecords(ctx context.Context, tx *sql.Tx, workspaceID string) ([]items.Item, error) {
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT id, workspace_id, created_by_user_id, title, status,
			version, created_at, updated_at
		FROM items WHERE workspace_id = ? ORDER BY created_at, id LIMIT ?
	`), workspaceID, s.maxRecords+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]items.Item, 0)
	for rows.Next() {
		var value items.Item
		var createdAt, updatedAt string
		if err := rows.Scan(
			&value.ID, &value.WorkspaceID, &value.CreatedByUserID, &value.Title,
			&value.Status, &value.Version, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		value.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		value.UpdatedAt, err = parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Service) auditEvents(ctx context.Context, tx *sql.Tx, workspaceID string) ([]AuditEvent, error) {
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT id, event_type, actor_user_id, auth_method, workspace_id,
			target_type, target_id, action, outcome, request_id,
			source_address, metadata, created_at
		FROM audit_events WHERE workspace_id = ? ORDER BY created_at, id LIMIT ?
	`), workspaceID, s.maxRecords+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AuditEvent, 0)
	for rows.Next() {
		var value AuditEvent
		var actor, scope, target, requestID, source sql.NullString
		var metadata, createdAt string
		if err := rows.Scan(
			&value.ID, &value.EventType, &actor, &value.AuthMethod, &scope,
			&value.TargetType, &target, &value.Action, &value.Outcome, &requestID,
			&source, &metadata, &createdAt,
		); err != nil {
			return nil, err
		}
		value.ActorUserID, value.WorkspaceID, value.TargetID = actor.String, scope.String, target.String
		value.RequestID, value.SourceAddress = requestID.String, source.String
		if !json.Valid([]byte(metadata)) {
			return nil, errors.New("audit metadata is invalid")
		}
		value.Metadata = json.RawMessage(metadata)
		value.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Service) recordAudit(
	ctx context.Context,
	tx *sql.Tx,
	actor identity.Principal,
	workspaceID string,
	outcome string,
	audit identity.AuditContext,
	now time.Time,
) error {
	return s.identity.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "workspace.exported", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "workspace", TargetID: workspaceID, Action: "workspace.export",
		Outcome: outcome, RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: fmt.Sprintf(`{"format_version":%q}`, FormatVersion), CreatedAt: now,
	})
}

func (s *Service) bind(query string) string {
	return database.Rebind(s.driver, query)
}

func (s *Service) queryRow(ctx context.Context, tx *sql.Tx, query string, args ...any) *sql.Row {
	return tx.QueryRowContext(ctx, s.bind(query), args...)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
