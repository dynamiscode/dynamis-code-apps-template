package intake

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

var (
	ErrActive       = errors.New("intake link already active")
	ErrNotFound     = errors.New("intake link not found")
	ErrInvalidInput = errors.New("invalid consent text")
)

type Service struct {
	db         *sql.DB
	driver     config.DatabaseDriver
	auth       *identity.Service
	clients    *clients.Service
	engagement *engagement.Service
	queue      *jobs.Queue
	publicURL  string
}

func (s *Service) ConfigureNotifications(queue *jobs.Queue, publicURL string) {
	s.queue = queue
	s.publicURL = strings.TrimRight(publicURL, "/")
}

type Settings struct {
	Active      bool
	ConsentText string
}

func NewService(db *sql.DB, driver config.DatabaseDriver, auth *identity.Service, clientServices ...*clients.Service) *Service {
	clientService := clients.NewService(db, driver, auth, 0)
	if len(clientServices) > 0 {
		clientService = clientServices[0]
	}
	return &Service{db: db, driver: driver, auth: auth, clients: clientService, engagement: engagement.NewService(db, driver, auth)}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Create(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, consent string, audit identity.AuditContext) (string, error) {
	return s.issue(ctx, actor, workspace, consent, audit, false)
}

func (s *Service) Rotate(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, consent string, audit identity.AuditContext) (string, error) {
	return s.issue(ctx, actor, workspace, consent, audit, true)
}

func (s *Service) issue(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, consent string, audit identity.AuditContext, rotate bool) (string, error) {
	consent = strings.TrimSpace(consent)
	if consent == "" || !utf8.ValidString(consent) || utf8.RuneCountInString(consent) > 2000 {
		return "", ErrInvalidInput
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.WorkspaceUpdate); err != nil {
		return "", identity.ErrForbidden
	}
	query := `INSERT INTO public_intake_links (workspace_id, token_hash, consent_text) VALUES (?, ?, ?)
		ON CONFLICT (workspace_id) DO UPDATE SET token_hash = excluded.token_hash, consent_text = excluded.consent_text`
	if !rotate {
		query += ` WHERE public_intake_links.token_hash IS NULL`
	}
	result, err := tx.ExecContext(ctx, database.Rebind(s.driver, query), workspace, hashToken(token), consent)
	if err != nil {
		return "", err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", ErrActive
	}
	action := "created"
	if rotate {
		action = "rotated"
	}
	if err := s.audit(ctx, tx, actor, workspace, audit, action); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) Disable(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, audit identity.AuditContext) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.WorkspaceUpdate); err != nil {
		return identity.ErrForbidden
	}
	result, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE public_intake_links SET token_hash = NULL WHERE workspace_id = ? AND token_hash IS NOT NULL`), workspace)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	if err := s.audit(ctx, tx, actor, workspace, audit, "disabled"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) GetSettings(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID) (Settings, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable, ReadOnly: true})
	if err != nil {
		return Settings{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.WorkspaceUpdate); err != nil {
		return Settings{}, identity.ErrForbidden
	}
	var settings Settings
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT token_hash IS NOT NULL, consent_text FROM public_intake_links WHERE workspace_id = ?`), workspace).Scan(&settings.Active, &settings.ConsentText)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return settings, tx.Commit()
}

func (s *Service) SetConsent(ctx context.Context, actor identity.Principal, workspace identity.WorkspaceID, consent string, audit identity.AuditContext) error {
	consent = strings.TrimSpace(consent)
	if consent == "" || !utf8.ValidString(consent) || utf8.RuneCountInString(consent) > 2000 {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspace, identity.WorkspaceUpdate); err != nil {
		return identity.ErrForbidden
	}
	_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `INSERT INTO public_intake_links (workspace_id, consent_text) VALUES (?, ?) ON CONFLICT (workspace_id) DO UPDATE SET consent_text = excluded.consent_text`), workspace, consent)
	if err != nil {
		return err
	}
	if err := s.audit(ctx, tx, actor, workspace, audit, "consent_updated"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) Resolve(ctx context.Context, token string) (identity.WorkspaceID, string, error) {
	if token == "" {
		return 0, "", ErrNotFound
	}
	var workspace identity.WorkspaceID
	var consent string
	err := s.db.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT workspace_id, consent_text FROM public_intake_links WHERE token_hash = ?`), hashToken(token)).Scan(&workspace, &consent)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return workspace, consent, err
}

func (s *Service) audit(ctx context.Context, tx *sql.Tx, actor identity.Principal, workspace identity.WorkspaceID, audit identity.AuditContext, action string) error {
	return s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "public_intake." + action, ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspace, TargetType: "workspace", TargetKey: int64(workspace), Action: "public_intake." + action, Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()})
}
