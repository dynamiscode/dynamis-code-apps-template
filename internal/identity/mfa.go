package identity

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
)

const (
	mfaChallengeLifetime        = 5 * time.Minute
	freshAuthenticationLifetime = 10 * time.Minute
	maxMFAAttempts              = 5
	recoveryCodeCount           = 10
)

type webAuthnUser struct {
	id          string
	email       string
	displayName string
	credentials []webauthnlib.Credential
}

func (u webAuthnUser) WebAuthnID() []byte   { return []byte(u.id) }
func (u webAuthnUser) WebAuthnName() string { return u.email }
func (u webAuthnUser) WebAuthnDisplayName() string {
	if u.displayName != "" {
		return u.displayName
	}
	return u.email
}
func (u webAuthnUser) WebAuthnCredentials() []webauthnlib.Credential { return u.credentials }

func (s *Service) MFARequired(ctx context.Context, userID string) (bool, error) {
	return s.mfaRequiredForRole(ctx, userID, "")
}

func (s *Service) mfaRequiredForRole(ctx context.Context, userID string, targetRole Role) (bool, error) {
	if !s.mfa.Enabled || !s.mfa.RequireForAdmins {
		return false, nil
	}
	status, err := s.MFAStatus(ctx, userID)
	if err != nil {
		return false, err
	}
	if !status.TOTPEnabled && status.PasskeyCount == 0 {
		return false, nil
	}
	if targetRole == Owner || targetRole == Admin {
		return true, nil
	}
	var admins int
	if err := s.queryRow(ctx, s.db, `
		SELECT COUNT(*) FROM workspace_members
		WHERE user_id = ? AND role IN ('owner', 'admin')
	`, userID).Scan(&admins); err != nil {
		return false, err
	}
	return admins > 0, nil
}

func (s *Service) MFAStatus(ctx context.Context, userID string) (MFAStatus, error) {
	if !s.mfa.Enabled {
		return MFAStatus{}, nil
	}
	var totp int
	if err := s.queryRow(ctx, s.db, "SELECT COUNT(*) FROM mfa_totp WHERE user_id = ?", userID).Scan(&totp); err != nil {
		return MFAStatus{}, err
	}
	var passkeys, recovery int
	if err := s.queryRow(ctx, s.db, "SELECT COUNT(*) FROM mfa_passkeys WHERE user_id = ? AND revoked_at IS NULL", userID).Scan(&passkeys); err != nil {
		return MFAStatus{}, err
	}
	if err := s.queryRow(ctx, s.db, "SELECT COUNT(*) FROM mfa_recovery_codes WHERE user_id = ? AND used_at IS NULL", userID).Scan(&recovery); err != nil {
		return MFAStatus{}, err
	}
	return MFAStatus{Enabled: true, TOTPEnabled: totp == 1, PasskeyCount: passkeys, RecoveryRemain: recovery}, nil
}

func (s *Service) BeginMFALogin(ctx context.Context, userID string, audit AuditContext) (MFALoginChallenge, error) {
	return s.BeginMFALoginWithMethod(ctx, userID, "local", "", audit)
}

func (s *Service) BeginMFALoginWithMethod(ctx context.Context, userID, authMethod, oidcProviderID string, audit AuditContext) (MFALoginChallenge, error) {
	if !s.mfa.Enabled {
		return MFALoginChallenge{}, ErrMFAUnavailable
	}
	if authMethod != "local" && authMethod != "oidc" {
		return MFALoginChallenge{}, ErrMFAUnavailable
	}
	status, err := s.MFAStatus(ctx, userID)
	if err != nil {
		return MFALoginChallenge{}, err
	}
	if !status.TOTPEnabled && status.PasskeyCount == 0 && status.RecoveryRemain == 0 {
		return MFALoginChallenge{}, ErrMFAUnavailable
	}
	token, err := newSecret()
	if err != nil {
		return MFALoginChallenge{}, err
	}
	challenge := MFALoginChallenge{Token: token, UserID: userID, AuthMethod: authMethod, OIDCProviderID: oidcProviderID, ExpiresAt: s.now().UTC().Add(mfaChallengeLifetime)}
	user, err := s.loadWebAuthnUser(ctx, s.db, userID)
	if err != nil {
		return MFALoginChallenge{}, err
	}
	if len(user.credentials) > 0 {
		options, session, err := s.webauthn.BeginLogin(user, webauthnlib.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return MFALoginChallenge{}, ErrMFAUnavailable
		}
		challenge.Methods = append(challenge.Methods, "passkey")
		encodedOptions, marshalErr := json.Marshal(options)
		if marshalErr != nil {
			return MFALoginChallenge{}, marshalErr
		}
		challenge.PasskeyJSON = json.RawMessage(encodedOptions)
		encoded, err := json.Marshal(session)
		if err != nil {
			return MFALoginChallenge{}, err
		}
		if err := s.insertMFAChallenge(ctx, challenge, "login", "", string(encoded), "", audit); err != nil {
			return MFALoginChallenge{}, err
		}
	} else if err := s.insertMFAChallenge(ctx, challenge, "login", "", "", "", audit); err != nil {
		return MFALoginChallenge{}, err
	}
	if status.TOTPEnabled {
		challenge.Methods = append(challenge.Methods, "totp")
	}
	if status.RecoveryRemain > 0 {
		challenge.Methods = append(challenge.Methods, "recovery")
	}
	required, err := s.MFARequired(ctx, userID)
	if err != nil {
		return MFALoginChallenge{}, err
	}
	if required {
		if err := s.audit(ctx, s.db, AuditEvent{EventType: "mfa.policy.enforced", ActorUserID: userID, AuthMethod: "policy", TargetType: "user", TargetID: userID, Action: "mfa.policy.enforce", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: s.now().UTC()}); err != nil {
			return MFALoginChallenge{}, err
		}
	}
	return challenge, nil
}

