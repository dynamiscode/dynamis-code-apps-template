package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func (s *Service) CreateAPIToken(
	ctx context.Context,
	actor Principal,
	name string,
	scopes []Permission,
	expiresAt *time.Time,
	audit AuditContext,
) (NewAPIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return NewAPIToken{}, errors.New("token name must be 1 to 100 characters")
	}
	if !validScopes(scopes, actor.Permissions) ||
		!s.scopesAllowed(ctx, actor, scopes) {
		return NewAPIToken{}, ErrForbidden
	}
	now := s.now().UTC()
	if expiresAt != nil {
		value := expiresAt.UTC()
		if !value.After(now) {
			return NewAPIToken{}, errors.New("token expiration must be in the future")
		}
		expiresAt = &value
	}
	secret, err := newSecret()
	if err != nil {
		return NewAPIToken{}, err
	}
	tokenID, err := id.New()
	if err != nil {
		return NewAPIToken{}, err
	}
	scopes = normalizeScopes(scopes)
	token := NewAPIToken{
		APIToken: APIToken{
			ID: tokenID, UserID: actor.UserID, WorkspaceID: actor.WorkspaceID,
			Name: name, Scopes: scopes, CreatedAt: now, ExpiresAt: expiresAt,
		},
		Secret: secret,
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return NewAPIToken{}, err
	}
	defer tx.Rollback()
	if !s.scopesAllowedTx(ctx, tx, actor, scopes) {
		return NewAPIToken{}, ErrForbidden
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO api_tokens (
			id, user_id, workspace_id, name, secret_hash, scopes,
			created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, token.ID, token.UserID, token.WorkspaceID, token.Name,
		hashSecret(secret), encodeScopes(scopes), timestamp(now),
		nullableTime(expiresAt)); err != nil {
		return NewAPIToken{}, fmt.Errorf("create API token: %w", err)
	}
	if err := s.auditToken(ctx, tx, actor, token.ID, "created", scopes, audit, now); err != nil {
		return NewAPIToken{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewAPIToken{}, err
	}
	return token, nil
}

func (s *Service) AuthenticateAPIToken(
	ctx context.Context,
	secret string,
	needed Permission,
	audit AuditContext,
) (Principal, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Principal{}, err
	}
	defer tx.Rollback()
	now := s.now().UTC()
	var tokenID, userID, workspaceID, encodedScopes string
	var expiresAt, revokedAt sql.NullString
	err = s.queryRow(ctx, tx, `
		SELECT id, user_id, workspace_id, scopes, expires_at, revoked_at
		FROM api_tokens WHERE secret_hash = ?
	`, hashSecret(secret)).Scan(
		&tokenID, &userID, &workspaceID, &encodedScopes, &expiresAt, &revokedAt,
	)
	if err != nil || revokedAt.Valid || isExpired(now, expiresAt) {
		return Principal{}, ErrInvalidToken
	}
	var role Role
	if err := s.queryRow(ctx, tx, `
		SELECT role FROM workspace_members
		WHERE workspace_id = ? AND user_id = ?
	`, workspaceID, userID).Scan(&role); err != nil {
		return Principal{}, ErrInvalidToken
	}
	if required, err := s.mfaRequiredForRoleQuery(ctx, tx, userID, role); err != nil {
		return Principal{}, err
	} else if required {
		return Principal{}, ErrMFARequired
	}
	roleAllowed := permissionsForRole(role)
	permissions := make(map[Permission]bool)
	for _, scope := range decodeScopes(encodedScopes) {
		if roleAllowed[scope] {
			permissions[scope] = true
		}
	}
	if needed != "" && !permissions[needed] {
		return Principal{}, ErrForbidden
	}
	result, err := s.exec(ctx, tx, `
		UPDATE api_tokens SET last_used_at = ?
		WHERE id = ? AND revoked_at IS NULL
		AND (expires_at IS NULL OR expires_at > ?)
	`,
		timestamp(now), tokenID, timestamp(now),
	)
	if err != nil {
		return Principal{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Principal{}, ErrInvalidToken
	}
	principal := Principal{
		UserID: userID, WorkspaceID: workspaceID, Role: role,
		Permissions: permissions, AuthMethod: "api_token", TokenID: tokenID,
	}
	usedScopes := []Permission{needed}
	if needed == "" {
		usedScopes = make([]Permission, 0, len(permissions))
		for scope := range permissions {
			usedScopes = append(usedScopes, scope)
		}
		usedScopes = normalizeScopes(usedScopes)
	}
	if err := s.auditToken(
		ctx, tx, principal, tokenID, "used", usedScopes, audit, now,
	); err != nil {
		return Principal{}, err
	}
	if err := tx.Commit(); err != nil {
		return Principal{}, err
	}
	return principal, nil
}

func (s *Service) UpdateAPITokenScopes(
	ctx context.Context,
	actor Principal,
	tokenID string,
	scopes []Permission,
	audit AuditContext,
) error {
	if !validScopes(scopes, actor.Permissions) ||
		!s.scopesAllowed(ctx, actor, scopes) {
		return ErrForbidden
	}
	scopes = normalizeScopes(scopes)
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !s.scopesAllowedTx(ctx, tx, actor, scopes) {
		return ErrForbidden
	}
	result, err := s.exec(ctx, tx, `
		UPDATE api_tokens SET scopes = ?
		WHERE id = ? AND user_id = ? AND workspace_id = ?
		AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)
	`, encodeScopes(scopes), tokenID, actor.UserID, actor.WorkspaceID, timestamp(now))
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrInvalidToken
	}
	if err := s.auditToken(ctx, tx, actor, tokenID, "scopes_changed", scopes, audit, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) RevokeAPIToken(
	ctx context.Context,
	actor Principal,
	tokenID string,
	audit AuditContext,
) error {
	if s.require(ctx, actor, WorkspaceRead) != nil {
		return ErrForbidden
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, WorkspaceRead) != nil {
		return ErrForbidden
	}
	result, err := s.exec(ctx, tx, `
		UPDATE api_tokens SET revoked_at = ?
		WHERE id = ? AND user_id = ? AND workspace_id = ? AND revoked_at IS NULL
	`, timestamp(now), tokenID, actor.UserID, actor.WorkspaceID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrInvalidToken
	}
	if err := s.auditToken(ctx, tx, actor, tokenID, "revoked", nil, audit, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ListAPITokens(
	ctx context.Context,
	actor Principal,
) ([]APIToken, error) {
	if s.require(ctx, actor, WorkspaceRead) != nil {
		return nil, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, WorkspaceRead) != nil {
		return nil, ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT id, user_id, workspace_id, name, scopes, created_at,
			expires_at, last_used_at, revoked_at
		FROM api_tokens WHERE user_id = ? AND workspace_id = ?
		ORDER BY created_at DESC, id DESC
	`), actor.UserID, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []APIToken
	for rows.Next() {
		var token APIToken
		var scopes, createdAt string
		var expiresAt, lastUsedAt, revokedAt sql.NullString
		if err := rows.Scan(
			&token.ID, &token.UserID, &token.WorkspaceID, &token.Name,
			&scopes, &createdAt, &expiresAt, &lastUsedAt, &revokedAt,
		); err != nil {
			return nil, err
		}
		token.Scopes = decodeScopes(scopes)
		token.CreatedAt, err = parseTimestamp(createdAt)
		if err != nil {
			return nil, err
		}
		if token.ExpiresAt, err = optionalTimestamp(expiresAt); err != nil {
			return nil, err
		}
		if token.LastUsedAt, err = optionalTimestamp(lastUsedAt); err != nil {
			return nil, err
		}
		if token.RevokedAt, err = optionalTimestamp(revokedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return tokens, nil
}

func (s *Service) auditToken(
	ctx context.Context,
	tx *sql.Tx,
	actor Principal,
	tokenID string,
	action string,
	scopes []Permission,
	audit AuditContext,
	now time.Time,
) error {
	return s.audit(ctx, tx, AuditEvent{
		EventType: "api_token." + action, ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID,
		TargetType: "api_token", TargetID: tokenID,
		Action: "api_token." + action, Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: metadata(map[string]any{"scopes": scopes}), CreatedAt: now,
	})
}

func encodeScopes(scopes []Permission) string {
	values := make([]string, len(scopes))
	for index, scope := range scopes {
		values[index] = string(scope)
	}
	return strings.Join(values, " ")
}

func decodeScopes(encoded string) []Permission {
	fields := strings.Fields(encoded)
	result := make([]Permission, len(fields))
	for index, value := range fields {
		result[index] = Permission(value)
	}
	return result
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return timestamp(*value)
}

func isExpired(now time.Time, value sql.NullString) bool {
	if !value.Valid {
		return false
	}
	expires, err := parseTimestamp(value.String)
	return err != nil || !now.Before(expires)
}

func (s *Service) scopesAllowed(
	ctx context.Context,
	actor Principal,
	scopes []Permission,
) bool {
	for _, scope := range scopes {
		if s.require(ctx, actor, scope) != nil {
			return false
		}
	}
	return true
}

func (s *Service) scopesAllowedTx(
	ctx context.Context,
	tx *sql.Tx,
	actor Principal,
	scopes []Permission,
) bool {
	for _, scope := range scopes {
		if s.requireTx(ctx, tx, actor, scope) != nil {
			return false
		}
	}
	return true
}
