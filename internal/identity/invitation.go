package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

var errInvitationExpired = errors.New("invitation expired")

func (s *Service) InvitationForSecret(
	ctx context.Context,
	secret string,
) (Invitation, error) {
	if secret == "" {
		return Invitation{}, ErrInvalidInvitation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Invitation{}, err
	}
	defer tx.Rollback()
	invitation, err := s.findInvitation(ctx, tx, secret)
	if err != nil {
		return Invitation{}, ErrInvalidInvitation
	}
	if err := tx.Commit(); err != nil {
		return Invitation{}, err
	}
	return invitation, nil
}

func (s *Service) CreateInvitation(
	ctx context.Context,
	actor Principal,
	email string,
	role Role,
	lifetime time.Duration,
	audit AuditContext,
) (NewInvitation, error) {
	if s.require(ctx, actor, InvitationsManage) != nil || !validInvitationRole(role) {
		return NewInvitation{}, ErrForbidden
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return NewInvitation{}, err
	}
	if lifetime == 0 {
		lifetime = defaultInvitationLifetime
	}
	if lifetime < time.Minute || lifetime > 30*24*time.Hour {
		return NewInvitation{}, errors.New("invitation lifetime must be 1 minute to 30 days")
	}
	secret, err := newSecret()
	if err != nil {
		return NewInvitation{}, err
	}
	invitationID, err := id.New()
	if err != nil {
		return NewInvitation{}, err
	}
	now := s.now().UTC()
	invitation := NewInvitation{
		Invitation: Invitation{
			ID: invitationID, WorkspaceID: actor.WorkspaceID,
			Email: email, Role: role, CreatedAt: now, ExpiresAt: now.Add(lifetime),
		},
		Secret: secret,
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return NewInvitation{}, err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, InvitationsManage) != nil {
		return NewInvitation{}, ErrForbidden
	}
	if err := s.expireInvitations(ctx, tx, actor.WorkspaceID, email, now); err != nil {
		return NewInvitation{}, err
	}
	var existing int
	err = s.queryRow(ctx, tx, `
		SELECT 1 FROM invitations
		WHERE workspace_id = ? AND active_email = ?
	`, actor.WorkspaceID, email).Scan(&existing)
	if err == nil {
		return NewInvitation{}, ErrActiveInvitation
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return NewInvitation{}, err
	}
	err = s.queryRow(ctx, tx, `
		SELECT 1 FROM workspace_members wm
		JOIN users u ON u.id = wm.user_id
		WHERE wm.workspace_id = ? AND u.email = ?
	`, actor.WorkspaceID, email).Scan(&existing)
	if err == nil {
		return NewInvitation{}, ErrActiveInvitation
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return NewInvitation{}, err
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO invitations (
			id, workspace_id, invited_by_user_id, email, active_email,
			role, secret_hash, created_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, invitation.ID, invitation.WorkspaceID, actor.UserID, email, email,
		role, hashSecret(secret), timestamp(now), timestamp(invitation.ExpiresAt)); err != nil {
		return NewInvitation{}, fmt.Errorf("create invitation: %w", err)
	}
	if err := s.auditInvitation(
		ctx, tx, actor, invitation.ID, "created", role, audit, now,
	); err != nil {
		return NewInvitation{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewInvitation{}, err
	}
	return invitation, nil
}

func (s *Service) ResendInvitation(
	ctx context.Context,
	actor Principal,
	invitationID string,
	lifetime time.Duration,
	audit AuditContext,
) (string, error) {
	if s.require(ctx, actor, InvitationsManage) != nil {
		return "", ErrForbidden
	}
	if lifetime == 0 {
		lifetime = defaultInvitationLifetime
	}
	if lifetime < time.Minute || lifetime > 30*24*time.Hour {
		return "", ErrInvalidInvitation
	}
	secret, err := newSecret()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, InvitationsManage) != nil {
		return "", ErrForbidden
	}
	var email string
	var role Role
	var expiresAt string
	var acceptedAt, expiredAt, revokedAt sql.NullString
	if err := s.queryRow(ctx, tx, `
		SELECT email, role, expires_at, accepted_at, expired_at, revoked_at
		FROM invitations WHERE id = ? AND workspace_id = ?
	`, invitationID, actor.WorkspaceID).Scan(
		&email, &role, &expiresAt, &acceptedAt, &expiredAt, &revokedAt,
	); err != nil || acceptedAt.Valid || revokedAt.Valid {
		return "", ErrInvalidInvitation
	}
	expires, err := parseTimestamp(expiresAt)
	if err != nil {
		return "", ErrInvalidInvitation
	}
	if !expiredAt.Valid && !now.Before(expires) {
		marked, err := s.markInvitationExpired(
			ctx, tx, invitationID, actor.WorkspaceID, now,
		)
		if err != nil {
			return "", err
		}
		if !marked {
			return "", ErrInvalidInvitation
		}
	}
	result, err := s.exec(ctx, tx, `
		UPDATE invitations
		SET secret_hash = ?, active_email = ?, expires_at = ?, expired_at = NULL
		WHERE id = ? AND accepted_at IS NULL AND revoked_at IS NULL
	`, hashSecret(secret), email, timestamp(now.Add(lifetime)), invitationID)
	if err != nil {
		return "", ErrInvalidInvitation
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return "", ErrInvalidInvitation
	}
	if err := s.auditInvitation(
		ctx, tx, actor, invitationID, "resent", role, audit, now,
	); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Service) RevokeInvitation(
	ctx context.Context,
	actor Principal,
	invitationID string,
	audit AuditContext,
) error {
	if s.require(ctx, actor, InvitationsManage) != nil {
		return ErrForbidden
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, InvitationsManage) != nil {
		return ErrForbidden
	}
	var role Role
	var expiresAt string
	var expiredAt sql.NullString
	if err := s.queryRow(ctx, tx, `
		SELECT role, expires_at, expired_at FROM invitations
		WHERE id = ? AND workspace_id = ?
		AND accepted_at IS NULL AND revoked_at IS NULL
	`, invitationID, actor.WorkspaceID).Scan(&role, &expiresAt, &expiredAt); err != nil {
		return ErrInvalidInvitation
	}
	expires, err := parseTimestamp(expiresAt)
	if err != nil {
		return ErrInvalidInvitation
	}
	if expiredAt.Valid || !now.Before(expires) {
		if !expiredAt.Valid {
			marked, err := s.markInvitationExpired(
				ctx, tx, invitationID, actor.WorkspaceID, now,
			)
			if err != nil {
				return err
			}
			if !marked {
				return ErrInvalidInvitation
			}
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		return ErrInvalidInvitation
	}
	result, err := s.exec(ctx, tx, `
		UPDATE invitations SET revoked_at = ?, active_email = NULL
		WHERE id = ? AND accepted_at IS NULL
		AND expired_at IS NULL AND revoked_at IS NULL
	`, timestamp(now), invitationID)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return ErrInvalidInvitation
	}
	if err := s.auditInvitation(
		ctx, tx, actor, invitationID, "revoked", role, audit, now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ListInvitations(
	ctx context.Context,
	actor Principal,
) ([]Invitation, error) {
	if s.require(ctx, actor, InvitationsManage) != nil {
		return nil, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, InvitationsManage) != nil {
		return nil, ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT id, workspace_id, email, role, created_at, expires_at,
			accepted_at, expired_at, revoked_at
		FROM invitations WHERE workspace_id = ?
		ORDER BY created_at DESC, id DESC
	`), actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invitations []Invitation
	var newlyExpired []string
	now := s.now().UTC()
	for rows.Next() {
		var invitation Invitation
		var createdAt, expiresAt string
		var acceptedAt, expiredAt, revokedAt sql.NullString
		if err := rows.Scan(
			&invitation.ID, &invitation.WorkspaceID, &invitation.Email,
			&invitation.Role, &createdAt, &expiresAt,
			&acceptedAt, &expiredAt, &revokedAt,
		); err != nil {
			return nil, err
		}
		invitation.CreatedAt, err = parseTimestamp(createdAt)
		if err != nil {
			return nil, err
		}
		invitation.ExpiresAt, err = parseTimestamp(expiresAt)
		if err != nil {
			return nil, err
		}
		if invitation.AcceptedAt, err = optionalTimestamp(acceptedAt); err != nil {
			return nil, err
		}
		if invitation.ExpiredAt, err = optionalTimestamp(expiredAt); err != nil {
			return nil, err
		}
		if invitation.RevokedAt, err = optionalTimestamp(revokedAt); err != nil {
			return nil, err
		}
		if invitation.AcceptedAt == nil && invitation.ExpiredAt == nil &&
			invitation.RevokedAt == nil && !now.Before(invitation.ExpiresAt) {
			invitation.ExpiredAt = &now
			newlyExpired = append(newlyExpired, invitation.ID)
		}
		invitations = append(invitations, invitation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, invitationID := range newlyExpired {
		marked, err := s.markInvitationExpired(
			ctx, tx, invitationID, actor.WorkspaceID, now,
		)
		if err != nil {
			return nil, err
		}
		if !marked {
			return nil, ErrInvalidInvitation
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return invitations, nil
}

func (s *Service) AcceptInvitation(
	ctx context.Context,
	secret string,
	userID string,
	audit AuditContext,
) (string, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var userEmail string
	if err := s.queryRow(ctx, tx,
		"SELECT email FROM users WHERE id = ?", userID,
	).Scan(&userEmail); err != nil {
		return "", ErrInvalidInvitation
	}
	invitation, err := s.acceptInvitation(ctx, tx, secret, userID, userEmail, audit)
	if err != nil {
		if errors.Is(err, errInvitationExpired) {
			if commitErr := tx.Commit(); commitErr != nil {
				return "", commitErr
			}
			return "", ErrInvalidInvitation
		}
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return invitation.WorkspaceID, nil
}

func (s *Service) CreateInvitedLocalUser(
	ctx context.Context,
	secret string,
	password string,
	audit AuditContext,
) (string, string, error) {
	if len(password) < 12 || len(password) > 1024 {
		return "", "", ErrInvalidInvitation
	}
	passwordHash, err := hashPassword(password, s.passwordParams)
	if err != nil {
		return "", "", ErrInvalidInvitation
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	invitation, err := s.findInvitation(ctx, tx, secret)
	if err != nil {
		if errors.Is(err, errInvitationExpired) {
			if commitErr := tx.Commit(); commitErr != nil {
				return "", "", commitErr
			}
			return "", "", ErrInvalidInvitation
		}
		return "", "", err
	}
	var existing int
	if err := s.queryRow(ctx, tx,
		"SELECT 1 FROM users WHERE email = ?", invitation.Email,
	).Scan(&existing); err == nil {
		return "", "", ErrInvalidInvitation
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	userID, err := id.New()
	if err != nil {
		return "", "", err
	}
	now := s.now().UTC()
	if _, err := s.exec(ctx, tx, `
		INSERT INTO users (id, email, password_hash, email_verified_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, userID, invitation.Email, passwordHash, timestamp(now), timestamp(now)); err != nil {
		return "", "", ErrInvalidInvitation
	}
	invitation, err = s.acceptLoadedInvitation(
		ctx, tx, invitation, userID, audit, now,
	)
	if err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return userID, invitation.WorkspaceID, nil
}

func (s *Service) acceptInvitation(
	ctx context.Context,
	tx *sql.Tx,
	secret string,
	userID string,
	userEmail string,
	audit AuditContext,
) (Invitation, error) {
	invitation, err := s.findInvitation(ctx, tx, secret)
	if err != nil || invitation.Email != userEmail {
		if errors.Is(err, errInvitationExpired) {
			return Invitation{}, errInvitationExpired
		}
		return Invitation{}, ErrInvalidInvitation
	}
	return s.acceptLoadedInvitation(
		ctx, tx, invitation, userID, audit, s.now().UTC(),
	)
}

func (s *Service) findInvitation(
	ctx context.Context,
	tx *sql.Tx,
	secret string,
) (Invitation, error) {
	var invitation Invitation
	var createdAt, expiresAt string
	var acceptedAt, expiredAt, revokedAt sql.NullString
	err := s.queryRow(ctx, tx, `
		SELECT i.id, i.workspace_id, w.locale, i.email, i.role, i.created_at, i.expires_at,
			accepted_at, expired_at, revoked_at
		FROM invitations i JOIN workspaces w ON w.id = i.workspace_id
		WHERE i.secret_hash = ?
	`, hashSecret(secret)).Scan(
		&invitation.ID, &invitation.WorkspaceID, &invitation.WorkspaceLocale, &invitation.Email,
		&invitation.Role, &createdAt, &expiresAt,
		&acceptedAt, &expiredAt, &revokedAt,
	)
	if err != nil || acceptedAt.Valid || expiredAt.Valid || revokedAt.Valid {
		return Invitation{}, ErrInvalidInvitation
	}
	invitation.CreatedAt, err = parseTimestamp(createdAt)
	if err != nil {
		return Invitation{}, ErrInvalidInvitation
	}
	invitation.ExpiresAt, err = parseTimestamp(expiresAt)
	if err != nil {
		return Invitation{}, ErrInvalidInvitation
	}
	if !s.now().UTC().Before(invitation.ExpiresAt) {
		now := s.now().UTC()
		marked, err := s.markInvitationExpired(
			ctx, tx, invitation.ID, invitation.WorkspaceID, now,
		)
		if err != nil {
			return Invitation{}, err
		}
		if !marked {
			return Invitation{}, ErrInvalidInvitation
		}
		return Invitation{}, errInvitationExpired
	}
	return invitation, nil
}

func (s *Service) acceptLoadedInvitation(
	ctx context.Context,
	tx *sql.Tx,
	invitation Invitation,
	userID string,
	audit AuditContext,
	now time.Time,
) (Invitation, error) {
	if _, err := s.exec(ctx, tx, `
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)
	`, invitation.WorkspaceID, userID, invitation.Role, timestamp(now)); err != nil {
		return Invitation{}, ErrInvalidInvitation
	}
	if err := s.ensureSCIMMembership(ctx, tx, invitation.WorkspaceID, userID, invitation.Role, true, timestamp(now)); err != nil {
		return Invitation{}, ErrInvalidInvitation
	}
	result, err := s.exec(ctx, tx, `
		UPDATE invitations SET accepted_at = ?, active_email = NULL
		WHERE id = ? AND accepted_at IS NULL
		AND expired_at IS NULL AND revoked_at IS NULL AND expires_at > ?
	`, timestamp(now), invitation.ID, timestamp(now))
	if err != nil {
		return Invitation{}, ErrInvalidInvitation
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return Invitation{}, ErrInvalidInvitation
	}
	actor := Principal{
		UserID: userID, WorkspaceID: invitation.WorkspaceID,
		AuthMethod: "invitation",
	}
	if err := s.auditInvitation(
		ctx, tx, actor, invitation.ID, "accepted", invitation.Role, audit, now,
	); err != nil {
		return Invitation{}, err
	}
	invitation.AcceptedAt = &now
	return invitation, nil
}

func (s *Service) expireInvitations(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID string,
	email string,
	now time.Time,
) error {
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT id FROM invitations
		WHERE workspace_id = ? AND active_email = ?
		AND expired_at IS NULL AND expires_at <= ?
	`), workspaceID, email, timestamp(now))
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var invitationID string
		if err := rows.Scan(&invitationID); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, invitationID)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, invitationID := range ids {
		marked, err := s.markInvitationExpired(
			ctx, tx, invitationID, workspaceID, now,
		)
		if err != nil {
			return err
		}
		if !marked {
			return ErrInvalidInvitation
		}
	}
	return nil
}

func (s *Service) markInvitationExpired(
	ctx context.Context,
	tx *sql.Tx,
	invitationID string,
	workspaceID string,
	now time.Time,
) (bool, error) {
	result, err := s.exec(ctx, tx, `
		UPDATE invitations SET active_email = NULL, expired_at = ?
		WHERE id = ? AND workspace_id = ? AND accepted_at IS NULL
		AND expired_at IS NULL AND revoked_at IS NULL AND expires_at <= ?
	`, timestamp(now), invitationID, workspaceID, timestamp(now))
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return false, err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "invitation.expired", AuthMethod: "system",
		WorkspaceID: workspaceID, TargetType: "invitation",
		TargetID: invitationID, Action: "invitation.expire",
		Outcome: "success", Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) auditInvitation(
	ctx context.Context,
	tx *sql.Tx,
	actor Principal,
	invitationID string,
	action string,
	role Role,
	audit AuditContext,
	now time.Time,
) error {
	return s.audit(ctx, tx, AuditEvent{
		EventType: "invitation." + action, ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID,
		TargetType: "invitation", TargetID: invitationID,
		Action: "invitation." + action, Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: metadata(map[string]any{"role": role}), CreatedAt: now,
	})
}

func optionalTimestamp(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseTimestamp(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