func (s *Service) MFALoginOptions(ctx context.Context, token string) (MFALoginChallenge, error) {
	challenge, _, err := s.loadChallenge(ctx, token, "login")
	if err != nil {
		return MFALoginChallenge{}, err
	}
	status, err := s.MFAStatus(ctx, challenge.UserID)
	if err != nil {
		return MFALoginChallenge{}, err
	}
	methods := make([]string, 0, 3)
	if challenge.webauthnJSON != "" {
		methods = append(methods, "passkey")
	}
	if status.TOTPEnabled {
		methods = append(methods, "totp")
	}
	if status.RecoveryRemain > 0 {
		methods = append(methods, "recovery")
	}
	return MFALoginChallenge{Token: token, UserID: challenge.UserID, Methods: methods, PasskeyJSON: json.RawMessage(challenge.webauthnJSON), ExpiresAt: challenge.expiresAt}, nil
}

func (s *Service) BeginTOTPEnrollment(ctx context.Context, userID, sessionID, password string, audit AuditContext) (TOTPEnrollment, error) {
	if !s.mfa.Enabled {
		return TOTPEnrollment{}, ErrMFAUnavailable
	}
	if err := s.VerifyFreshAuthentication(ctx, userID, sessionID, password); err != nil {
		return TOTPEnrollment{}, err
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return TOTPEnrollment{}, err
	}
	secret := strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
	token, err := newSecret()
	if err != nil {
		return TOTPEnrollment{}, err
	}
	encrypted, err := s.encrypt([]byte(secret))
	if err != nil {
		return TOTPEnrollment{}, err
	}
	expires := s.now().UTC().Add(mfaChallengeLifetime)
	if err := s.insertMFAChallenge(ctx, MFALoginChallenge{Token: token, UserID: userID, ExpiresAt: expires}, "totp_enrollment", sessionID, "", encrypted, audit); err != nil {
		return TOTPEnrollment{}, err
	}
	profile, err := s.GetUserProfile(ctx, userID)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	issuer := url.QueryEscape(s.mfa.DisplayName)
	label := url.QueryEscape(s.mfa.DisplayName + ":" + profile.Email)
	return TOTPEnrollment{Challenge: token, Secret: secret, OTPAuthURL: "otpauth://totp/" + label + "?secret=" + secret + "&issuer=" + issuer + "&algorithm=SHA1&digits=6&period=30", ExpiresAt: expires}, nil
}

