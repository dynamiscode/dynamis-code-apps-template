package identity

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const (
	accountTokenLifetime = 24 * time.Hour
	maxDisplayNameLength = 120
)

func (s *Service) GetUserProfile(ctx context.Context, userID string) (UserProfile, error) {
	var profile UserProfile
	var locale, verifiedAt sql.NullString
	if err := s.queryRow(ctx, s.db, `
		SELECT id, email, display_name, locale, timezone, theme, email_verified_at
		FROM users WHERE id = ?
	`, userID).Scan(
		&profile.ID, &profile.Email, &profile.DisplayName, &locale,
		&profile.Timezone, &profile.Theme, &verifiedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserProfile{}, ErrForbidden
		}
		return UserProfile{}, err
	}
	profile.Locale = locale.String
	if verifiedAt.Valid {
		value, err := parseTimestamp(verifiedAt.String)
		if err != nil {
			return UserProfile{}, err
		}
		profile.EmailVerifiedAt = &value
	}
	return profile, nil
}

func (s *Service) UpdateUserProfile(
	ctx context.Context,
	userID string,
	input ProfileUpdateInput,
	audit AuditContext,
) error {
	displayName := strings.TrimSpace(input.DisplayName)
	if len(displayName) > maxDisplayNameLength {
		return ErrInvalidProfile
	}
	locale, err := normalizeLocale(input.Locale, true)
	if err != nil {
		return err
	}
	timezone, err := normalizeTimezone(input.Timezone)
	if err != nil {
		return ErrInvalidProfile
	}
	theme, err := normalizeTheme(input.Theme)
	if err != nil {
		return ErrInvalidProfile
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := s.queryRow(ctx, tx, "SELECT 1 FROM users WHERE id = ?", userID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrForbidden
		}
		return err
	}
	if _, err := s.exec(ctx, tx, `
		UPDATE users SET display_name = ?, locale = ?, timezone = ?, theme = ?
		WHERE id = ?
	`, displayName, nullable(locale), timezone, theme, userID); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "user.profile.updated", ActorUserID: userID, AuthMethod: "session",
		TargetType: "user", TargetID: userID, Action: "user.profile.update", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata:  metadata(map[string]any{"locale": locale, "timezone": timezone, "theme": theme}),
		CreatedAt: s.now().UTC(),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeTimezone(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 128 || value == "Local" {
		return "", errors.New("timezone is invalid")
	}
	location, err := time.LoadLocation(value)
	if err != nil || location.String() != value {
		return "", errors.New("timezone is invalid")
	}
	return value, nil
}

func normalizeTheme(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "system", "light", "dark":
		return strings.TrimSpace(raw), nil
	default:
		return "", errors.New("theme is invalid")
	}
}

func validateNewPassword(password string) error {
	if len(password) < 12 || len(password) > 1024 {
		return ErrInvalidPassword
	}
	return nil
}

