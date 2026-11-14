// Package clients owns client records: the four common fields, validation,
// authorization, and audit. Custom fields arrive in later tickets.
package clients

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

var (
	ErrInvalidInput        = errors.New("client input is invalid")
	ErrDisplayNameRequired = fmt.Errorf("%w: display name is required", ErrInvalidInput)
	ErrInvalidEmail        = fmt.Errorf("%w: email is invalid", ErrInvalidInput)
	ErrNotFound            = errors.New("client not found")
	ErrPreconditionFailed  = errors.New("client version does not match")
	ErrInvalidCursor       = errors.New("cursor is invalid")
)

const (
	maxDisplayNameRunes = 200
	maxEmailBytes       = 254
	maxPhoneRunes       = 50
	maxNotesRunes       = 5000
)

type Client struct {
	ID              ClientID             `json:"id"`
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
}

type ListInput struct {
	Limit   int
	Cursor  string
	Sort    SortInput
	Filters []FilterInput
	// Archived is ArchivedActive when empty, ArchivedOnly, or ArchivedAll.
	Archived string
}

// Page is one keyset page of clients, newest first. The cursor is opaque.
type Page struct {
	Clients    []Client `json:"clients"`
	NextCursor string   `json:"nextCursor,omitempty"`
}

// Input carries the common fields. Email, phone, and notes may be empty.
type Input struct {
	DisplayName string
	Email       string
	Phone       string
	Notes       string
}

// UpdateInput changes only the fields it sets. Nil leaves a field as it was.
type UpdateInput struct {
	DisplayName *string
	Email       *string
	Phone       *string
	Notes       *string
}

// EventPublisher writes a webhook event inside the caller's transaction. The
// webhooks service implements it.
type EventPublisher interface {
	PublishInTx(context.Context, *sql.Tx, identity.WorkspaceID, string, []byte, time.Time) error
}

type Service struct {
	db        *sql.DB
	driver    config.DatabaseDriver
	auth      *identity.Service
	now       func() time.Time
	maxFields int
	publisher EventPublisher
}

// NewService takes at most one publisher. Without one, client changes emit no webhook events.
func NewService(
	db *sql.DB,
	driver config.DatabaseDriver,
	auth *identity.Service,
	maxFields int,
	publishers ...EventPublisher,
) *Service {
	var publisher EventPublisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{db: db, driver: driver, auth: auth, now: time.Now, maxFields: maxFields, publisher: publisher}
}