func (s *Service) CompleteTOTPEnrollment(ctx context.Context, sessionID, token, code string, audit AuditContext) ([]string, error) {
	challenge, secret, err := s.loadChallenge(ctx, token, "totp_enrollment")
	if err != nil || challenge.sessionID != sessionID {
		return nil, ErrInvalidMFAChallenge
	}
	if !s.reserveMFAAttempt(ctx, challenge.ID) {
		return nil, ErrInvalidMFAChallenge
	}
	if !validTOTP(secret, code, s.now()) {
		s.auditMFAFailure(ctx, challenge, "totp", audit)
		return nil, ErrInvalidMFACode
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := s.lockMFAUser(ctx, tx, challenge.UserID); err != nil {
		return nil, ErrInvalidMFAChallenge
	}
	if err := s.verifyMFAEnrollmentSession(ctx, tx, challenge.UserID, sessionID); err != nil {
		return nil, ErrInvalidMFAChallenge
	}
	if !s.consumeChallenge(ctx, tx, challenge.ID) {
		return nil, ErrInvalidMFAChallenge
	}
	now := s.now().UTC()
	if _, err := s.exec(ctx, tx, "UPDATE mfa_totp SET encrypted_secret = ?, updated_at = ? WHERE user_id = ?", challenge.encryptedSecret, timestamp(now), challenge.UserID); err != nil {
		return nil, err
	}
	if _, err := s.exec(ctx, tx, "INSERT INTO mfa_totp (user_id, encrypted_secret, created_at, updated_at) SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM mfa_totp WHERE user_id = ?)", challenge.UserID, challenge.encryptedSecret, timestamp(now), timestamp(now), challenge.UserID); err != nil {
		return nil, err
	}
	codes, err := s.ensureRecoveryCodes(ctx, tx, challenge.UserID, now)
	if err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.totp.enrolled", ActorUserID: challenge.UserID, AuthMethod: "session", TargetType: "user", TargetID: challenge.UserID, Action: "mfa.totp.enroll", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now}); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.challenge.completed", ActorUserID: challenge.UserID, AuthMethod: "totp", TargetType: "mfa_challenge", TargetID: challenge.ID, Action: "mfa.challenge.complete", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"method": "totp_enrollment"}), CreatedAt: now}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) BeginPasskeyEnrollment(ctx context.Context, userID, sessionID, password string, audit AuditContext) (PasskeyEnrollment, error) {
	if !s.mfa.Enabled {
		return PasskeyEnrollment{}, ErrMFAUnavailable
	}
	if err := s.VerifyFreshAuthentication(ctx, userID, sessionID, password); err != nil {
		return PasskeyEnrollment{}, err
	}
	user, err := s.loadWebAuthnUser(ctx, s.db, userID)
	if err != nil {
		return PasskeyEnrollment{}, err
	}
	options, session, err := s.webauthn.BeginRegistration(user, webauthnlib.WithExclusions(webauthnlib.Credentials(user.credentials).CredentialDescriptors()), webauthnlib.WithAuthenticatorSelection(protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired}))
	if err != nil {
		return PasskeyEnrollment{}, ErrMFAUnavailable
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return PasskeyEnrollment{}, err
	}
	sessionJSON, err := json.Marshal(session)
	if err != nil {
		return PasskeyEnrollment{}, err
	}
	token, err := newSecret()
	if err != nil {
		return PasskeyEnrollment{}, err
	}
	expires := s.now().UTC().Add(mfaChallengeLifetime)
	if err := s.insertMFAChallenge(ctx, MFALoginChallenge{Token: token, UserID: userID, ExpiresAt: expires}, "passkey_enrollment", sessionID, string(sessionJSON), "", audit); err != nil {
		return PasskeyEnrollment{}, err
	}
	return PasskeyEnrollment{Challenge: token, Options: json.RawMessage(optionsJSON), ExpiresAt: expires}, nil
}

