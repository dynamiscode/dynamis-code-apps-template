// Package workspaceemail stores each workspace's SMTP sender and routes
// invitation mail through it. Account email never uses these settings.
package workspaceemail

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
)

const (
	maxSenderNameLength = 100
	maxHostLength       = 253
	maxUsernameLength   = 254
	maxPasswordLength   = 1024
)

var (
	ErrInvalidInput = errors.New("workspace email settings are invalid")
	ErrSecretKey    = errors.New("workspace email encryption key is unavailable")
	ErrUnreadable   = errors.New("workspace email password cannot be opened")
	// ErrPasswordRequired means the server or login changed, so the password must be re-entered.
	ErrPasswordRequired = errors.New("workspace email password must be entered when the server changes")
)

func smtpConfig(input SaveInput, password string) config.Mail {
	return config.Mail{
		Host: input.Host, Port: input.Port, Username: input.Username, Password: password, From: input.SenderAddress,
	}
}

// LoginError reports a failed SMTP login test. Reason is fixed text from the
// SMTP client and never contains the password.
type LoginError struct{ Reason string }

func (e *LoginError) Error() string { return "SMTP login test failed: " + e.Reason }

// Settings is the masked view shown to administrators.
type Settings struct {
	Configured    bool      `json:"configured"`
	SenderAddress string    `json:"senderAddress"`
	SenderName    string    `json:"senderName"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	Username      string    `json:"username"`
	PasswordSet   bool      `json:"passwordSet"`
	UpdatedAt     time.Time `json:"updatedAt,omitzero"`
}

// SaveInput is the form data. An empty Password keeps the stored one.
type SaveInput struct {
	SenderAddress string
	SenderName    string
	Host          string
	Port          int
	Username      string
	Password      string
}

type stored struct {
	SaveInput
	ciphertext string
	keyVersion int
	updatedAt  time.Time
}

type Service struct {
	db       *sql.DB
	driver   config.DatabaseDriver
	auth     *identity.Service
	secrets  secretbox.Keyring
	fallback appmail.Sender
	// login runs the SMTP login test; tests replace it to avoid a network server.
	login func(context.Context, config.Mail, string) error
}

// NewService takes the instance sender (env config) as the fallback and for account email.
func NewService(db *sql.DB, driver config.DatabaseDriver, auth *identity.Service, secrets secretbox.Keyring, fallback appmail.Sender) *Service {
	return &Service{
		db: db, driver: driver, auth: auth, secrets: secrets, fallback: fallback,
		login: func(ctx context.Context, cfg config.Mail, senderName string) error {
			sender, err := appmail.NewWorkspaceSMTP(cfg, senderName)
			if err != nil {
				return err
			}
			return sender.TestLogin(ctx)
		},
	}
}

// Send delivers account email through the instance sender only.
func (s *Service) Send(ctx context.Context, recipient, subject, body string) error {
	if s.fallback == nil {
		return appmail.ErrNotConfigured
	}
	return s.fallback.Send(ctx, recipient, subject, body)
}

// Available reports whether instance-level email is configured.
func (s *Service) Available() bool { return s.fallback != nil }

// SendFor sends invitation mail through the workspace's SMTP when configured,
// otherwise through the instance sender. The caller must already have authorized
// the invitation.
func (s *Service) SendFor(ctx context.Context, workspaceID, recipient, subject, body string) error {
	row, found, err := s.load(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found {
		return s.Send(ctx, recipient, subject, body)
	}
	password, err := s.secrets.Open(row.keyVersion, row.ciphertext)
	if err != nil {
		return ErrUnreadable
	}
	sender, err := appmail.NewWorkspaceSMTP(smtpConfig(row.SaveInput, password), row.SenderName)
	if err != nil {
		return ErrInvalidInput
	}
	return sender.Send(ctx, recipient, subject, body)
}

// Get returns the masked settings to a principal who can update the workspace.
func (s *Service) Get(ctx context.Context, actor identity.Principal, workspaceID string) (Settings, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
		return Settings{}, identity.ErrForbidden
	}
	row, found, err := s.load(ctx, workspaceID)
	if err != nil {
		return Settings{}, err
	}
	if !found {
		return Settings{}, nil
	}
	return Settings{
		Configured: true, SenderAddress: row.SenderAddress, SenderName: row.SenderName,
		Host: row.Host, Port: row.Port, Username: row.Username, PasswordSet: true, UpdatedAt: row.updatedAt,
	}, nil
}

// Save runs an SMTP login test with the submitted settings and stores them only if it passes.
func (s *Service) Save(ctx context.Context, actor identity.Principal, workspaceID string, input SaveInput, audit identity.AuditContext) error {
	input, err := normalize(input)
	if err != nil {
		return err
	}
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
		return identity.ErrForbidden
	}
	existing, found, err := s.load(ctx, workspaceID)
	if err != nil {
		return err
	}
	ciphertext, keyVersion, password, err := s.resolvePassword(input, existing, found)
	if err != nil {
		return err
	}
	cfg := smtpConfig(input, password)
	if _, err := appmail.NewWorkspaceSMTP(cfg, input.SenderName); err != nil {
		return ErrInvalidInput
	}
	if err := s.login(ctx, cfg, input.SenderName); err != nil {
		return &LoginError{Reason: err.Error()}
	}
	if input.Password != "" {
		ciphertext, keyVersion, err = s.secrets.Seal(input.Password)
		if err != nil {
			return ErrSecretKey
		}
	}
	return s.store(ctx, actor, workspaceID, input, ciphertext, keyVersion, found, input.Password != "", audit)
}

func (s *Service) resolvePassword(input SaveInput, existing stored, found bool) (string, int, string, error) {
	if input.Password != "" {
		if !s.secrets.Configured() {
			return "", 0, "", ErrSecretKey
		}
		return "", 0, input.Password, nil
	}
	if !found {
		return "", 0, "", ErrInvalidInput
	}
	// The stored password is reused only for the same server and login; a changed
	// host must not receive the stored secret.
	if input.Host != existing.Host || input.Port != existing.Port || input.Username != existing.Username {
		return "", 0, "", ErrPasswordRequired
	}
	password, err := s.secrets.Open(existing.keyVersion, existing.ciphertext)
	if err != nil {
		return "", 0, "", ErrUnreadable
	}
	return existing.ciphertext, existing.keyVersion, password, nil
}

func (s *Service) store(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	input SaveInput,
	ciphertext string,
	keyVersion int,
	existed, passwordChanged bool,
	audit identity.AuditContext,
) error {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WorkspaceUpdate); err != nil {
		return identity.ErrForbidden
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO workspace_email_settings (
			workspace_id, sender_address, sender_name, host, port, username,
			encrypted_password, secret_key_version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id) DO UPDATE SET
			sender_address = excluded.sender_address,
			sender_name = excluded.sender_name,
			host = excluded.host,
			port = excluded.port,
			username = excluded.username,
			encrypted_password = excluded.encrypted_password,
			secret_key_version = excluded.secret_key_version,
			updated_at = excluded.updated_at
	`, workspaceID, input.SenderAddress, input.SenderName, input.Host, input.Port, input.Username,
		ciphertext, keyVersion, stamp(now), stamp(now)); err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "workspace_email.updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: workspaceID, TargetType: "workspace_email", TargetID: workspaceID,
		Action: "workspace_email.update", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata: metadata(map[string]any{
			"host": input.Host, "replaced": existed, "passwordChanged": passwordChanged,
		}),
		CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) load(ctx context.Context, workspaceID string) (stored, bool, error) {
	var row stored
	var updated string
	err := s.queryRow(ctx, s.db, `
		SELECT sender_address, sender_name, host, port, username,
			encrypted_password, secret_key_version, updated_at
		FROM workspace_email_settings WHERE workspace_id = ?
	`, workspaceID).Scan(&row.SenderAddress, &row.SenderName, &row.Host, &row.Port, &row.Username,
		&row.ciphertext, &row.keyVersion, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return stored{}, false, nil
	}
	if err != nil {
		return stored{}, false, err
	}
	row.updatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return row, true, nil
}

