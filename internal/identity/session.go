package identity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func (s *Service) CreateSession(
	ctx context.Context,
	userID string,
	authMethod string,
	oidcProviderID string,
	lifetime time.Duration,
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
	if _, err := s.exec(ctx, tx, `
		INSERT INTO sessions (
			id, user_id, secret_hash, csrf_hash, auth_method,
			oidc_provider_id, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, session.ID, userID, hashSecret(secret), hashSecret(csrfSecret),
		authMethod, nullable(oidcProviderID), timestamp(now),
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

func (s *Service) AuthenticateSession(
	ctx context.Context,
	secret string,
) (Session, error) {
	var session Session
	var createdAt, expiresAt string
	var oidcProviderID sql.NullString
	var revokedAt sql.NullString
	err := s.queryRow(ctx, s.db, `
		SELECT id, user_id, auth_method, oidc_provider_id,
			created_at, expires_at, revoked_at
		FROM sessions WHERE secret_hash = ?
	`, hashSecret(secret)).Scan(
		&session.ID, &session.UserID, &session.AuthMethod, &oidcProviderID,
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
		SELECT id, user_id, auth_method, oidc_provider_id,
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
		if err := rows.Scan(
			&session.ID, &session.UserID, &session.AuthMethod, &oidcProviderID,
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
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}
