package sharing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const (
	DefaultLifetime = 7 * 24 * time.Hour
	MaximumLifetime = 30 * 24 * time.Hour
	linkTokenBytes  = 32
)

var (
	ErrForbidden    = identity.ErrForbidden
	ErrInvalidInput = errors.New("public share input is invalid")
	ErrNotFound     = errors.New("public share not found")
	ErrUnavailable  = errors.New("public share is unavailable")
)

type Link struct {
	ID          string
	WorkspaceID string
	ItemID      string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	Token       string
}

type PublicItem struct {
	Title  string
	Status string
}

type Service struct {
	db     *sql.DB
	driver config.DatabaseDriver
	auth   *identity.Service
	now    func() time.Time
}

func NewService(db *sql.DB, driver config.DatabaseDriver, auth *identity.Service) *Service {
	return &Service{db: db, driver: driver, auth: auth, now: time.Now}
}

func (s *Service) Create(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	itemID string,
	lifetime time.Duration,
	audit identity.AuditContext,
) (Link, error) {
	var err error
	lifetime, err = normalizeLifetime(lifetime)
	if err != nil {
		return Link{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Link{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return Link{}, ErrForbidden
	}
	var exists int
	if err := s.queryRow(ctx, tx, "SELECT 1 FROM items WHERE workspace_id = ? AND id = ?", workspaceID, itemID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	} else if err != nil {
		return Link{}, err
	}
	linkID, err := id.New()
	if err != nil {
		return Link{}, err
	}
	token, err := newToken()
	if err != nil {
		return Link{}, err
	}
	now := s.now().UTC()
	link := Link{ID: linkID, WorkspaceID: workspaceID, ItemID: itemID, CreatedAt: now, ExpiresAt: now.Add(lifetime), Token: token}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO public_links (id, workspace_id, item_id, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, link.ID, link.WorkspaceID, link.ItemID, identity.SecretHash(token), stamp(now), stamp(link.ExpiresAt)); err != nil {
		return Link{}, err
	}
	if err := s.audit(ctx, tx, identity.AuditEvent{
		EventType: "public_share.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: workspaceID, TargetType: "public_share", TargetID: link.ID,
		Action: "public_share.create", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"item_id": itemID}), CreatedAt: now,
	}); err != nil {
		return Link{}, err
	}
	if err := tx.Commit(); err != nil {
		return Link{}, err
	}
	return link, nil
}

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID string) ([]Link, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return nil, ErrForbidden
	}
	now := stamp(s.now())
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT id, workspace_id, item_id, created_at, expires_at
		FROM public_links
		WHERE workspace_id = ? AND revoked_at IS NULL AND expires_at > ?
		ORDER BY created_at, id
	`), workspaceID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := make([]Link, 0)
	for rows.Next() {
		var link Link
		var createdAt, expiresAt string
		if err := rows.Scan(&link.ID, &link.WorkspaceID, &link.ItemID, &createdAt, &expiresAt); err != nil {
			return nil, err
		}
		link.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		link.ExpiresAt, err = parseTime(expiresAt)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

func (s *Service) Revoke(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	itemID string,
	linkID string,
	audit identity.AuditContext,
) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return ErrForbidden
	}
	now := s.now().UTC()
	result, err := s.exec(ctx, tx, `
		UPDATE public_links SET revoked_at = ?
		WHERE id = ? AND workspace_id = ? AND item_id = ? AND revoked_at IS NULL
	`, stamp(now), linkID, workspaceID, itemID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrNotFound
	}
	if err := s.audit(ctx, tx, identity.AuditEvent{
		EventType: "public_share.revoked", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: workspaceID, TargetType: "public_share", TargetID: linkID,
		Action: "public_share.revoke", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"item_id": itemID}), CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Resolve(ctx context.Context, token string, audit identity.AuditContext) (PublicItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PublicItem{}, err
	}
	defer tx.Rollback()
	var linkID, workspaceID, itemID, title, status, expiresAt string
	var revokedAt sql.NullString
	err = s.queryRow(ctx, tx, `
		SELECT p.id, p.workspace_id, p.item_id, p.expires_at, p.revoked_at, i.title, i.status
		FROM public_links p JOIN items i ON i.id = p.item_id AND i.workspace_id = p.workspace_id
		WHERE p.token_hash = ?
	`, identity.SecretHash(token)).Scan(&linkID, &workspaceID, &itemID, &expiresAt, &revokedAt, &title, &status)
	now := s.now().UTC()
	outcome := "success"
	if errors.Is(err, sql.ErrNoRows) {
		outcome = "not_found"
	} else if err != nil {
		return PublicItem{}, err
	} else if revokedAt.Valid {
		outcome = "revoked"
	} else if expiry, parseErr := parseTime(expiresAt); parseErr != nil {
		return PublicItem{}, parseErr
	} else if !now.Before(expiry) {
		outcome = "expired"
	}
	if auditErr := s.audit(ctx, tx, identity.AuditEvent{
		EventType: "public_share.accessed", AuthMethod: "public", WorkspaceID: workspaceID,
		TargetType: "public_share", TargetID: linkID, Action: "public_share.access",
		Outcome: outcome, RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: metadata(map[string]any{"item_id": itemID}), CreatedAt: now,
	}); auditErr != nil {
		return PublicItem{}, auditErr
	}
	if err := tx.Commit(); err != nil {
		return PublicItem{}, err
	}
	if outcome != "success" {
		return PublicItem{}, ErrUnavailable
	}
	return PublicItem{Title: title, Status: status}, nil
}

func newToken() (string, error) {
	value := make([]byte, linkTokenBytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate public share token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func normalizeLifetime(value time.Duration) (time.Duration, error) {
	if value == 0 {
		return DefaultLifetime, nil
	}
	if value < time.Minute || value > MaximumLifetime {
		return 0, ErrInvalidInput
	}
	return value, nil
}

func metadata(value map[string]any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func (s *Service) bind(query string) string { return database.Rebind(s.driver, query) }

func (s *Service) exec(ctx context.Context, tx *sql.Tx, query string, args ...any) (sql.Result, error) {
	return tx.ExecContext(ctx, s.bind(query), args...)
}

func (s *Service) queryRow(ctx context.Context, tx *sql.Tx, query string, args ...any) *sql.Row {
	return tx.QueryRowContext(ctx, s.bind(query), args...)
}

func stamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func (s *Service) audit(ctx context.Context, tx *sql.Tx, event identity.AuditEvent) error {
	return s.auth.RecordAuditInTx(ctx, tx, event)
}