func normalize(input SaveInput) (SaveInput, error) {
	input.SenderAddress = strings.TrimSpace(input.SenderAddress)
	input.SenderName = strings.TrimSpace(input.SenderName)
	input.Host = strings.TrimSpace(input.Host)
	input.Username = strings.TrimSpace(input.Username)
	address, err := mail.ParseAddress(input.SenderAddress)
	if err != nil || address.Address != input.SenderAddress {
		return SaveInput{}, ErrInvalidInput
	}
	if len(input.SenderName) > maxSenderNameLength || strings.ContainsAny(input.SenderName, "\r\n") {
		return SaveInput{}, ErrInvalidInput
	}
	if input.Host == "" || len(input.Host) > maxHostLength || strings.ContainsAny(input.Host, " \t\r\n/@:") {
		return SaveInput{}, ErrInvalidInput
	}
	if input.Port < 1 || input.Port > 65535 {
		return SaveInput{}, ErrInvalidInput
	}
	if input.Username == "" || len(input.Username) > maxUsernameLength || strings.ContainsAny(input.Username, "\r\n") {
		return SaveInput{}, ErrInvalidInput
	}
	if len(input.Password) > maxPasswordLength || strings.ContainsAny(input.Password, "\r\n") {
		return SaveInput{}, ErrInvalidInput
	}
	return input, nil
}

func (s *Service) exec(ctx context.Context, executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, query string, args ...any) (sql.Result, error) {
	return executor.ExecContext(ctx, s.bind(query), args...)
}

func (s *Service) queryRow(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) *sql.Row {
	return queryer.QueryRowContext(ctx, s.bind(query), args...)
}

func (s *Service) bind(query string) string { return database.Rebind(s.driver, query) }

func stamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func metadata(value map[string]any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
