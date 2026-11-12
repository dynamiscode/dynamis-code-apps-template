// Package workspaceemail stores each workspace's invitation sender (SMTP or
// Resend) and routes invitation mail through it. Account email never uses these settings.
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
	ProviderSMTP     = "smtp"
	ProviderResend   = "resend"
	ProviderPostmark = "postmark"

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

// LoginError reports a failed provider check (SMTP login test or Resend key check).
// Reason is fixed text from the provider client and never contains the secret.
type LoginError struct{ Reason string }

func (e *LoginError) Error() string { return "email provider check failed: " + e.Reason }

// Settings is the masked view shown to administrators.
type Settings struct {
	Configured    bool   `json:"configured"`
	Provider      string `json:"provider"`
	SenderAddress string `json:"senderAddress"`
	SenderName    string `json:"senderName"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	Username      string `json:"username"`
	PasswordSet   bool   `json:"passwordSet"`
	// AccountTokenSet reports whether a Postmark account token is stored. The token is never returned.
	AccountTokenSet bool `json:"accountTokenSet"`
	// DomainStatus is the sender domain's status as of the last save (Resend or Postmark).
	DomainStatus string    `json:"senderDomainStatus"`
	UpdatedAt    time.Time `json:"updatedAt,omitzero"`
}

// UsesSMTP reports whether the SMTP fields apply. An unconfigured form defaults to SMTP.
func (s Settings) UsesSMTP() bool {
	return s.Provider != ProviderResend && s.Provider != ProviderPostmark
}

// SaveInput is the form data. For SMTP, Password is the SMTP password; for Resend
// it is the API key; for Postmark it is the Server Token and AccountToken is the
// Account Token. An empty secret keeps the stored one for the same provider.
type SaveInput struct {
	Provider      string
	SenderAddress string
	SenderName    string
	Host          string
	Port          int
	Username      string
	Password      string
	AccountToken  string
}

type stored struct {
	SaveInput
	ciphertext        string
	keyVersion        int
	accountCiphertext string
	accountKeyVersion int
	domainStatus      string
	updatedAt         time.Time
}

// sealedSecrets are the encrypted values written to a row. An empty account
// ciphertext clears any stored Postmark account token.
type sealedSecrets struct {
	ciphertext        string
	keyVersion        int
	accountCiphertext string
	accountKeyVersion int
}

type Service struct {
	db       *sql.DB
	driver   config.DatabaseDriver
	auth     *identity.Service
	secrets  secretbox.Keyring
	fallback appmail.Sender
	// resendBase and postmarkBase are provider API origins; tests point them at fake servers.
	resendBase   string
	postmarkBase string
	// login runs the SMTP login test; tests replace it to avoid a network server.
	login func(context.Context, config.Mail, string) error
}

// NewService takes the instance sender (env config) as the fallback and for account email.
func NewService(db *sql.DB, driver config.DatabaseDriver, auth *identity.Service, secrets secretbox.Keyring, fallback appmail.Sender) *Service {
	return &Service{
		db: db, driver: driver, auth: auth, secrets: secrets, fallback: fallback,
		resendBase:   appmail.ResendBaseURL,
		postmarkBase: appmail.PostmarkBaseURL,
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

// SendFor sends invitation mail through the workspace's provider when configured,
// otherwise through the instance sender. The caller must already have authorized
// the invitation.
func (s *Service) SendFor(ctx context.Context, shortID, recipient, subject, body string) error {
	workspaceID, err := identity.ParseWorkspaceID(shortID)
	if err != nil {
		return err
	}
	row, found, err := s.load(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found {
		return s.Send(ctx, recipient, subject, body)
	}
	secret, err := s.secrets.Open(row.keyVersion, row.ciphertext)
	if err != nil {
		return ErrUnreadable
	}
	if row.Provider == ProviderResend {
		sender, err := appmail.NewResend(s.resendBase, secret, row.SenderAddress, row.SenderName)
		if err != nil {
			return ErrInvalidInput
		}
		return sender.Send(ctx, recipient, subject, body)
	}
	if row.Provider == ProviderPostmark {
		sender, err := appmail.NewPostmark(s.postmarkBase, secret, row.SenderAddress, row.SenderName)
		if err != nil {
			return ErrInvalidInput
		}
		return sender.Send(ctx, recipient, subject, body)
	}
	sender, err := appmail.NewWorkspaceSMTP(smtpConfig(row.SaveInput, secret), row.SenderName)
	if err != nil {
		return ErrInvalidInput
	}
	return sender.Send(ctx, recipient, subject, body)
}

// Get returns the masked settings to a principal who can update the workspace.
func (s *Service) Get(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) (Settings, error) {
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
		Configured: true, Provider: row.Provider, SenderAddress: row.SenderAddress, SenderName: row.SenderName,
		Host: row.Host, Port: row.Port, Username: row.Username, PasswordSet: true,
		AccountTokenSet: row.accountCiphertext != "",
		DomainStatus:    row.domainStatus, UpdatedAt: row.updatedAt,
	}, nil
}

// Save runs the provider check with the submitted settings and stores them only if it passes.
// SMTP runs a login test; Resend reads the account's sending domains; Postmark checks the
// server token and reads sending domains with the account token. Nothing is sent.
func (s *Service) Save(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, input SaveInput, audit identity.AuditContext) error {
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
	ciphertext, keyVersion, secret, err := s.resolveSecret(input, existing, found)
	if err != nil {
		return err
	}
	sealed := sealedSecrets{ciphertext: ciphertext, keyVersion: keyVersion}
	domainStatus := ""
	switch input.Provider {
	case ProviderResend:
		domains, err := appmail.ResendDomains(ctx, s.resendBase, secret)
		if err != nil {
			return &LoginError{Reason: err.Error()}
		}
		domainStatus = senderDomainStatus(domains, input.SenderAddress)
	case ProviderPostmark:
		accountCiphertext, accountKeyVersion, accountSecret, err := s.resolveAccountSecret(input, existing, found)
		if err != nil {
			return err
		}
		if err := appmail.PostmarkServerCheck(ctx, s.postmarkBase, secret); err != nil {
			return &LoginError{Reason: err.Error()}
		}
		domains, err := appmail.PostmarkDomains(ctx, s.postmarkBase, accountSecret)
		if err != nil {
			return &LoginError{Reason: err.Error()}
		}
		domainStatus = postmarkDomainStatus(domains, input.SenderAddress)
		sealed.accountCiphertext, sealed.accountKeyVersion = accountCiphertext, accountKeyVersion
	default:
		cfg := smtpConfig(input, secret)
		if _, err := appmail.NewWorkspaceSMTP(cfg, input.SenderName); err != nil {
			return ErrInvalidInput
		}
		if err := s.login(ctx, cfg, input.SenderName); err != nil {
			return &LoginError{Reason: err.Error()}
		}
	}
	if input.Password != "" {
		sealed.ciphertext, sealed.keyVersion, err = s.secrets.Seal(input.Password)
		if err != nil {
			return ErrSecretKey
		}
	}
	if input.AccountToken != "" {
		sealed.accountCiphertext, sealed.accountKeyVersion, err = s.secrets.Seal(input.AccountToken)
		if err != nil {
			return ErrSecretKey
		}
	}
	return s.store(ctx, actor, workspaceID, input, sealed, domainStatus, found, input.Password != "", audit)
}

// resolveAccountSecret returns the Postmark account token to check with. A new token
// is returned unsealed for the caller to seal; a blank one reuses the stored token
// only when the stored provider is Postmark too.
func (s *Service) resolveAccountSecret(input SaveInput, existing stored, found bool) (string, int, string, error) {
	if input.AccountToken != "" {
		if !s.secrets.Configured() {
			return "", 0, "", ErrSecretKey
		}
		return "", 0, input.AccountToken, nil
	}
	if !found || existing.Provider != ProviderPostmark || existing.accountCiphertext == "" {
		return "", 0, "", ErrInvalidInput
	}
	token, err := s.secrets.Open(existing.accountKeyVersion, existing.accountCiphertext)
	if err != nil {
		return "", 0, "", ErrUnreadable
	}
	return existing.accountCiphertext, existing.accountKeyVersion, token, nil
}

// postmarkDomainStatus maps the sender address's domain to verified (DKIM and Return-Path
// both verified), pending (listed but not verified), or not_found.
func postmarkDomainStatus(domains []appmail.PostmarkDomain, address string) string {
	domain := strings.ToLower(address[strings.LastIndex(address, "@")+1:])
	for _, candidate := range domains {
		if strings.ToLower(candidate.Name) == domain {
			if candidate.DKIMVerified && candidate.ReturnPathDomainVerified {
				return "verified"
			}
			return "pending"
		}
	}
	return "not_found"
}

// resolveSecret returns the secret to check with. A blank password reuses the stored
// secret only for the same provider and, for SMTP, the same server and login.
func (s *Service) resolveSecret(input SaveInput, existing stored, found bool) (string, int, string, error) {
	if input.Password != "" {
		if !s.secrets.Configured() {
			return "", 0, "", ErrSecretKey
		}
		return "", 0, input.Password, nil
	}
	if !found {
		return "", 0, "", ErrInvalidInput
	}
	// A changed provider, or a changed SMTP host, must not receive the stored secret.
	if input.Provider != existing.Provider {
		return "", 0, "", ErrPasswordRequired
	}
	if input.Provider == ProviderSMTP && (input.Host != existing.Host || input.Port != existing.Port || input.Username != existing.Username) {
		return "", 0, "", ErrPasswordRequired
	}
	password, err := s.secrets.Open(existing.keyVersion, existing.ciphertext)
	if err != nil {
		return "", 0, "", ErrUnreadable
	}
	return existing.ciphertext, existing.keyVersion, password, nil
}

// senderDomainStatus reports the Resend status of the sender address's domain.
func senderDomainStatus(domains []appmail.ResendDomain, address string) string {
	domain := strings.ToLower(address[strings.LastIndex(address, "@")+1:])
	for _, candidate := range domains {
		if strings.ToLower(candidate.Name) == domain {
			return candidate.Status
		}
	}
	return "not_found"
}

func (s *Service) store(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	input SaveInput,
	sealed sealedSecrets,
	domainStatus string,
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
			workspace_id, provider, sender_address, sender_name, host, port, username,
			encrypted_password, secret_key_version, encrypted_account_token, account_secret_key_version,
			sender_domain_status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (workspace_id) DO UPDATE SET
			provider = excluded.provider,
			sender_address = excluded.sender_address,
			sender_name = excluded.sender_name,
			host = excluded.host,
			port = excluded.port,
			username = excluded.username,
			encrypted_password = excluded.encrypted_password,
			secret_key_version = excluded.secret_key_version,
			encrypted_account_token = excluded.encrypted_account_token,
			account_secret_key_version = excluded.account_secret_key_version,
			sender_domain_status = excluded.sender_domain_status,
			updated_at = excluded.updated_at
	`, workspaceID, input.Provider, input.SenderAddress, input.SenderName, input.Host, input.Port, input.Username,
		sealed.ciphertext, sealed.keyVersion, sealed.accountCiphertext, sealed.accountKeyVersion,
		domainStatus, stamp(now), stamp(now)); err != nil {
		return err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "workspace_email.updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: workspaceID, TargetType: "workspace_email", TargetID: workspaceID.String(),
		Action: "workspace_email.update", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata: metadata(map[string]any{
			"provider": input.Provider, "host": input.Host, "replaced": existed, "passwordChanged": passwordChanged,
		}),
		CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) load(ctx context.Context, workspaceID identity.WorkspaceID) (stored, bool, error) {
	var row stored
	var updated string
	err := s.queryRow(ctx, s.db, `
		SELECT provider, sender_address, sender_name, host, port, username,
			encrypted_password, secret_key_version, encrypted_account_token, account_secret_key_version,
			sender_domain_status, updated_at
		FROM workspace_email_settings WHERE workspace_id = ?
	`, workspaceID).Scan(&row.Provider, &row.SenderAddress, &row.SenderName, &row.Host, &row.Port, &row.Username,
		&row.ciphertext, &row.keyVersion, &row.accountCiphertext, &row.accountKeyVersion, &row.domainStatus, &updated)
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
	input.Provider = strings.TrimSpace(input.Provider)
	if input.Provider == "" {
		input.Provider = ProviderSMTP
	}
	if input.Provider != ProviderSMTP && input.Provider != ProviderResend && input.Provider != ProviderPostmark {
		return SaveInput{}, ErrInvalidInput
	}
	input.SenderAddress = strings.TrimSpace(input.SenderAddress)
	input.SenderName = strings.TrimSpace(input.SenderName)
	address, err := mail.ParseAddress(input.SenderAddress)
	if err != nil || address.Address != input.SenderAddress {
		return SaveInput{}, ErrInvalidInput
	}
	if len(input.SenderName) > maxSenderNameLength || strings.ContainsAny(input.SenderName, "\r\n") {
		return SaveInput{}, ErrInvalidInput
	}
	if input.Provider != ProviderSMTP {
		// Resend and Postmark use only API tokens and sender; SMTP fields stay empty.
		input.Host, input.Port, input.Username = "", 0, ""
	}
	if input.Provider != ProviderPostmark {
		input.AccountToken = ""
	} else if len(input.AccountToken) > maxPasswordLength || strings.ContainsAny(input.AccountToken, "\r\n") {
		return SaveInput{}, ErrInvalidInput
	}
	if input.Provider == ProviderSMTP {
		input.Host = strings.TrimSpace(input.Host)
		input.Username = strings.TrimSpace(input.Username)
		if input.Host == "" || len(input.Host) > maxHostLength || strings.ContainsAny(input.Host, " \t\r\n/@:") {
			return SaveInput{}, ErrInvalidInput
		}
		if input.Port < 1 || input.Port > 65535 {
			return SaveInput{}, ErrInvalidInput
		}
		if input.Username == "" || len(input.Username) > maxUsernameLength || strings.ContainsAny(input.Username, "\r\n") {
			return SaveInput{}, ErrInvalidInput
		}
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

// SetResendBaseURL points Resend calls at another origin. Production keeps the
// default; handler tests use it to reach a fake Resend server.
func (s *Service) SetResendBaseURL(base string) { s.resendBase = base }

// SetPostmarkBaseURL points Postmark calls at another origin, for the same reason.
func (s *Service) SetPostmarkBaseURL(base string) { s.postmarkBase = base }
