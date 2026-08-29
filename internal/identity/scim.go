package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const (
	defaultSCIMPageSize = 50
	maxSCIMPageSize     = 100
)

func (s *Service) CreateSCIMToken(ctx context.Context, actor Principal, audit AuditContext) (NewSCIMToken, error) {
	if s.require(ctx, actor, WorkspaceUpdate) != nil {
		return NewSCIMToken{}, ErrForbidden
	}
	secret, err := newSecret()
	if err != nil {
		return NewSCIMToken{}, err
	}
	tokenID, err := id.New()
	if err != nil {
		return NewSCIMToken{}, err
	}
	now := s.now().UTC()
	token := NewSCIMToken{ID: tokenID, WorkspaceID: actor.WorkspaceID, CreatedAt: now, Secret: secret}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return NewSCIMToken{}, err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, WorkspaceUpdate) != nil {
		return NewSCIMToken{}, ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT id FROM scim_tokens
		WHERE workspace_id = ? AND revoked_at IS NULL
	`), actor.WorkspaceID)
	if err != nil {
		return NewSCIMToken{}, err
	}
	var revokedIDs []string
	for rows.Next() {
		var revokedID string
		if err := rows.Scan(&revokedID); err != nil {
			rows.Close()
			return NewSCIMToken{}, err
		}
		revokedIDs = append(revokedIDs, revokedID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return NewSCIMToken{}, err
	}
	rows.Close()
	if _, err := s.exec(ctx, tx, `
		UPDATE scim_tokens SET revoked_at = ?
		WHERE workspace_id = ? AND revoked_at IS NULL
	`, timestamp(now), actor.WorkspaceID); err != nil {
		return NewSCIMToken{}, err
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO scim_tokens (id, workspace_id, created_by_user_id, secret_hash, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, token.ID, token.WorkspaceID, actor.UserID, hashSecret(secret), timestamp(now)); err != nil {
		return NewSCIMToken{}, err
	}
	for _, revokedID := range revokedIDs {
		if err := s.audit(ctx, tx, AuditEvent{
			EventType: "scim.token.revoked", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
			WorkspaceID: actor.WorkspaceID, TargetType: "scim_token", TargetID: revokedID,
			Action: "scim.token.revoke", Outcome: "success", RequestID: audit.RequestID,
			SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
		}); err != nil {
			return NewSCIMToken{}, err
		}
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "scim.token.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: actor.WorkspaceID, TargetType: "scim_token", TargetID: token.ID,
		Action: "scim.token.create", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return NewSCIMToken{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewSCIMToken{}, err
	}
	return token, nil
}

func (s *Service) RevokeSCIMToken(ctx context.Context, actor Principal, audit AuditContext) error {
	if s.require(ctx, actor, WorkspaceUpdate) != nil {
		return ErrForbidden
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, WorkspaceUpdate) != nil {
		return ErrForbidden
	}
	result, err := s.exec(ctx, tx, `
		UPDATE scim_tokens SET revoked_at = ?
		WHERE workspace_id = ? AND revoked_at IS NULL
	`, timestamp(now), actor.WorkspaceID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return tx.Commit()
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "scim.token.revoked", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: actor.WorkspaceID, TargetType: "scim_token", Action: "scim.token.revoke",
		Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) AuthenticateSCIMToken(ctx context.Context, secret, workspaceID string) (Principal, error) {
	if strings.TrimSpace(secret) == "" || workspaceID == "" {
		return Principal{}, ErrInvalidToken
	}
	var userID string
	var role Role
	var tokenID string
	if err := s.queryRow(ctx, s.db, `
		SELECT t.id, t.created_by_user_id, m.role
		FROM scim_tokens t JOIN workspace_members m
		  ON m.workspace_id = t.workspace_id AND m.user_id = t.created_by_user_id
		WHERE t.secret_hash = ? AND t.workspace_id = ? AND t.revoked_at IS NULL
	`, hashSecret(secret), workspaceID).Scan(&tokenID, &userID, &role); err != nil {
		return Principal{}, ErrInvalidToken
	}
	permissions := permissionsForRole(role)
	if !permissions[SCIMManage] {
		return Principal{}, ErrForbidden
	}
	return Principal{
		UserID: userID, WorkspaceID: workspaceID, Role: role,
		Permissions: permissions, AuthMethod: "scim_token", TokenID: tokenID,
	}, nil
}

func (s *Service) ListSCIMUsers(ctx context.Context, actor Principal, filterField, filterValue string, startIndex, count int) ([]SCIMUser, int, error) {
	if s.require(ctx, actor, SCIMManage) != nil {
		return nil, 0, ErrForbidden
	}
	if startIndex < 1 || count < 1 || count > maxSCIMPageSize {
		return nil, 0, ErrSCIMInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	where := "s.workspace_id = ?"
	args := []any{actor.WorkspaceID}
	if filterField == "userName" {
		where += " AND lower(u.email) = ?"
		args = append(args, strings.ToLower(filterValue))
	} else if filterField == "externalId" {
		where += " AND s.external_id = ?"
		args = append(args, filterValue)
	} else if filterField != "" {
		return nil, 0, ErrSCIMInvalid
	}
	var total int
	countArgs := append([]any(nil), args...)
	if err := s.queryRow(ctx, tx, `SELECT COUNT(*) FROM scim_users s JOIN users u ON u.id = s.user_id WHERE `+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, count, (startIndex - 1))
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT s.external_id, s.user_id, u.email, u.display_name, s.role, s.version,
			s.created_at, s.updated_at, s.active, m.user_id
		FROM scim_users s JOIN users u ON u.id = s.user_id
		LEFT JOIN workspace_members m ON m.workspace_id = s.workspace_id AND m.user_id = s.user_id
		WHERE `+where+` ORDER BY lower(u.email), s.external_id LIMIT ? OFFSET ?
	`), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	users := make([]SCIMUser, 0, count)
	for rows.Next() {
		user, err := scanSCIMUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (s *Service) GetSCIMUser(ctx context.Context, actor Principal, externalID string) (SCIMUser, error) {
	if s.require(ctx, actor, SCIMManage) != nil {
		return SCIMUser{}, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SCIMUser{}, err
	}
	defer tx.Rollback()
	user, err := s.getSCIMUserTx(ctx, tx, actor.WorkspaceID, externalID)
	if err != nil {
		return SCIMUser{}, err
	}
	if err := tx.Commit(); err != nil {
		return SCIMUser{}, err
	}
	return user, nil
}

func (s *Service) CreateSCIMUser(ctx context.Context, actor Principal, input SCIMUserInput, audit AuditContext) (SCIMUser, error) {
	if s.require(ctx, actor, SCIMManage) != nil {
		return SCIMUser{}, ErrForbidden
	}
	email, err := normalizeSCIMEmail(input.UserName, input.Email)
	if err != nil {
		return SCIMUser{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName != "" {
		return SCIMUser{}, ErrSCIMInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SCIMUser{}, err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, SCIMManage) != nil {
		return SCIMUser{}, ErrForbidden
	}
	externalID := strings.TrimSpace(input.ExternalID)
	if len(externalID) > 256 || externalID == "." || externalID == ".." || strings.ContainsAny(externalID, "/\\") {
		return SCIMUser{}, ErrSCIMInvalid
	}
	var userID string
	if externalID != "" {
		var existingEmail string
		err = s.queryRow(ctx, tx, `
			SELECT s.user_id, u.email FROM scim_users s JOIN users u ON u.id = s.user_id
			WHERE s.workspace_id = ? AND s.external_id = ?
		`, actor.WorkspaceID, externalID).Scan(&userID, &existingEmail)
		if err == nil {
			if existingEmail != email {
				return SCIMUser{}, ErrSCIMConflict
			}
			user, loadErr := s.getSCIMUserTx(ctx, tx, actor.WorkspaceID, externalID)
			if loadErr != nil {
				return SCIMUser{}, loadErr
			}
			if err := tx.Commit(); err != nil {
				return SCIMUser{}, err
			}
			return user, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return SCIMUser{}, err
		}
	}
	err = s.queryRow(ctx, tx, "SELECT id FROM users WHERE email = ?", email).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		userID, err = id.New()
		if err != nil {
			return SCIMUser{}, err
		}
		if _, err := s.exec(ctx, tx, `
			INSERT INTO users (id, email, password_hash, display_name, created_at)
			VALUES (?, ?, NULL, ?, ?)
		`, userID, email, displayName, timestamp(s.now().UTC())); err != nil {
			return SCIMUser{}, fmt.Errorf("create SCIM user: %w", err)
		}
	} else if err != nil {
		return SCIMUser{}, err
	}
	var member int
	if err := s.queryRow(ctx, tx, `SELECT 1 FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, actor.WorkspaceID, userID).Scan(&member); err == nil {
		return SCIMUser{}, ErrSCIMConflict
	} else if !errors.Is(err, sql.ErrNoRows) {
		return SCIMUser{}, err
	}
	if externalID == "" {
		externalID = userID
	}
	var mapped int
	if err := s.queryRow(ctx, tx, `SELECT 1 FROM scim_users WHERE workspace_id = ? AND user_id = ?`, actor.WorkspaceID, userID).Scan(&mapped); err == nil {
		return SCIMUser{}, ErrSCIMConflict
	} else if !errors.Is(err, sql.ErrNoRows) {
		return SCIMUser{}, err
	}
	now := s.now().UTC()
	if _, err := s.exec(ctx, tx, `
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)
	`, actor.WorkspaceID, userID, Member, timestamp(now)); err != nil {
		return SCIMUser{}, fmt.Errorf("add SCIM member: %w", err)
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO scim_users (workspace_id, external_id, user_id, role, active, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, TRUE, 1, ?, ?)
	`, actor.WorkspaceID, externalID, userID, Member, timestamp(now), timestamp(now)); err != nil {
		return SCIMUser{}, fmt.Errorf("map SCIM user: %w", err)
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "scim.user.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: actor.WorkspaceID, TargetType: "scim_user", TargetID: externalID,
		Action: "scim.user.create", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"role": Member}), CreatedAt: now,
	}); err != nil {
		return SCIMUser{}, err
	}
	if err := tx.Commit(); err != nil {
		return SCIMUser{}, err
	}
	return SCIMUser{ID: externalID, UserID: userID, ExternalID: externalID, UserName: email, Email: email, DisplayName: displayName, Active: true, Role: Member, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) PatchSCIMUser(ctx context.Context, actor Principal, externalID string, patch SCIMUserPatch, version int64, audit AuditContext) (SCIMUser, error) {
	if s.require(ctx, actor, SCIMManage) != nil {
		return SCIMUser{}, ErrForbidden
	}
	if version < 1 {
		return SCIMUser{}, ErrSCIMPrecondition
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SCIMUser{}, err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, SCIMManage) != nil {
		return SCIMUser{}, ErrForbidden
	}
	current, err := s.getSCIMUserTx(ctx, tx, actor.WorkspaceID, externalID)
	if err != nil {
		return SCIMUser{}, err
	}
	if current.Version != version {
		return SCIMUser{}, ErrSCIMPrecondition
	}
	if patch.UserName != nil || patch.Email != nil || patch.DisplayName != nil {
		return SCIMUser{}, ErrSCIMInvalid
	}
	active := current.Active
	if patch.Active != nil {
		active = *patch.Active
	}
	if !active && current.Role == Owner {
		return SCIMUser{}, ErrLastOwner
	}
	now := s.now().UTC()
	if active && !current.Active {
		if _, err := s.exec(ctx, tx, `
			INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
			VALUES (?, ?, ?, ?)
		`, actor.WorkspaceID, current.UserID, current.Role, timestamp(now)); err != nil {
			return SCIMUser{}, fmt.Errorf("reactivate SCIM user: %w", err)
		}
	}
	if !active && current.Active {
		if _, err := s.exec(ctx, tx, "DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?", actor.WorkspaceID, current.UserID); err != nil {
			return SCIMUser{}, err
		}
		if _, err := s.exec(ctx, tx, "DELETE FROM workspace_notification_preferences WHERE workspace_id = ? AND user_id = ?", actor.WorkspaceID, current.UserID); err != nil {
			return SCIMUser{}, err
		}
		if _, err := s.exec(ctx, tx, "UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", timestamp(now), current.UserID); err != nil {
			return SCIMUser{}, err
		}
		if _, err := s.exec(ctx, tx, "UPDATE api_tokens SET revoked_at = ? WHERE user_id = ? AND workspace_id = ? AND revoked_at IS NULL", timestamp(now), current.UserID, actor.WorkspaceID); err != nil {
			return SCIMUser{}, err
		}
		if _, err := s.exec(ctx, tx, "UPDATE scim_tokens SET revoked_at = ? WHERE created_by_user_id = ? AND workspace_id = ? AND revoked_at IS NULL", timestamp(now), current.UserID, actor.WorkspaceID); err != nil {
			return SCIMUser{}, err
		}
	}
	newVersion := current.Version + 1
	if _, err := s.exec(ctx, tx, `
		UPDATE scim_users SET active = ?, version = ?, updated_at = ?
		WHERE workspace_id = ? AND external_id = ?
	`, active, newVersion, timestamp(now), actor.WorkspaceID, externalID); err != nil {
		return SCIMUser{}, err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "scim.user.updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: actor.WorkspaceID, TargetType: "scim_user", TargetID: externalID,
		Action: "scim.user.update", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"active": active}), CreatedAt: now,
	}); err != nil {
		return SCIMUser{}, err
	}
	if err := tx.Commit(); err != nil {
		return SCIMUser{}, err
	}
	return SCIMUser{ID: externalID, UserID: current.UserID, ExternalID: externalID, UserName: current.UserName, Email: current.Email, DisplayName: current.DisplayName, Active: active, Role: current.Role, Version: newVersion, CreatedAt: current.CreatedAt, UpdatedAt: now}, nil
}