func (s *Service) ChangePassword(
	ctx context.Context,
	userID, currentPassword, newPassword string,
	audit AuditContext,
) error {
	if validateNewPassword(newPassword) != nil {
		return ErrInvalidPassword
	}
	if err := s.ReauthenticateLocal(ctx, userID, currentPassword); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := hashPassword(newPassword, s.passwordParams)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := s.exec(ctx, tx, "UPDATE users SET password_hash = ? WHERE id = ?", hash, userID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrInvalidCredentials
	}
	if _, err := s.exec(ctx, tx, `
		UPDATE sessions SET revoked_at = ?
		WHERE user_id = ? AND revoked_at IS NULL
	`, timestamp(s.now().UTC()), userID); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "user.password.changed", ActorUserID: userID, AuthMethod: "session",
		TargetType: "user", TargetID: userID, Action: "user.password.change", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}",
		CreatedAt: s.now().UTC(),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) BeginEmailVerification(
	ctx context.Context,
	userID string,
	audit AuditContext,
) (NewEmailVerification, error) {
	var email string
	var verifiedAt sql.NullString
	if err := s.queryRow(ctx, s.db,
		"SELECT email, email_verified_at FROM users WHERE id = ?", userID,
	).Scan(&email, &verifiedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NewEmailVerification{}, ErrForbidden
		}
		return NewEmailVerification{}, err
	}
	if verifiedAt.Valid {
		return NewEmailVerification{}, ErrEmailAlreadyVerified
	}
	secret, err := newSecret()
	if err != nil {
		return NewEmailVerification{}, err
	}
	verificationID, err := id.New()
	if err != nil {
		return NewEmailVerification{}, err
	}
	now := s.now().UTC()
	verification := NewEmailVerification{Email: email, Secret: secret, ExpiresAt: now.Add(accountTokenLifetime)}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return NewEmailVerification{}, err
	}
	defer tx.Rollback()
	if _, err := s.exec(ctx, tx, `
		UPDATE email_verifications SET consumed_at = ?
		WHERE user_id = ? AND consumed_at IS NULL
	`, timestamp(now), userID); err != nil {
		return NewEmailVerification{}, err
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO email_verifications (id, user_id, email, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, verificationID, userID, email, hashSecret(secret), timestamp(now), timestamp(verification.ExpiresAt)); err != nil {
		return NewEmailVerification{}, err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "user.email_verification.requested", ActorUserID: userID, AuthMethod: "session",
		TargetType: "user", TargetID: userID, Action: "user.email.verify.request", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return NewEmailVerification{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewEmailVerification{}, err
	}
	return verification, nil
}

func (s *Service) VerifyEmail(ctx context.Context, secret string, audit AuditContext) error {
	if secret == "" {
		return ErrInvalidVerification
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID, email, expiresAt string
	var consumedAt sql.NullString
	if err := s.queryRow(ctx, tx, `
		SELECT user_id, email, expires_at, consumed_at FROM email_verifications
		WHERE token_hash = ?
	`, hashSecret(secret)).Scan(&userID, &email, &expiresAt, &consumedAt); err != nil {
		return ErrInvalidVerification
	}
	expires, err := parseTimestamp(expiresAt)
	if err != nil || consumedAt.Valid || !now.Before(expires) {
		return ErrInvalidVerification
	}
	result, err := s.exec(ctx, tx, `
		UPDATE email_verifications SET consumed_at = ?
		WHERE token_hash = ? AND consumed_at IS NULL
	`, timestamp(now), hashSecret(secret))
	if err != nil {
		return ErrInvalidVerification
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrInvalidVerification
	}
	if _, err := s.exec(ctx, tx, "UPDATE users SET email_verified_at = ? WHERE id = ?", timestamp(now), userID); err != nil {
		return ErrInvalidVerification
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "user.email_verified", ActorUserID: userID, AuthMethod: "email",
		TargetType: "user", TargetID: userID, Action: "user.email.verify", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: metadata(map[string]any{"email": email}), CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) BeginPasswordReset(
	ctx context.Context,
	email string,
	audit AuditContext,
) (NewPasswordReset, error) {
	normalized, err := normalizeEmail(email)
	if err != nil {
		return NewPasswordReset{}, nil
	}
	var userID string
	if err := s.queryRow(ctx, s.db, "SELECT id FROM users WHERE email = ?", normalized).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NewPasswordReset{}, nil
		}
		return NewPasswordReset{}, err
	}
	secret, err := newSecret()
	if err != nil {
		return NewPasswordReset{}, err
	}
	resetID, err := id.New()
	if err != nil {
		return NewPasswordReset{}, err
	}
	now := s.now().UTC()
	reset := NewPasswordReset{Email: normalized, Secret: secret, ExpiresAt: now.Add(accountTokenLifetime)}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return NewPasswordReset{}, err
	}
	defer tx.Rollback()
	if _, err := s.exec(ctx, tx, `
		UPDATE password_resets SET consumed_at = ?
		WHERE user_id = ? AND consumed_at IS NULL
	`, timestamp(now), userID); err != nil {
		return NewPasswordReset{}, err
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO password_resets (id, user_id, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, resetID, userID, hashSecret(secret), timestamp(now), timestamp(reset.ExpiresAt)); err != nil {
		return NewPasswordReset{}, err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "user.password_reset.requested", TargetType: "user", TargetID: userID,
		AuthMethod: "anonymous", Action: "user.password.reset.request", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return NewPasswordReset{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewPasswordReset{}, err
	}
	return reset, nil
}

func (s *Service) CompletePasswordReset(
	ctx context.Context,
	secret, newPassword string,
	audit AuditContext,
) error {
	if secret == "" || validateNewPassword(newPassword) != nil {
		return ErrInvalidReset
	}
	hash, err := hashPassword(newPassword, s.passwordParams)
	if err != nil {
		return ErrInvalidReset
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID, expiresAt string
	var consumedAt sql.NullString
	if err := s.queryRow(ctx, tx, `
		SELECT user_id, expires_at, consumed_at FROM password_resets WHERE token_hash = ?
	`, hashSecret(secret)).Scan(&userID, &expiresAt, &consumedAt); err != nil {
		return ErrInvalidReset
	}
	expires, err := parseTimestamp(expiresAt)
	if err != nil || consumedAt.Valid || !now.Before(expires) {
		return ErrInvalidReset
	}
	result, err := s.exec(ctx, tx, `
		UPDATE password_resets SET consumed_at = ?
		WHERE token_hash = ? AND consumed_at IS NULL
	`, timestamp(now), hashSecret(secret))
	if err != nil {
		return ErrInvalidReset
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrInvalidReset
	}
	if _, err := s.exec(ctx, tx, "UPDATE users SET password_hash = ? WHERE id = ?", hash, userID); err != nil {
		return ErrInvalidReset
	}
	if _, err := s.exec(ctx, tx, "UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", timestamp(now), userID); err != nil {
		return ErrInvalidReset
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "user.password_reset.completed", ActorUserID: userID, AuthMethod: "password_reset",
		TargetType: "user", TargetID: userID, Action: "user.password.reset", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) DeleteAccount(
	ctx context.Context,
	userID, password string,
	audit AuditContext,
) error {
	if err := s.ReauthenticateLocal(ctx, userID, password); err != nil {
		return ErrInvalidCredentials
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owners int
	if err := s.queryRow(ctx, tx,
		"SELECT COUNT(*) FROM workspace_members WHERE user_id = ? AND role = ?", userID, Owner,
	).Scan(&owners); err != nil {
		return err
	}
	if owners != 0 {
		return ErrOwnedWorkspace
	}
	now := s.now().UTC()
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "user.account_deleted", ActorUserID: userID, AuthMethod: "session",
		TargetType: "user", TargetID: userID, Action: "user.delete", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, "DELETE FROM invitations WHERE invited_by_user_id = ?", userID); err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, "DELETE FROM scim_tokens WHERE created_by_user_id = ?", userID); err != nil {
		return err
	}
	result, err := s.exec(ctx, tx, "DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrForbidden
	}
	return tx.Commit()
}