func (s *Service) Create(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	input Input,
	audit identity.AuditContext,
) (Client, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return Client{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return Client{}, identity.ErrForbidden
	}
	now := s.now().UTC()
	client := Client{
		WorkspaceID: workspaceID, CreatedByUserID: &actor.UserID,
		DisplayName: normalized.DisplayName, Email: normalized.Email,
		Phone: normalized.Phone, Notes: normalized.Notes,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.queryRow(ctx, tx, `
		INSERT INTO clients (
			workspace_id, created_by_user_id, display_name, email, phone, notes,
			version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, workspaceID, actor.UserID, client.DisplayName, client.Email, client.Phone,
		client.Notes, client.Version, formatTime(now), formatTime(now)).Scan(&client.ID); err != nil {
		return Client{}, err
	}
	if err := s.recordChange(ctx, tx, workspaceID, client.ID, "created", client.Version, now); err != nil {
		return Client{}, err
	}
	if err := s.publishEvent(ctx, tx, workspaceID, "client.created", client, now); err != nil {
		return Client{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "client.created", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "client", TargetKey: int64(client.ID), Action: "client.create",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	return client, nil
}

func (s *Service) Get(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
) (Client, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesRead,
	); err != nil {
		return Client{}, identity.ErrForbidden
	}
	client, err := s.get(ctx, tx, workspaceID, clientID)
	if err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	return client, nil
}

// Update applies a partial change when version matches the stored client.
// Writers are Member, Admin, and Owner; Viewer is refused.
func (s *Service) Update(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	clientID ClientID,
	version int64,
	input UpdateInput,
	audit identity.AuditContext,
) (Client, error) {
	if version < 1 || (input.DisplayName == nil && input.Email == nil && input.Phone == nil && input.Notes == nil) {
		return Client{}, ErrInvalidInput
	}
	if err := normalizeUpdate(&input); err != nil {
		return Client{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Client{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return Client{}, identity.ErrForbidden
	}
	client, err := s.get(ctx, tx, workspaceID, clientID)
	if err != nil {
		return Client{}, err
	}
	if client.Version != version {
		return Client{}, ErrPreconditionFailed
	}
	if input.DisplayName != nil {
		client.DisplayName = *input.DisplayName
	}
	if input.Email != nil {
		client.Email = *input.Email
	}
	if input.Phone != nil {
		client.Phone = *input.Phone
	}
	if input.Notes != nil {
		client.Notes = *input.Notes
	}
	client.Version++
	client.UpdatedAt = s.now().UTC()
	result, err := s.exec(ctx, tx, `
		UPDATE clients SET display_name = ?, email = ?, phone = ?, notes = ?, version = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ? AND version = ?
	`, client.DisplayName, client.Email, client.Phone, client.Notes, client.Version,
		formatTime(client.UpdatedAt), workspaceID, clientID, version)
	if err != nil {
		return Client{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Client{}, ErrPreconditionFailed
	}
	if err := s.recordChange(ctx, tx, workspaceID, client.ID, "updated", client.Version, client.UpdatedAt); err != nil {
		return Client{}, err
	}
	if err := s.publishEvent(ctx, tx, workspaceID, "client.updated", client, client.UpdatedAt); err != nil {
		return Client{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "client.updated", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "client", TargetKey: int64(client.ID), Action: "client.update",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      fmt.Sprintf(`{"version":%d}`, client.Version),
		CreatedAt:     client.UpdatedAt,
	}); err != nil {
		return Client{}, err
	}
	if err := tx.Commit(); err != nil {
		return Client{}, err
	}
	return client, nil
}

// normalizeUpdate applies the same rules as create to each field the update sets.
func normalizeUpdate(input *UpdateInput) error {
	if input.DisplayName != nil {
		value, err := validDisplayName(*input.DisplayName)
		if err != nil {
			return err
		}
		input.DisplayName = &value
	}
	if input.Email != nil {
		value, err := validEmail(*input.Email)
		if err != nil {
			return err
		}
		input.Email = &value
	}
	if input.Phone != nil {
		value, err := validText(*input.Phone, maxPhoneRunes)
		if err != nil {
			return err
		}
		input.Phone = &value
	}
	if input.Notes != nil {
		value, err := validText(*input.Notes, maxNotesRunes)
		if err != nil {
			return err
		}
		input.Notes = &value
	}
	return nil
}

// List returns one page of clients. The archived selector picks active (the
// default), archived, or all clients. Without a sort the page is newest first.
// A sort orders by one field with empty values last, and a filter narrows the
// rows. The cursor binds the sort, filters, and archived selector.
func (s *Service) List(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	input ListInput,
) (Page, error) {
	if input.Limit < 1 {
		return Page{}, ErrInvalidInput
	}
	cursor, err := decodeListCursor(input.Cursor)
	if err != nil {
		return Page{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return Page{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesRead,
	); err != nil {
		return Page{}, identity.ErrForbidden
	}
	query, err := s.buildListQuery(ctx, tx, workspaceID, input, cursor)
	if err != nil {
		return Page{}, err
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, query.sql), append(query.args, input.Limit+1)...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	clients := make([]Client, 0, input.Limit+1)
	sortValues := make([]sql.NullString, 0, input.Limit+1)
	for rows.Next() {
		var sortValue sql.NullString
		client, err := scanClient(listRow{rows: rows, sortValue: &sortValue})
		if err != nil {
			return Page{}, err
		}
		clients = append(clients, client)
		sortValues = append(sortValues, sortValue)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if err := rows.Close(); err != nil {
		return Page{}, err
	}
	page := Page{Clients: clients}
	if len(clients) > input.Limit {
		page.Clients = clients[:input.Limit]
		last := input.Limit - 1
		page.NextCursor, err = encodeListCursor(query, input, sortValues[last], clients[last].ID)
		if err != nil {
			return Page{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Page{}, err
	}
	return page, nil
}

func (s *Service) get(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	workspaceID identity.WorkspaceID,
	clientID ClientID,
) (Client, error) {
	if clientID == 0 {
		return Client{}, ErrNotFound
	}
	client, err := scanClient(s.queryRow(ctx, queryer, `
		SELECT id, workspace_id, created_by_user_id, display_name, email, phone, notes,
			version, created_at, updated_at, archived_at
		FROM clients
		WHERE workspace_id = ? AND id = ?
	`, workspaceID, clientID))
	if errors.Is(err, sql.ErrNoRows) {
		return Client{}, ErrNotFound
	}
	return client, err
}

type scanner interface {
	Scan(...any) error
}

func scanClient(row scanner) (Client, error) {
	var client Client
	var createdByUserID sql.NullInt64
	var createdAt, updatedAt string
	var archivedAt sql.NullString
	if err := row.Scan(
		&client.ID, &client.WorkspaceID, &createdByUserID, &client.DisplayName,
		&client.Email, &client.Phone, &client.Notes, &client.Version, &createdAt, &updatedAt,
		&archivedAt,
	); err != nil {
		return Client{}, err
	}
	if createdByUserID.Valid {
		creator := identity.UserID(createdByUserID.Int64)
		client.CreatedByUserID = &creator
	}
	var err error
	if client.CreatedAt, err = parseTime(createdAt); err != nil {
		return Client{}, err
	}
	if client.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return Client{}, err
	}
	if archivedAt.Valid {
		value, err := parseTime(archivedAt.String)
		if err != nil {
			return Client{}, err
		}
		client.ArchivedAt = &value
	}
	return client, nil
}

// normalizeInput trims the common fields and refuses values outside the rules.
func normalizeInput(input Input) (Input, error) {
	displayName, err := validDisplayName(input.DisplayName)
	if err != nil {
		return Input{}, err
	}
	email, err := validEmail(input.Email)
	if err != nil {
		return Input{}, err
	}
	phone, err := validText(input.Phone, maxPhoneRunes)
	if err != nil {
		return Input{}, err
	}
	notes, err := validText(input.Notes, maxNotesRunes)
	if err != nil {
		return Input{}, err
	}
	return Input{DisplayName: displayName, Email: email, Phone: phone, Notes: notes}, nil
}

func validDisplayName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrDisplayNameRequired
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxDisplayNameRunes {
		return "", ErrInvalidInput
	}
	return value, nil
}

// validEmail allows an empty value. A non-empty value must be a bare address,
// so a display-name form such as "Ada <ada@example.com>" is refused.
func validEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > maxEmailBytes || !utf8.ValidString(value) {
		return "", ErrInvalidEmail
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return "", ErrInvalidEmail
	}
	return value, nil
}

func validText(value string, maxRunes int) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes {
		return "", ErrInvalidInput
	}
	return value, nil
}

func (s *Service) exec(
	ctx context.Context,
	executor interface {
		ExecContext(context.Context, string, ...any) (sql.Result, error)
	},
	query string,
	args ...any,
) (sql.Result, error) {
	return executor.ExecContext(ctx, database.Rebind(s.driver, query), args...)
}

func formatTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func (s *Service) queryRow(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	query string,
	args ...any,
) *sql.Row {
	return queryer.QueryRowContext(ctx, database.Rebind(s.driver, query), args...)
}