func (s *Service) DeleteSCIMUser(ctx context.Context, actor Principal, externalID string, version int64, audit AuditContext) error {
	_, err := s.PatchSCIMUser(ctx, actor, externalID, SCIMUserPatch{Active: boolPtr(false)}, version, audit)
	return err
}

func (s *Service) ListSCIMGroups(ctx context.Context, actor Principal) ([]SCIMGroup, error) {
	if s.require(ctx, actor, SCIMManage) != nil {
		return nil, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	groups, err := s.listSCIMGroupsTx(ctx, tx, actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Service) GetSCIMGroup(ctx context.Context, actor Principal, groupID string) (SCIMGroup, error) {
	if s.require(ctx, actor, SCIMManage) != nil {
		return SCIMGroup{}, ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SCIMGroup{}, err
	}
	defer tx.Rollback()
	group, err := s.getSCIMGroupTx(ctx, tx, actor.WorkspaceID, groupID)
	if err != nil {
		return SCIMGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return SCIMGroup{}, err
	}
	return group, nil
}

func (s *Service) PatchSCIMGroup(ctx context.Context, actor Principal, groupID string, operations []SCIMGroupOperation, version int64, audit AuditContext) (SCIMGroup, error) {
	if s.require(ctx, actor, SCIMManage) != nil {
		return SCIMGroup{}, ErrForbidden
	}
	if version < 1 || len(operations) == 0 {
		return SCIMGroup{}, ErrSCIMInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return SCIMGroup{}, err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, SCIMManage) != nil {
		return SCIMGroup{}, ErrForbidden
	}
	if err := s.ensureSCIMGroups(ctx, tx, actor.WorkspaceID, actor, audit); err != nil {
		return SCIMGroup{}, err
	}
	group, err := s.getSCIMGroupTx(ctx, tx, actor.WorkspaceID, groupID)
	if err != nil {
		return SCIMGroup{}, err
	}
	if group.Version != version {
		return SCIMGroup{}, ErrSCIMPrecondition
	}
	now := s.now().UTC()
	for _, operation := range operations {
		op := strings.ToLower(strings.TrimSpace(operation.Operation))
		if (op != "add" && op != "remove") || len(operation.Members) == 0 {
			return SCIMGroup{}, ErrSCIMInvalid
		}
		for _, externalID := range operation.Members {
			var userID string
			var currentRole Role
			var active bool
			if err := s.queryRow(ctx, tx, `
				SELECT s.user_id, COALESCE(m.role, s.role), s.active AND m.user_id IS NOT NULL
				FROM scim_users s LEFT JOIN workspace_members m
				  ON m.workspace_id = s.workspace_id AND m.user_id = s.user_id
				WHERE s.workspace_id = ? AND s.external_id = ?
			`, actor.WorkspaceID, externalID).Scan(&userID, &currentRole, &active); err != nil {
				return SCIMGroup{}, ErrSCIMNotFound
			}
			if currentRole == Owner {
				return SCIMGroup{}, ErrLastOwner
			}
			switch op {
			case "add":
				if active {
					if _, err := s.exec(ctx, tx, "UPDATE workspace_members SET role = ? WHERE workspace_id = ? AND user_id = ?", group.Role, actor.WorkspaceID, userID); err != nil {
						return SCIMGroup{}, err
					}
				} else if _, err := s.exec(ctx, tx, "INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)", actor.WorkspaceID, userID, group.Role, timestamp(now)); err != nil {
					return SCIMGroup{}, err
				}
				if _, err := s.exec(ctx, tx, "UPDATE scim_users SET role = ?, active = TRUE, version = version + 1, updated_at = ? WHERE workspace_id = ? AND external_id = ?", group.Role, timestamp(now), actor.WorkspaceID, externalID); err != nil {
					return SCIMGroup{}, err
				}
				if !active || currentRole != group.Role {
					if err := s.audit(ctx, tx, AuditEvent{EventType: "scim.group.member.added", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID, TargetType: "scim_user", TargetID: externalID, Action: "scim.group.member.add", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"role": group.Role}), CreatedAt: now}); err != nil {
						return SCIMGroup{}, err
					}
				}
			case "remove":
				if currentRole != group.Role || !active {
					continue
				}
				if _, err := s.exec(ctx, tx, "DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = ?", actor.WorkspaceID, userID); err != nil {
					return SCIMGroup{}, err
				}
				if _, err := s.exec(ctx, tx, "DELETE FROM workspace_notification_preferences WHERE workspace_id = ? AND user_id = ?", actor.WorkspaceID, userID); err != nil {
					return SCIMGroup{}, err
				}
				if _, err := s.exec(ctx, tx, "UPDATE scim_users SET active = FALSE, version = version + 1, updated_at = ? WHERE workspace_id = ? AND external_id = ?", timestamp(now), actor.WorkspaceID, externalID); err != nil {
					return SCIMGroup{}, err
				}
				if _, err := s.exec(ctx, tx, "UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL", timestamp(now), userID); err != nil {
					return SCIMGroup{}, err
				}
				if _, err := s.exec(ctx, tx, "UPDATE api_tokens SET revoked_at = ? WHERE user_id = ? AND workspace_id = ? AND revoked_at IS NULL", timestamp(now), userID, actor.WorkspaceID); err != nil {
					return SCIMGroup{}, err
				}
				if _, err := s.exec(ctx, tx, "UPDATE scim_tokens SET revoked_at = ? WHERE created_by_user_id = ? AND workspace_id = ? AND revoked_at IS NULL", timestamp(now), userID, actor.WorkspaceID); err != nil {
					return SCIMGroup{}, err
				}
				if err := s.audit(ctx, tx, AuditEvent{EventType: "scim.group.member.removed", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID, TargetType: "scim_user", TargetID: externalID, Action: "scim.group.member.remove", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"role": group.Role}), CreatedAt: now}); err != nil {
					return SCIMGroup{}, err
				}
			}
		}
	}
	newVersion := group.Version + 1
	if _, err := s.exec(ctx, tx, "UPDATE scim_groups SET version = ? WHERE workspace_id = ? AND role = ?", newVersion, actor.WorkspaceID, group.Role); err != nil {
		return SCIMGroup{}, err
	}
	if err := s.audit(ctx, tx, AuditEvent{EventType: "scim.group.updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID, TargetType: "scim_group", TargetID: group.ID, Action: "scim.group.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: metadata(map[string]any{"role": group.Role}), CreatedAt: now}); err != nil {
		return SCIMGroup{}, err
	}
	updated, err := s.getSCIMGroupTx(ctx, tx, actor.WorkspaceID, groupID)
	if err != nil {
		return SCIMGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return SCIMGroup{}, err
	}
	return updated, nil
}