func (s *Service) CompletePasskeyEnrollment(ctx context.Context, userID, sessionID, token, name string, request *http.Request, audit AuditContext) ([]string, error) {
	challenge, _, err := s.loadChallenge(ctx, token, "passkey_enrollment")
	if err != nil || challenge.UserID != userID || challenge.sessionID != sessionID {
		return nil, ErrInvalidMFAChallenge
	}
	var session webauthnlib.SessionData
	if err := json.Unmarshal([]byte(challenge.webauthnJSON), &session); err != nil {
		return nil, ErrInvalidMFAChallenge
	}
	user, err := s.loadWebAuthnUser(ctx, s.db, userID)
	if err != nil {
		return nil, err
	}
	if !s.reserveMFAAttempt(ctx, challenge.ID) {
		return nil, ErrInvalidMFAChallenge
	}
	credential, err := s.webauthn.FinishRegistration(user, session, request)
	if err != nil {
		s.auditMFAFailure(ctx, challenge, "passkey_enrollment", audit)
		return nil, ErrInvalidMFAChallenge
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}
	if len(name) > 120 {
		return nil, ErrInvalidMFAChallenge
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		return nil, err
	}
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := s.lockMFAUser(ctx, tx, userID); err != nil {
		return nil, ErrInvalidMFAChallenge
	}
	if err := s.verifyMFAEnrollmentSession(ctx, tx, userID, sessionID); err != nil {
		return nil, ErrInvalidMFAChallenge
	}
	if !s.consumeChallenge(ctx, tx, challenge.ID) {
		return nil, ErrInvalidMFAChallenge
	}
	passkeyID, err := id.New()
	if err != nil {
		return nil, err
	}
	if _, err := s.exec(ctx, tx, "INSERT INTO mfa_passkeys (id, user_id, credential_id, credential_json, name, created_at) VALUES (?, ?, ?, ?, ?, ?)", passkeyID, userID, credentialID, string(encoded), name, timestamp(now)); err != nil {
		return nil, ErrInvalidMFAChallenge
	}
	codes, err := s.ensureRecoveryCodes(ctx, tx, userID, now)
	if err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.passkey.enrolled", ActorUserID: userID, AuthMethod: "session", TargetType: "passkey", TargetID: passkeyID, Action: "mfa.passkey.enroll", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now}); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.challenge.completed", ActorUserID: userID, AuthMethod: "passkey", TargetType: "mfa_challenge", TargetID: challenge.ID, Action: "mfa.challenge.complete", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"method": "passkey_enrollment"}), CreatedAt: now}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) CompleteTOTPLogin(ctx context.Context, token, code string, audit AuditContext) (NewSession, error) {
	challenge, secret, err := s.loadChallenge(ctx, token, "login")
	if err != nil {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	if !s.reserveMFAAttempt(ctx, challenge.ID) {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	if !validTOTP(secret, code, s.now()) {
		s.auditMFAFailure(ctx, challenge, "totp", audit)
		return NewSession{}, ErrInvalidMFACode
	}
	return s.completeMFAChallenge(ctx, challenge, "totp", audit)
}

func (s *Service) CompleteRecoveryLogin(ctx context.Context, token, code string, audit AuditContext) (NewSession, error) {
	challenge, _, err := s.loadChallenge(ctx, token, "login")
	if err != nil {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	if !s.reserveMFAAttempt(ctx, challenge.ID) {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NewSession{}, err
	}
	defer tx.Rollback()
	if err := s.lockMFAUser(ctx, tx, challenge.UserID); err != nil {
		return NewSession{}, err
	}
	var codeID string
	if err := s.queryRow(ctx, tx, "SELECT id FROM mfa_recovery_codes WHERE user_id = ? AND used_at IS NULL AND code_hash = ?", challenge.UserID, hashSecret(strings.TrimSpace(code))).Scan(&codeID); err != nil {
		_ = tx.Commit()
		s.auditMFAFailure(ctx, challenge, "recovery", audit)
		return NewSession{}, ErrInvalidMFACode
	}
	if !s.consumeChallenge(ctx, tx, challenge.ID) {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	result, err := s.exec(ctx, tx, "UPDATE mfa_recovery_codes SET used_at = ? WHERE id = ? AND used_at IS NULL", timestamp(s.now().UTC()), codeID)
	if err != nil {
		return NewSession{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		_ = tx.Rollback()
		s.auditMFAFailure(ctx, challenge, "recovery", audit)
		return NewSession{}, ErrInvalidMFACode
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.recovery.used", ActorUserID: challenge.UserID, AuthMethod: "recovery", TargetType: "mfa_challenge", TargetID: challenge.ID, Action: "mfa.recovery.use", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: s.now().UTC()}); err != nil {
		return NewSession{}, err
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.challenge.completed", ActorUserID: challenge.UserID, AuthMethod: "recovery", TargetType: "mfa_challenge", TargetID: challenge.ID, Action: "mfa.challenge.complete", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"method": "recovery"}), CreatedAt: s.now().UTC()}); err != nil {
		return NewSession{}, err
	}
	newSession, err := s.createSessionWithLevelTx(ctx, tx, challenge.UserID, challenge.authMethod, challenge.oidcProviderID, 0, AuthLevelMFA, audit)
	if err != nil {
		return NewSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewSession{}, err
	}
	return newSession, nil
}

func (s *Service) CompletePasskeyLogin(ctx context.Context, token string, request *http.Request, audit AuditContext) (NewSession, error) {
	challenge, _, err := s.loadChallenge(ctx, token, "login")
	if err != nil {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	if challenge.webauthnJSON == "" {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	var session webauthnlib.SessionData
	if err := json.Unmarshal([]byte(challenge.webauthnJSON), &session); err != nil {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	user, err := s.loadWebAuthnUser(ctx, s.db, challenge.UserID)
	if err != nil {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	if !s.reserveMFAAttempt(ctx, challenge.ID) {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	credential, err := s.webauthn.FinishLogin(user, session, request)
	if err != nil {
		s.auditMFAFailure(ctx, challenge, "passkey", audit)
		return NewSession{}, ErrInvalidMFAChallenge
	}
	credentialID := base64.RawURLEncoding.EncodeToString(credential.ID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NewSession{}, err
	}
	defer tx.Rollback()
	if err := s.lockMFAUser(ctx, tx, challenge.UserID); err != nil {
		return NewSession{}, err
	}
	stored, err := s.loadCredential(ctx, tx, challenge.UserID, credentialID)
	if err != nil {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	stored.Authenticator.UpdateCounter(credential.Authenticator.SignCount)
	if stored.Authenticator.CloneWarning {
		_ = tx.Rollback()
		s.auditMFAFailure(ctx, challenge, "passkey", audit)
		return NewSession{}, ErrInvalidMFAChallenge
	}
	stored.Flags = stored.Flags.Update(credential.Flags.ProtocolValue())
	encoded, err := json.Marshal(stored)
	if err != nil {
		return NewSession{}, err
	}
	if !s.consumeChallenge(ctx, tx, challenge.ID) {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	result, err := s.exec(ctx, tx, "UPDATE mfa_passkeys SET credential_json = ?, last_used_at = ? WHERE user_id = ? AND credential_id = ? AND revoked_at IS NULL", string(encoded), timestamp(s.now().UTC()), challenge.UserID, credentialID)
	if err != nil {
		return NewSession{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.challenge.completed", ActorUserID: challenge.UserID, AuthMethod: "passkey", TargetType: "mfa_challenge", TargetID: challenge.ID, Action: "mfa.challenge.complete", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"method": "passkey"}), CreatedAt: s.now().UTC()}); err != nil {
		return NewSession{}, err
	}
	newSession, err := s.createSessionWithLevelTx(ctx, tx, challenge.UserID, challenge.authMethod, challenge.oidcProviderID, 0, AuthLevelMFA, audit)
	if err != nil {
		return NewSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewSession{}, err
	}
	return newSession, nil
}

func (s *Service) ListPasskeys(ctx context.Context, userID string) ([]Passkey, error) {
	rows, err := s.db.QueryContext(ctx, s.bind("SELECT id, name, created_at, last_used_at, revoked_at FROM mfa_passkeys WHERE user_id = ? ORDER BY created_at, id"), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Passkey
	for rows.Next() {
		var p Passkey
		var created string
		var last, revoked sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &created, &last, &revoked); err != nil {
			return nil, err
		}
		p.CreatedAt, err = parseTimestamp(created)
		if err != nil {
			return nil, err
		}
		if last.Valid {
			value, err := parseTimestamp(last.String)
			if err != nil {
				return nil, err
			}
			p.LastUsedAt = &value
		}
		if revoked.Valid {
			value, err := parseTimestamp(revoked.String)
			if err != nil {
				return nil, err
			}
			p.RevokedAt = &value
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *Service) RemovePasskey(ctx context.Context, userID, sessionID, passkeyID, password string, audit AuditContext) error {
	if err := s.VerifyFreshAuthentication(ctx, userID, sessionID, password); err != nil {
		return ErrInvalidCredentials
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.lockMFAUser(ctx, tx, userID); err != nil {
		return err
	}
	var passkeyCount, totpCount int
	if err := s.queryRow(ctx, tx, "SELECT COUNT(*) FROM mfa_passkeys WHERE user_id = ? AND revoked_at IS NULL", userID).Scan(&passkeyCount); err != nil {
		return err
	}
	if err := s.queryRow(ctx, tx, "SELECT COUNT(*) FROM mfa_totp WHERE user_id = ?", userID).Scan(&totpCount); err != nil {
		return err
	}
	if passkeyCount+totpCount <= 1 {
		return ErrLastMFAFactor
	}
	result, err := s.exec(ctx, tx, "UPDATE mfa_passkeys SET revoked_at = ? WHERE id = ? AND user_id = ? AND revoked_at IS NULL", timestamp(now), passkeyID, userID)
	if err != nil {
		return err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return ErrInvalidMFAChallenge
	}
	if _, err := s.exec(ctx, tx, "UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", timestamp(now), userID); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.passkey.removed", ActorUserID: userID, AuthMethod: "session", TargetType: "passkey", TargetID: passkeyID, Action: "mfa.passkey.remove", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) RemoveTOTP(ctx context.Context, userID, sessionID, password string, audit AuditContext) error {
	if err := s.VerifyFreshAuthentication(ctx, userID, sessionID, password); err != nil {
		return ErrInvalidCredentials
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.lockMFAUser(ctx, tx, userID); err != nil {
		return err
	}
	var passkeyCount, totpCount int
	if err := s.queryRow(ctx, tx, "SELECT COUNT(*) FROM mfa_passkeys WHERE user_id = ? AND revoked_at IS NULL", userID).Scan(&passkeyCount); err != nil {
		return err
	}
	if err := s.queryRow(ctx, tx, "SELECT COUNT(*) FROM mfa_totp WHERE user_id = ?", userID).Scan(&totpCount); err != nil {
		return err
	}
	if totpCount != 1 || passkeyCount == 0 {
		return ErrLastMFAFactor
	}
	if _, err := s.exec(ctx, tx, "DELETE FROM mfa_totp WHERE user_id = ?", userID); err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, "UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", timestamp(now), userID); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.totp.removed", ActorUserID: userID, AuthMethod: "session", TargetType: "user", TargetID: userID, Action: "mfa.totp.remove", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) lockMFAUser(ctx context.Context, tx *sql.Tx, userID string) error {
	query := "SELECT id FROM users WHERE id = ?"
	if s.driver == config.Postgres {
		query += " FOR UPDATE"
	}
	var lockedUserID string
	return s.queryRow(ctx, tx, query, userID).Scan(&lockedUserID)
}

func (s *Service) verifyMFAEnrollmentSession(ctx context.Context, tx *sql.Tx, userID, sessionID string) error {
	var sessionUserID string
	return s.queryRow(ctx, tx, "SELECT user_id FROM sessions WHERE id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, userID, timestamp(s.now().UTC())).Scan(&sessionUserID)
}

func (s *Service) VerifyFreshAuthentication(ctx context.Context, userID, sessionID, password string) error {
	if sessionID == "" {
		return ErrInvalidSession
	}
	if password != "" {
		var sessionUserID string
		if err := s.queryRow(ctx, s.db, "SELECT user_id FROM sessions WHERE id = ? AND revoked_at IS NULL", sessionID).Scan(&sessionUserID); err != nil || sessionUserID != userID {
			return ErrInvalidSession
		}
		if err := s.ReauthenticateLocal(ctx, userID, password); err != nil {
			return ErrInvalidCredentials
		}
		_, err := s.exec(ctx, s.db, "UPDATE sessions SET fresh_at = ? WHERE id = ? AND revoked_at IS NULL", timestamp(s.now().UTC()), sessionID)
		return err
	}
	var sessionUserID, authMethod string
	var level AuthLevel
	var fresh sql.NullString
	if err := s.queryRow(ctx, s.db, "SELECT user_id, auth_method, auth_level, fresh_at FROM sessions WHERE id = ? AND revoked_at IS NULL", sessionID).Scan(&sessionUserID, &authMethod, &level, &fresh); err != nil || sessionUserID != userID {
		return ErrInvalidSession
	}
	if !fresh.Valid {
		return ErrInvalidCredentials
	}
	when, err := parseTimestamp(fresh.String)
	if err != nil || s.now().UTC().Sub(when) > freshAuthenticationLifetime {
		return ErrInvalidCredentials
	}
	if level >= AuthLevelMFA || (authMethod == "oidc" && level >= AuthLevelPassword) {
		return nil
	}
	return ErrInvalidCredentials
}

func (s *Service) CreateMFASession(ctx context.Context, userID, authMethod, oidcProviderID string, lifetime time.Duration, audit AuditContext) (NewSession, error) {
	return s.createSession(ctx, userID, authMethod, oidcProviderID, lifetime, AuthLevelMFA, audit)
}

func (s *Service) createSession(ctx context.Context, userID, authMethod, oidcProviderID string, lifetime time.Duration, level AuthLevel, audit AuditContext) (NewSession, error) {
	// Kept separate from CreateSession so ordinary password/OIDC callers retain their existing contract.
	return s.createSessionWithLevel(ctx, userID, authMethod, oidcProviderID, lifetime, level, audit)
}

type loadedChallenge struct {
	ID, UserID, authMethod, oidcProviderID, sessionID, webauthnJSON, encryptedSecret string
	attempts                                                                         int
	expiresAt                                                                        time.Time
}

func (s *Service) insertMFAChallenge(ctx context.Context, challenge MFALoginChallenge, purpose, sessionID, webauthnJSON, encryptedSecret string, audit AuditContext) error {
	idValue, err := id.New()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	authMethod := challenge.AuthMethod
	if authMethod == "" {
		authMethod = "local"
	}
	_, err = s.exec(ctx, s.db, "INSERT INTO mfa_challenges (id, token_hash, user_id, purpose, session_id, auth_method, oidc_provider_id, webauthn_session_json, encrypted_secret, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", idValue, hashSecret(challenge.Token), challenge.UserID, purpose, nullable(sessionID), authMethod, nullable(challenge.OIDCProviderID), nullable(webauthnJSON), nullable(encryptedSecret), timestamp(now), timestamp(challenge.ExpiresAt))
	if err != nil {
		return err
	}
	return s.audit(ctx, s.db, AuditEvent{EventType: "mfa.challenge.created", ActorUserID: challenge.UserID, AuthMethod: "session", TargetType: "mfa_challenge", TargetID: idValue, Action: "mfa.challenge.create", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"purpose": purpose}), CreatedAt: now})
}

func (s *Service) loadChallenge(ctx context.Context, token, purpose string) (loadedChallenge, string, error) {
	var c loadedChallenge
	var expires string
	var encrypted sql.NullString
	var webauthn sql.NullString
	var sessionID sql.NullString
	var consumed sql.NullString
	var provider sql.NullString
	err := s.queryRow(ctx, s.db, "SELECT id, user_id, session_id, auth_method, oidc_provider_id, webauthn_session_json, encrypted_secret, attempts, expires_at, consumed_at FROM mfa_challenges WHERE token_hash = ? AND purpose = ?", hashSecret(token), purpose).Scan(&c.ID, &c.UserID, &sessionID, &c.authMethod, &provider, &webauthn, &encrypted, &c.attempts, &expires, &consumed)
	if err != nil {
		return loadedChallenge{}, "", ErrInvalidMFAChallenge
	}
	c.sessionID = sessionID.String
	c.oidcProviderID = provider.String
	c.webauthnJSON = webauthn.String
	c.encryptedSecret = encrypted.String
	c.expiresAt, err = parseTimestamp(expires)
	if err != nil || c.attempts >= maxMFAAttempts || consumed.Valid || !s.now().UTC().Before(c.expiresAt) {
		return loadedChallenge{}, "", ErrInvalidMFAChallenge
	}
	secret := ""
	if c.encryptedSecret != "" {
		bytes, err := s.decrypt(c.encryptedSecret)
		if err != nil {
			return loadedChallenge{}, "", ErrInvalidMFAChallenge
		}
		secret = string(bytes)
	}
	if purpose == "login" && secret == "" {
		var encryptedSecret string
		if err := s.queryRow(ctx, s.db, "SELECT encrypted_secret FROM mfa_totp WHERE user_id = ?", c.UserID).Scan(&encryptedSecret); err == nil {
			bytes, err := s.decrypt(encryptedSecret)
			if err != nil {
				return loadedChallenge{}, "", ErrInvalidMFAChallenge
			}
			secret = string(bytes)
		}
	}
	if c.authMethod == "" {
		c.authMethod = "local"
	}
	return c, secret, nil
}

func (s *Service) reserveMFAAttempt(ctx context.Context, challengeID string) bool {
	result, err := s.exec(ctx, s.db, "UPDATE mfa_challenges SET attempts = attempts + 1 WHERE id = ? AND consumed_at IS NULL AND attempts < ?", challengeID, maxMFAAttempts)
	if err != nil {
		return false
	}
	changed, _ := result.RowsAffected()
	return changed == 1
}

func (s *Service) auditMFAFailure(ctx context.Context, challenge loadedChallenge, method string, audit AuditContext) {
	_ = s.audit(ctx, s.db, AuditEvent{EventType: "mfa.challenge.failed", ActorUserID: challenge.UserID, AuthMethod: method, TargetType: "mfa_challenge", TargetID: challenge.ID, Action: "mfa.challenge.complete", Outcome: "failure", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"method": method}), CreatedAt: s.now().UTC()})
}
func (s *Service) consumeChallenge(ctx context.Context, tx *sql.Tx, challengeID string) bool {
	now := timestamp(s.now().UTC())
	result, err := s.exec(ctx, tx, "UPDATE mfa_challenges SET consumed_at = ? WHERE id = ? AND consumed_at IS NULL AND expires_at > ?", now, challengeID, now)
	if err != nil {
		return false
	}
	changed, _ := result.RowsAffected()
	return changed == 1
}

func (s *Service) completeMFAChallenge(ctx context.Context, challenge loadedChallenge, method string, audit AuditContext) (NewSession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NewSession{}, err
	}
	defer tx.Rollback()
	if err := s.lockMFAUser(ctx, tx, challenge.UserID); err != nil {
		return NewSession{}, err
	}
	if method == "totp" {
		var enabled int
		if err := s.queryRow(ctx, tx, "SELECT COUNT(*) FROM mfa_totp WHERE user_id = ?", challenge.UserID).Scan(&enabled); err != nil || enabled != 1 {
			return NewSession{}, ErrInvalidMFAChallenge
		}
	}
	if !s.consumeChallenge(ctx, tx, challenge.ID) {
		return NewSession{}, ErrInvalidMFAChallenge
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "mfa.challenge.completed", ActorUserID: challenge.UserID, AuthMethod: method, TargetType: "mfa_challenge", TargetID: challenge.ID, Action: "mfa.challenge.complete", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"method": method}), CreatedAt: s.now().UTC()}); err != nil {
		return NewSession{}, err
	}
	newSession, err := s.createSessionWithLevelTx(ctx, tx, challenge.UserID, challenge.authMethod, challenge.oidcProviderID, 0, AuthLevelMFA, audit)
	if err != nil {
		return NewSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewSession{}, err
	}
	return newSession, nil
}

func (s *Service) loadWebAuthnUser(ctx context.Context, queryer rowQueryer, userID string) (webAuthnUser, error) {
	var user webAuthnUser
	if err := s.queryRow(ctx, queryer, "SELECT email, COALESCE(display_name, '') FROM users WHERE id = ?", userID).Scan(&user.email, &user.displayName); err != nil {
		return webAuthnUser{}, ErrForbidden
	}
	user.id = userID
	rows, err := queryer.QueryContext(ctx, s.bind("SELECT credential_json FROM mfa_passkeys WHERE user_id = ? AND revoked_at IS NULL"), userID)
	if err != nil {
		return webAuthnUser{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return webAuthnUser{}, err
		}
		var credential webauthnlib.Credential
		if err := json.Unmarshal([]byte(encoded), &credential); err != nil {
			return webAuthnUser{}, ErrMFAUnavailable
		}
		user.credentials = append(user.credentials, credential)
	}
	return user, rows.Err()
}

func (s *Service) loadCredential(ctx context.Context, queryer rowQueryer, userID, credentialID string) (webauthnlib.Credential, error) {
	var encoded string
	if err := s.queryRow(ctx, queryer, "SELECT credential_json FROM mfa_passkeys WHERE user_id = ? AND credential_id = ? AND revoked_at IS NULL", userID, credentialID).Scan(&encoded); err != nil {
		return webauthnlib.Credential{}, err
	}
	var credential webauthnlib.Credential
	err := json.Unmarshal([]byte(encoded), &credential)
	return credential, err
}

func (s *Service) ensureRecoveryCodes(ctx context.Context, tx *sql.Tx, userID string, now time.Time) ([]string, error) {
	var existing int
	if err := s.queryRow(ctx, tx, "SELECT COUNT(*) FROM mfa_recovery_codes WHERE user_id = ? AND used_at IS NULL", userID).Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, nil
	}
	codes := make([]string, 0, recoveryCodeCount)
	for i := 0; i < recoveryCodeCount; i++ {
		raw, err := newSecret()
		if err != nil {
			return nil, err
		}
		code := strings.ToUpper(raw[:10])
		code = code[:5] + "-" + code[5:]
		codeID, err := id.New()
		if err != nil {
			return nil, err
		}
		if _, err := s.exec(ctx, tx, "INSERT INTO mfa_recovery_codes (id, user_id, code_hash, created_at) VALUES (?, ?, ?, ?)", codeID, userID, hashSecret(code), timestamp(now)); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func (s *Service) encrypt(plain []byte) (string, error) {
	block, err := aes.NewCipher(s.mfa.EncryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(gcm.Seal(nonce, nonce, plain, nil)), nil
}
func (s *Service) decrypt(encoded string) ([]byte, error) {
	block, err := aes.NewCipher(s.mfa.EncryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	encodedBytes, err := hex.DecodeString(encoded)
	if err != nil || len(encodedBytes) < gcm.NonceSize() {
		return nil, fmt.Errorf("invalid encrypted MFA value")
	}
	return gcm.Open(nil, encodedBytes[:gcm.NonceSize()], encodedBytes[gcm.NonceSize():], nil)
}

func validTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	if _, err := strconv.Atoi(code); err != nil {
		return false
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return false
	}
	for offset := int64(-1); offset <= 1; offset++ {
		counter := uint64(now.Unix()/30 + offset)
		var bytes [8]byte
		for i := 7; i >= 0; i-- {
			bytes[i] = byte(counter)
			counter >>= 8
		}
		mac := hmac.New(sha1.New, decoded)
		_, _ = mac.Write(bytes[:])
		sum := mac.Sum(nil)
		index := sum[len(sum)-1] & 15
		value := (uint32(sum[index])&127)<<24 | uint32(sum[index+1])<<16 | uint32(sum[index+2])<<8 | uint32(sum[index+3])
		expected := fmt.Sprintf("%06d", value%1000000)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}
