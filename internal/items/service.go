package items

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
)

var (
	ErrIdempotencyConflict = errors.New("idempotency key was used for another request")
	ErrInvalidCursor       = errors.New("cursor is invalid")
	ErrInvalidInput        = errors.New("item input is invalid")
	ErrNotFound            = errors.New("item not found")
	ErrPreconditionFailed  = errors.New("item version does not match")
	ErrLimit               = errors.New("workspace item limit reached")
)

const idempotencyRetention = 24 * time.Hour
const maximumSearchLength = 100

type Status string

const (
	Active   Status = "active"
	Complete Status = "complete"
)

type Item struct {
	ID              string    `json:"id"`
	WorkspaceID     string    `json:"workspaceId"`
	CreatedByUserID string    `json:"createdByUserId"`
	Title           string    `json:"title"`
	Status          Status    `json:"status"`
	Version         int64     `json:"version"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type CreateResult struct {
	Item   Item
	Replay bool
}

type UpdateInput struct {
	Title  *string
	Status *Status
}

type ListInput struct {
	Status Status
	Search string
	Sort   string
	Limit  int
	Cursor string
}

type Page struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type Service struct {
	db       *sql.DB
	driver   config.DatabaseDriver
	auth     *identity.Service
	now      func() time.Time
	maxItems int
}

func NewService(
	db *sql.DB,
	driver config.DatabaseDriver,
	auth *identity.Service,
	maxItems int,
) *Service {
	return &Service{db: db, driver: driver, auth: auth, now: time.Now, maxItems: maxItems}
}

func (s *Service) Create(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	title string,
	idempotencyKey string,
	audit identity.AuditContext,
) (CreateResult, error) {
	title, err := validateTitle(title)
	if err != nil || !validIdempotencyKey(idempotencyKey) {
		return CreateResult{}, ErrInvalidInput
	}
	if _, err := s.auth.AuthorizePrincipal(
		ctx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return CreateResult{}, identity.ErrForbidden
	}
	return s.create(ctx, actor, workspaceID, title, idempotencyKey, audit, true)
}

func (s *Service) create(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	title string,
	idempotencyKey string,
	audit identity.AuditContext,
	retry bool,
) (CreateResult, error) {
	now := s.now().UTC()
	keyHash := hash(idempotencyKey)
	requestHash := hash(title)
	principalID := principalID(actor)
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return CreateResult{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return CreateResult{}, identity.ErrForbidden
	}
	var storedRequestHash, resultJSON, expiresAt string
	err = s.queryRow(ctx, tx, `
		SELECT request_hash, result_json, expires_at
		FROM idempotency_records
		WHERE key_hash = ? AND principal_id = ?
		AND workspace_id = ? AND operation = ?
	`, keyHash, principalID, workspaceID, "items.create").Scan(
		&storedRequestHash, &resultJSON, &expiresAt,
	)
	if err == nil {
		expires, parseErr := parseTime(expiresAt)
		if parseErr != nil {
			return CreateResult{}, parseErr
		}
		if now.Before(expires) {
			if storedRequestHash != requestHash {
				return CreateResult{}, ErrIdempotencyConflict
			}
			var item Item
			if err := json.Unmarshal([]byte(resultJSON), &item); err != nil {
				return CreateResult{}, err
			}
			return CreateResult{Item: item, Replay: true}, nil
		}
		if _, err := s.exec(ctx, tx, `
			DELETE FROM idempotency_records
			WHERE key_hash = ? AND principal_id = ?
			AND workspace_id = ? AND operation = ?
		`, keyHash, principalID, workspaceID, "items.create"); err != nil {
			return CreateResult{}, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return CreateResult{}, err
	}
	var itemCount int
	if err := s.queryRow(ctx, tx,
		"SELECT COUNT(*) FROM items WHERE workspace_id = ?", workspaceID,
	).Scan(&itemCount); err != nil {
		return CreateResult{}, err
	}
	if itemCount >= s.maxItems {
		telemetry.RecordLimitRejection(ctx, "workspace_items")
		return CreateResult{}, ErrLimit
	}

	itemID, err := id.New()
	if err != nil {
		return CreateResult{}, err
	}
	item := Item{
		ID: itemID, WorkspaceID: workspaceID,
		CreatedByUserID: actor.UserID, Title: title, Status: Active,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO items (
			id, workspace_id, created_by_user_id, title, status,
			version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.WorkspaceID, item.CreatedByUserID, item.Title,
		item.Status, item.Version, formatTime(now), formatTime(now)); err != nil {
		return CreateResult{}, err
	}
	if err := s.recordChange(
		ctx, tx, workspaceID, item.ID, "created", item.Version, now,
	); err != nil {
		return CreateResult{}, err
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		return CreateResult{}, err
	}
	result, err := s.exec(ctx, tx, `
		INSERT INTO idempotency_records (
			key_hash, principal_id, workspace_id, operation, request_hash,
			result_json, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (key_hash, principal_id, workspace_id, operation) DO NOTHING
	`, keyHash, principalID, workspaceID, "items.create", requestHash,
		string(encoded), formatTime(now), formatTime(now.Add(idempotencyRetention)))
	if err != nil {
		return CreateResult{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		if !retry {
			return CreateResult{}, ErrIdempotencyConflict
		}
		if err := tx.Rollback(); err != nil {
			return CreateResult{}, err
		}
		return s.create(
			ctx, actor, workspaceID, title, idempotencyKey, audit, false,
		)
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "item.created", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "item", TargetID: item.ID, Action: "item.create",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return CreateResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Item: item}, nil
}

func (s *Service) Get(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	itemID string,
) (Item, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesRead,
	); err != nil {
		return Item{}, identity.ErrForbidden
	}
	item, err := s.get(ctx, tx, workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Service) List(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	input ListInput,
) (Page, error) {
	search, err := validateSearch(input.Search)
	if err != nil {
		return Page{}, ErrInvalidInput
	}
	input.Search = search
	if input.Limit < 1 || input.Limit > 100 ||
		(input.Status != "" && input.Status != Active && input.Status != Complete) ||
		(input.Sort != "created_at" && input.Sort != "-created_at") {
		return Page{}, ErrInvalidInput
	}
	position, err := decodeCursor(input.Cursor, input)
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
	query := `
		SELECT id, workspace_id, created_by_user_id, title, status,
			version, created_at, updated_at
		FROM items WHERE workspace_id = ?
	`
	args := []any{workspaceID}
	if input.Status != "" {
		query += " AND status = ?"
		args = append(args, input.Status)
	}
	if input.Search != "" {
		query += " AND LOWER(title) LIKE LOWER(?) ESCAPE '\\'"
		args = append(args, searchPattern(input.Search))
	}
	comparison := ">"
	direction := "ASC"
	if input.Sort == "-created_at" {
		comparison = "<"
		direction = "DESC"
	}
	if position != nil {
		query += fmt.Sprintf(
			" AND (created_at %s ? OR (created_at = ? AND id %s ?))",
			comparison, comparison,
		)
		args = append(args, position.CreatedAt, position.CreatedAt, position.ID)
	}
	query += " ORDER BY created_at " + direction + ", id " + direction + " LIMIT ?"
	args = append(args, input.Limit+1)
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, query), args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := make([]Item, 0, input.Limit+1)
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return Page{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if err := rows.Close(); err != nil {
		return Page{}, err
	}
	page := Page{Items: items}
	if len(items) > input.Limit {
		last := items[input.Limit-1]
		page.Items = items[:input.Limit]
		page.NextCursor, err = encodeCursor(input, last)
		if err != nil {
			return Page{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Page{}, err
	}
	return page, nil
}

func (s *Service) Update(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	itemID string,
	version int64,
	input UpdateInput,
	audit identity.AuditContext,
) (Item, error) {
	if version < 1 || (input.Title == nil && input.Status == nil) {
		return Item{}, ErrInvalidInput
	}
	if input.Title != nil {
		title, err := validateTitle(*input.Title)
		if err != nil {
			return Item{}, ErrInvalidInput
		}
		input.Title = &title
	}
	if input.Status != nil && *input.Status != Active && *input.Status != Complete {
		return Item{}, ErrInvalidInput
	}
	if _, err := s.auth.AuthorizePrincipal(
		ctx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return Item{}, identity.ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Item{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return Item{}, identity.ErrForbidden
	}
	item, err := s.get(ctx, tx, workspaceID, itemID)
	if err != nil {
		return Item{}, err
	}
	if item.Version != version {
		return Item{}, ErrPreconditionFailed
	}
	if input.Title != nil {
		item.Title = *input.Title
	}
	if input.Status != nil {
		item.Status = *input.Status
	}
	item.Version++
	item.UpdatedAt = s.now().UTC()
	result, err := s.exec(ctx, tx, `
		UPDATE items SET title = ?, status = ?, version = ?, updated_at = ?
		WHERE workspace_id = ? AND id = ? AND version = ?
	`, item.Title, item.Status, item.Version, formatTime(item.UpdatedAt),
		workspaceID, itemID, version)
	if err != nil {
		return Item{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Item{}, ErrPreconditionFailed
	}
	if err := s.recordChange(
		ctx, tx, workspaceID, item.ID, "updated", item.Version, item.UpdatedAt,
	); err != nil {
		return Item{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "item.updated", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "item", TargetID: item.ID, Action: "item.update",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      fmt.Sprintf(`{"version":%d}`, item.Version),
		CreatedAt:     item.UpdatedAt,
	}); err != nil {
		return Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Service) Delete(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	itemID string,
	version int64,
	audit identity.AuditContext,
) error {
	if version < 1 {
		return ErrInvalidInput
	}
	if _, err := s.auth.AuthorizePrincipal(
		ctx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return identity.ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(
		ctx, tx, actor, workspaceID, identity.ResourcesWrite,
	); err != nil {
		return identity.ErrForbidden
	}
	item, err := s.get(ctx, tx, workspaceID, itemID)
	if err != nil {
		return err
	}
	if item.Version != version {
		return ErrPreconditionFailed
	}
	result, err := s.exec(ctx, tx, `
		DELETE FROM items WHERE workspace_id = ? AND id = ? AND version = ?
	`, workspaceID, itemID, version)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrPreconditionFailed
	}
	now := s.now().UTC()
	if err := s.recordChange(
		ctx, tx, workspaceID, item.ID, "deleted", item.Version+1, now,
	); err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "item.deleted", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "item", TargetID: item.ID, Action: "item.delete",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      fmt.Sprintf(`{"version":%d,"permanent":true}`, item.Version),
		CreatedAt:     now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) get(
	ctx context.Context,
	queryer interface {
		QueryRowContext(context.Context, string, ...any) *sql.Row
	},
	workspaceID string,
	itemID string,
) (Item, error) {
	item, err := scanItem(s.queryRow(ctx, queryer, `
		SELECT id, workspace_id, created_by_user_id, title, status,
			version, created_at, updated_at
		FROM items WHERE workspace_id = ? AND id = ?
	`, workspaceID, itemID))
	if errors.Is(err, sql.ErrNoRows) {
		return Item{}, ErrNotFound
	}
	return item, err
}

type scanner interface {
	Scan(...any) error
}

func scanItem(row scanner) (Item, error) {
	var item Item
	var createdAt, updatedAt string
	err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.CreatedByUserID,
		&item.Title, &item.Status, &item.Version, &createdAt, &updatedAt,
	)
	if err != nil {
		return Item{}, err
	}
	item.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return Item{}, err
	}
	item.UpdatedAt, err = parseTime(updatedAt)
	return item, err
}

type cursor struct {
	Status    Status `json:"status"`
	Search    string `json:"search"`
	Sort      string `json:"sort"`
	CreatedAt string `json:"createdAt"`
	ID        string `json:"id"`
}

func encodeCursor(input ListInput, item Item) (string, error) {
	encoded, err := json.Marshal(cursor{
		Status: input.Status, Search: input.Search, Sort: input.Sort,
		CreatedAt: formatTime(item.CreatedAt), ID: item.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(encoded string, input ListInput) (*cursor, error) {
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(raw) > 512 {
		return nil, ErrInvalidCursor
	}
	var value cursor
	if err := json.Unmarshal(raw, &value); err != nil ||
		value.Status != input.Status || value.Search != input.Search ||
		value.Sort != input.Sort || !validCursorID(value.ID) {
		return nil, ErrInvalidCursor
	}
	if _, err := parseTime(value.CreatedAt); err != nil {
		return nil, ErrInvalidCursor
	}
	return &value, nil
}

func validateTitle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 200 {
		return "", ErrInvalidInput
	}
	return value, nil
}

func validCursorID(value string) bool {
	if len(value) != 32 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func validateSearch(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value != "" && (!utf8.ValidString(value) || utf8.RuneCountInString(value) > maximumSearchLength) {
		return "", ErrInvalidInput
	}
	return value, nil
}

func searchPattern(value string) string {
	value = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
	return "%" + value + "%"
}

func validIdempotencyKey(value string) bool {
	if len(value) < 1 || len(value) > 255 {
		return false
	}
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func principalID(actor identity.Principal) string {
	if actor.TokenID != "" {
		return "token:" + actor.TokenID
	}
	return "user:" + actor.UserID
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func formatTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
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