func normalizeSCIMEmail(userName, email string) (string, error) {
	userName = strings.TrimSpace(userName)
	if email == "" {
		email = userName
	}
	normalizedEmail, err := normalizeEmail(email)
	if err != nil || !strings.EqualFold(userName, normalizedEmail) {
		return "", ErrSCIMInvalid
	}
	return normalizedEmail, nil
}

func (s *Service) ensureSCIMMembership(ctx context.Context, tx *sql.Tx, workspaceID, userID string, role Role, active bool, now string) error {
	result, err := s.exec(ctx, tx, `
		UPDATE scim_users SET role = ?, active = ?, version = version + 1, updated_at = ?
		WHERE workspace_id = ? AND user_id = ?
	`, role, active, now, workspaceID, userID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 0 {
		return nil
	}
	_, err = s.exec(ctx, tx, `
		INSERT INTO scim_users (workspace_id, external_id, user_id, role, active, version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 1, ?, ?)
	`, workspaceID, userID, userID, role, active, now, now)
	return err
}

func (s *Service) syncSCIMMembership(ctx context.Context, tx *sql.Tx, workspaceID, userID string, role Role, active bool, now string) error {
	_, err := s.exec(ctx, tx, `
		UPDATE scim_users SET role = ?, active = ?, version = version + 1, updated_at = ?
		WHERE workspace_id = ? AND user_id = ?
	`, role, active, now, workspaceID, userID)
	return err
}

func (s *Service) ensureSCIMGroups(ctx context.Context, tx *sql.Tx, workspaceID string, actor Principal, audit AuditContext) error {
	createdAt := s.now().UTC()
	now := timestamp(createdAt)
	for _, role := range []Role{Admin, Member, Viewer} {
		result, err := s.exec(ctx, tx, `
			INSERT INTO scim_groups (workspace_id, role, version, created_at)
			SELECT ?, ?, 1, ? WHERE NOT EXISTS (
				SELECT 1 FROM scim_groups WHERE workspace_id = ? AND role = ?
			)
		`, workspaceID, role, now, workspaceID, role)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed == 0 {
			continue
		}
		if err := s.audit(ctx, tx, AuditEvent{
			EventType: "scim.group.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
			WorkspaceID: workspaceID, TargetType: "scim_group", TargetID: string(role),
			Action: "scim.group.create", Outcome: "success", RequestID: audit.RequestID,
			SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: createdAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) getSCIMUserTx(ctx context.Context, tx *sql.Tx, workspaceID, externalID string) (SCIMUser, error) {
	row := tx.QueryRowContext(ctx, s.bind(`
		SELECT s.external_id, s.user_id, u.email, u.display_name, s.role, s.version,
			s.created_at, s.updated_at, s.active, m.user_id
		FROM scim_users s JOIN users u ON u.id = s.user_id
		LEFT JOIN workspace_members m ON m.workspace_id = s.workspace_id AND m.user_id = s.user_id
		WHERE s.workspace_id = ? AND s.external_id = ?
	`), workspaceID, externalID)
	return scanSCIMUser(row)
}

func (s *Service) listSCIMGroupsTx(ctx context.Context, tx *sql.Tx, workspaceID string) ([]SCIMGroup, error) {
	groups := make([]SCIMGroup, 0, 3)
	for _, role := range []Role{Admin, Member, Viewer} {
		group, err := s.getSCIMGroupTx(ctx, tx, workspaceID, string(role))
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (s *Service) getSCIMGroupTx(ctx context.Context, tx *sql.Tx, workspaceID, groupID string) (SCIMGroup, error) {
	role := Role(strings.ToLower(strings.TrimSpace(groupID)))
	if role != Admin && role != Member && role != Viewer {
		return SCIMGroup{}, ErrSCIMNotFound
	}
	var version int64
	if err := s.queryRow(ctx, tx, "SELECT version FROM scim_groups WHERE workspace_id = ? AND role = ?", workspaceID, role).Scan(&version); err != nil {
		return SCIMGroup{}, ErrSCIMNotFound
	}
	rows, err := tx.QueryContext(ctx, s.bind(`
		SELECT s.external_id FROM scim_users s JOIN workspace_members m
		  ON m.workspace_id = s.workspace_id AND m.user_id = s.user_id
		WHERE s.workspace_id = ? AND m.role = ? AND s.active = TRUE
		ORDER BY s.external_id
	`), workspaceID, role)
	if err != nil {
		return SCIMGroup{}, err
	}
	defer rows.Close()
	members := make([]string, 0)
	for rows.Next() {
		var externalID string
		if err := rows.Scan(&externalID); err != nil {
			return SCIMGroup{}, err
		}
		members = append(members, externalID)
	}
	if err := rows.Err(); err != nil {
		return SCIMGroup{}, err
	}
	return SCIMGroup{ID: string(role), DisplayName: string(role), Role: role, Version: version, Members: members}, nil
}

type scimScanner interface{ Scan(...any) error }

func scanSCIMUser(scanner scimScanner) (SCIMUser, error) {
	var user SCIMUser
	var userID, role, createdAt, updatedAt string
	var memberID sql.NullString
	var active bool
	if err := scanner.Scan(&user.ExternalID, &userID, &user.Email, &user.DisplayName, &role, &user.Version, &createdAt, &updatedAt, &active, &memberID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SCIMUser{}, ErrSCIMNotFound
		}
		return SCIMUser{}, err
	}
	user.ID, user.UserID, user.UserName, user.Role = user.ExternalID, userID, user.Email, Role(role)
	user.Active = active && memberID.Valid
	var err error
	user.CreatedAt, err = parseTimestamp(createdAt)
	if err != nil {
		return SCIMUser{}, err
	}
	user.UpdatedAt, err = parseTimestamp(updatedAt)
	return user, err
}

func boolPtr(value bool) *bool { return &value }

func SCIMPageSize(count int) int {
	if count == 0 {
		return defaultSCIMPageSize
	}
	return count
}
