package portability

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var (
	ErrLimit = errors.New("workspace export limit reached")
)

const FormatVersion = "dynamis-code.workspace/v1"

type Export struct {
	FormatVersion string    `json:"formatVersion"`
	ExportedAt    time.Time `json:"exportedAt"`
	Workspace     Workspace `json:"workspace"`
	Members       []Member  `json:"members"`
	// ClientExport adds the additive client keys. It is nil, and the keys are
	// absent, unless a ClientSource is wired (the Clients profile).
	*ClientExport
	AuditEvents []AuditEvent `json:"auditEvents"`
	Excluded    []string     `json:"excluded"`
}

// ClientSource supplies the client section of an export. Only the Clients
// profile implements it. Bootstrap passes it to NewService, and generation
// without that profile passes nil.
type ClientSource interface {
	ExportClients(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID, limit int) (ClientExport, error)
}

// ClientExport is the additive client section of dynamis-code.workspace/v1.
// Its keys are documented in docs/data-lifecycle.md.
type ClientExport struct {
	Clients            []ClientRecord      `json:"clients"`
	ClientFields       []ClientField       `json:"clientFields"`
	ClientFieldOptions []ClientFieldOption `json:"clientFieldOptions"`
}

// The client record types are declared here, not in the Clients package, so
// this package does not import internal/clients. The generator prunes that
// package from apps without the Clients profile, and the export ships in every
// app. IDs are short IDs.

// ClientRecord is one client. CustomValues is keyed by field short ID.
type ClientRecord struct {
	ID              string               `json:"id"`
	WorkspaceID     identity.WorkspaceID `json:"workspaceId"`
	CreatedByUserID *identity.UserID     `json:"createdByUserId"`
	DisplayName     string               `json:"displayName"`
	Email           string               `json:"email"`
	Phone           string               `json:"phone"`
	Notes           string               `json:"notes"`
	ArchivedAt      *time.Time           `json:"archivedAt"`
	Version         int64                `json:"version"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	CustomValues    map[string]string    `json:"customValues"`
}

// ClientField is a workspace-defined custom field, archived ones included.
type ClientField struct {
	ID          string               `json:"id"`
	WorkspaceID identity.WorkspaceID `json:"workspaceId"`
	Label       string               `json:"label"`
	Type        string               `json:"type"`
	ArchivedAt  *time.Time           `json:"archivedAt"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

// ClientFieldOption is one choice of a single-choice client field.
type ClientFieldOption struct {
	ID        string    `json:"id"`
	FieldID   string    `json:"fieldId"`
	Label     string    `json:"label"`
	CreatedAt time.Time `json:"createdAt"`
}

type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Locale    string    `json:"locale"`
	CreatedAt time.Time `json:"createdAt"`
}

type Member struct {
	UserID    identity.UserID `json:"userId"`
	Email     string          `json:"email"`
	Role      identity.Role   `json:"role"`
	CreatedAt time.Time       `json:"createdAt"`
}

type AuditEvent struct {
	ID            string               `json:"id"`
	EventType     string               `json:"eventType"`
	ActorUserID   string               `json:"actorUserId,omitempty"`
	AuthMethod    string               `json:"authMethod"`
	WorkspaceID   identity.WorkspaceID `json:"workspaceId,omitempty"`
	TargetType    string               `json:"targetType"`
	TargetID      string               `json:"targetId,omitempty"`
	Action        string               `json:"action"`
	Outcome       string               `json:"outcome"`
	RequestID     string               `json:"requestId,omitempty"`
	SourceAddress string               `json:"sourceAddress,omitempty"`
	Metadata      json.RawMessage      `json:"metadata"`
	CreatedAt     time.Time            `json:"createdAt"`
}

type Service struct {
	db         *sql.DB
	driver     config.DatabaseDriver
	identity   *identity.Service
	clients    ClientSource
	maxRecords int
	maxBytes   int
	now        func() time.Time
}

// NewService takes clients as the client section source for exports. Apps
// without the Clients profile pass nil, and exports then omit the section.
func NewService(
	db *sql.DB,
	driver config.DatabaseDriver,
	identityService *identity.Service,
	maxRecords int,
	maxBytes int,
	clients ClientSource,
) *Service {
	return &Service{
		db: db, driver: driver, identity: identityService,
		maxRecords: maxRecords, maxBytes: maxBytes,
		clients: clients, now: time.Now,
	}
}

func (s *Service) Export(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
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
		Members: []Member{}, AuditEvents: []AuditEvent{},
		Excluded: []string{
			"credentials", "sessions", "apiTokens", "externalIdentities",
			"invitations", "oidcTransactions", "idempotencyRecords", "realtimeReplay",
			"notifications", "notificationPreferences",
			"webhooks", "webhookDeliveries", "backgroundJobs",
		},
	}
	var createdAt string
	var workspaceKey identity.WorkspaceID
	if err := s.queryRow(ctx, tx,
		"SELECT id, name, locale, created_at FROM workspaces WHERE id = ?", workspaceID,
	).Scan(&workspaceKey, &result.Workspace.Name, &result.Workspace.Locale, &createdAt); err != nil {
		return nil, err
	}
	result.Workspace.ID = workspaceKey.String()
	result.Workspace.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	if result.Members, err = s.members(ctx, tx, workspaceID); err != nil {
		return nil, err
	}
	if s.clients != nil {
		section, err := s.clients.ExportClients(ctx, tx, workspaceID, s.maxRecords+1)
		if err != nil {
			return nil, err
		}
		result.ClientExport = &section
	}
	if result.AuditEvents, err = s.auditEvents(ctx, tx, workspaceID); err != nil {
		return nil, err
	}
	records := 1 + len(result.Members) + len(result.AuditEvents)
	if result.ClientExport != nil {
		records += len(result.Clients) + len(result.ClientFields) + len(result.ClientFieldOptions)
	}
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

func (s *Service) members(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID) ([]Member, error) {
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

func (s *Service) auditEvents(ctx context.Context, tx *sql.Tx, workspaceID identity.WorkspaceID) ([]AuditEvent, error) {
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT id, event_type, actor_user_id, auth_method, workspace_id,
			target_type, target_key, action, outcome, request_id,
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
		var actor sql.NullInt64
		var target sql.NullInt64
		var requestID, source sql.NullString
		var scope sql.NullInt64
		var metadata, createdAt string
		var key int64
		if err := rows.Scan(
			&key, &value.EventType, &actor, &value.AuthMethod, &scope,
			&value.TargetType, &target, &value.Action, &value.Outcome, &requestID,
			&source, &metadata, &createdAt,
		); err != nil {
			return nil, err
		}
		value.ID = id.EncodeKey(id.TypeAuditEvent, key)
		if actor.Valid {
			value.ActorUserID = identity.UserID(actor.Int64).String()
		}
		if target.Valid {
			// The short ID is derived from the row's type and key; key-less targets have none.
			value.TargetID = id.EncodeKey(id.TypeCode(value.TargetType), target.Int64)
		}
		if scope.Valid {
			value.WorkspaceID = identity.WorkspaceID(scope.Int64)
		}
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
	workspaceID identity.WorkspaceID,
	outcome string,
	audit identity.AuditContext,
	now time.Time,
) error {
	return s.identity.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "workspace.exported", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "workspace", TargetKey: int64(workspaceID), Action: "workspace.export",
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
