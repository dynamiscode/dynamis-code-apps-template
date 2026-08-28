package identity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const maxActiveSessionsPerUser = 10

func (s *Service) CreateSession(
	ctx context.Context,
	userID string,
	authMethod string,
	oidcProviderID string,
	lifetime time.Duration,
	audit AuditContext,
) (NewSession, error) {
	return s.createSessionWithLevel(ctx, userID, authMethod, oidcProviderID, lifetime, AuthLevelPassword, audit)
}

func (s *Service) createSessionWithLevel(
	ctx context.Context,
	userID string,
	authMethod string,
	oidcProviderID string,
	lifetime time.Duration,
	authLevel AuthLevel,
	audit AuditContext,
) (NewSession, error) {
	if (authMethod != "local" && authMethod != "oidc") ||
		(authMethod == "local" && oidcProviderID != "") ||
		(authMethod == "oidc" && oidcProviderID == "") {
		return NewSession{}, fmt.Errorf("session authentication method is invalid")
	}
	if lifetime == 0 {
		lifetime = defaultSessionLifetime
	}
	if lifetime < time.Minute || lifetime > 30*24*time.Hour {
		return NewSession{}, fmt.Errorf("session lifetime must be 1 minute to 30 days")
	}
	secret, err := newSecret()
	if err != nil {
		return NewSession{}, err
	}
	csrfSecret, err := newSecret()
	if err != nil {
		return NewSession{}, err
	}
	sessionID, err := id.New()
	if err != nil {
		return NewSession{}, err
	}
	now := s.now().UTC()
	session := NewSession{
		Session: Session{
			ID: sessionID, UserID: userID, AuthMethod: authMethod,
			AuthLevel:      authLevel,
			OIDCProviderID: oidcProviderID,
			CreatedAt:      now, ExpiresAt: now.Add(lifetime),
		},
		Secret: secret, CSRFSecret: csrfSecret,
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return NewSession{}, err
	}
	defer tx.Rollback()
	if err := s.enforceSessionLimit(ctx, tx, userID, now, audit); err != nil {
		return NewSession{}, err
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO sessions (
			id, user_id, secret_hash, csrf_hash, auth_method,
			oidc_provider_id, auth_level, fresh_at, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, userID, hashSecret(secret), hashSecret(csrfSecret),
		authMethod, nullable(oidcProviderID), session.AuthLevel, timestamp(now), timestamp(now),
		timestamp(session.ExpiresAt)); err != nil {
		return NewSession{}, fmt.Errorf("create session: %w", err)
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "session.created", ActorUserID: userID,
		AuthMethod: authMethod, TargetType: "session", TargetID: session.ID,
		Action: "session.create", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return NewSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewSession{}, err
	}
	return session, nil
}

func (s *Service) enforceSessionLimit(
	ctx context.Context,
	tx *sql.Tx,
	userID string,
	now time.Time,
	audit AuditContext,
) error {
	lockQuery := "SELECT id FROM users WHERE id = ?"
	if s.driver == config.Postgres {
		lockQuery += " FOR UPDATE"
	}
	var lockedUserID string
	if err := tx.QueryRowContext(ctx, s.bind(lockQuery), userID).Scan(&lockedUserID); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT id FROM sessions
		WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ?
		ORDER BY created_at DESC, id DESC
	`), userID, timestamp(now))
	if err != nil {
		return err
	}
	defer rows.Close()
	var active []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		active = append(active, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(active) < maxActiveSessionsPerUser {
		return nil
	}
	for _, sessionID := range active[maxActiveSessionsPerUser-1:] {
		if _, err := s.exec(ctx, tx, `UPDATE sessions SET revoked_at = ? WHERE id = ?`,
			timestamp(now), sessionID); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, AuditEvent{
			EventType: "session.limit_revoked", ActorUserID: userID,
			AuthMethod: "system", TargetType: "session", TargetID: sessionID,
			Action: "session.revoke", Outcome: "success", RequestID: audit.RequestID,
			SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AuthenticateSession(
	ctx context.Context,
	secret string,
) (Session, error) {
	var session Session
	var createdAt, expiresAt string
	var authLevel AuthLevel
	var freshAt sql.NullString
	var oidcProviderID sql.NullString
	var revokedAt sql.NullString
	err := s.queryRow(ctx, s.db, `
		SELECT id, user_id, auth_method, oidc_provider_id, auth_level, fresh_at,
			created_at, expires_at, revoked_at
		FROM sessions WHERE secret_hash = ?
	`, hashSecret(secret)).Scan(
		&session.ID, &session.UserID, &session.AuthMethod, &oidcProviderID, &authLevel, &freshAt,
		&createdAt, &expiresAt, &revokedAt,
	)
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	session.CreatedAt, err = parseTimestamp(createdAt)
	if err != nil {
		return Session{}, ErrInvalidSession
	}
	session.ExpiresAt, err = parseTimestamp(expiresAt)
	if err != nil || !s.now().UTC().Before(session.ExpiresAt) || revokedAt.Valid {
		return Session{}, ErrInvalidSession
	}
	if oidcProviderID.Valid {
		session.OIDCProviderID = oidcProviderID.String
	}
	session.AuthLevel = authLevel
	return session, nil
}

func (s *Service) AuthenticateSessionForWorkspace(
	ctx context.Context,
	secret string,
	workspaceID string,
	permission Permission,
) (Principal, error) {
	session, err := s.AuthenticateSession(ctx, secret)
	if err != nil {
		return Principal{}, err
	}
	principal, err := s.Authorize(
		ctx, session.UserID, workspaceID, permission,
	)
	if err != nil {
		return Principal{}, err
	}
	principal.AuthMethod = session.AuthMethod
	principal.AuthLevel = session.AuthLevel
	if s.MFARequired(ctx, session.UserID) && session.AuthLevel < AuthLevelMFA {
		return Principal{}, ErrMFARequired
	}
	return principal, nil
}

func (s *Service) VerifyCSRF(
	ctx context.Context,
	sessionID string,
	csrfSecret string,
) bool {
	var encoded string
	var expiresAt string
	var revokedAt sql.NullString
	err := s.queryRow(ctx, s.db, `
		SELECT csrf_hash, expires_at, revoked_at
		FROM sessions WHERE id = ?
	`, sessionID).Scan(&encoded, &expiresAt, &revokedAt)
	if err != nil || revokedAt.Valid || !equalSecretHash(csrfSecret, encoded) {
		return false
	}
	expires, err := parseTimestamp(expiresAt)
	return err == nil && s.now().UTC().Before(expires)
}

func (s *Service) RevokeSession(
	ctx context.Context,
	userID string,
	sessionID string,
	audit AuditContext,
) (string, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var authMethod string
	var oidcProviderID sql.NullString
	if err := s.queryRow(ctx, tx, `
		SELECT auth_method, oidc_provider_id FROM sessions
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL
	`, sessionID, userID).Scan(&authMethod, &oidcProviderID); err != nil {
		return "", ErrInvalidSession
	}
	result, err := s.exec(ctx, tx, `
		UPDATE sessions SET revoked_at = ?
		WHERE id = ? AND user_id = ? AND revoked_at IS NULL
	`, timestamp(now), sessionID, userID)
	if err != nil {
		return "", err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return "", ErrInvalidSession
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "session.revoked", ActorUserID: userID,
		AuthMethod: authMethod, TargetType: "session", TargetID: sessionID,
		Action: "session.revoke", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return oidcProviderID.String, nil
}

func (s *Service) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT id, user_id, auth_method, oidc_provider_id, auth_level,
			created_at, expires_at, revoked_at
		FROM sessions WHERE user_id = ? ORDER BY created_at DESC, id DESC
	`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []Session
	for rows.Next() {
		var session Session
		var createdAt, expiresAt string
		var oidcProviderID, revokedAt sql.NullString
		var authLevel AuthLevel
		if err := rows.Scan(
			&session.ID, &session.UserID, &session.AuthMethod, &oidcProviderID, &authLevel,
			&createdAt, &expiresAt, &revokedAt,
		); err != nil {
			return nil, err
		}
		session.CreatedAt, err = parseTimestamp(createdAt)
		if err != nil {
			return nil, err
		}
		session.ExpiresAt, err = parseTimestamp(expiresAt)
		if err != nil {
			return nil, err
		}
		if revokedAt.Valid {
			value, err := parseTimestamp(revokedAt.String)
			if err != nil {
				return nil, err
			}
			session.RevokedAt = &value
		}
		if oidcProviderID.Valid {
			session.OIDCProviderID = oidcProviderID.String
		}
		session.AuthLevel = authLevel
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}
