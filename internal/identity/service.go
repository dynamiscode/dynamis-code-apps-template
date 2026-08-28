package identity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const (
	defaultSessionLifetime    = 24 * time.Hour
	defaultInvitationLifetime = 7 * 24 * time.Hour
	defaultOIDCLifetime       = 10 * time.Minute
)

var (
	dummyHashOnce sync.Once
	dummyHash     string
	dummyHashErr  error
)

type Service struct {
	db             *sql.DB
	driver         config.DatabaseDriver
	now            func() time.Time
	passwordParams passwordParams
	dummyHash      string
}

func NewService(db *sql.DB, driver config.DatabaseDriver) (*Service, error) {
	dummyHashOnce.Do(func() {
		dummyHash, dummyHashErr = hashPassword(
			"not-a-real-account-password",
			defaultPasswordParams,
		)
	})
	if dummyHashErr != nil {
		return nil, dummyHashErr
	}
	return &Service{
		db:             db,
		driver:         driver,
		now:            time.Now,
		passwordParams: defaultPasswordParams,
		dummyHash:      dummyHash,
	}, nil
}

func (s *Service) BootstrapFirstOwner(
	ctx context.Context,
	input BootstrapInput,
	audit AuditContext,
) (BootstrapResult, error) {
	email, err := normalizeEmail(input.Email)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("%w: %v", ErrInvalidBootstrap, err)
	}
	workspaceName, err := validateWorkspaceName(input.WorkspaceName)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("%w: %v", ErrInvalidBootstrap, err)
	}
	workspaceLocale, err := defaultLocale(input.WorkspaceLocale)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("%w: %v", ErrInvalidBootstrap, err)
	}
	if len(input.Password) < 12 || len(input.Password) > 1024 {
		return BootstrapResult{}, fmt.Errorf(
			"%w: password must be 12 to 1024 characters", ErrInvalidBootstrap,
		)
	}
	passwordHash, err := hashPassword(input.Password, s.passwordParams)
	if err != nil {
		return BootstrapResult{}, err
	}
	userID, err := id.New()
	if err != nil {
		return BootstrapResult{}, err
	}
	workspaceID, err := id.New()
	if err != nil {
		return BootstrapResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("begin bootstrap: %w", err)
	}
	defer tx.Rollback()

	now := s.now().UTC()
	if _, err := s.exec(ctx, tx,
		"INSERT INTO bootstrap_state (id, completed_at) VALUES (?, ?)",
		1, timestamp(now),
	); err != nil {
		return BootstrapResult{}, ErrAlreadyBootstrapped
	}
	if _, err := s.exec(ctx, tx,
		"INSERT INTO users (id, email, password_hash, email_verified_at, created_at) VALUES (?, ?, ?, ?, ?)",
		userID, email, passwordHash, timestamp(now), timestamp(now),
	); err != nil {
		return BootstrapResult{}, fmt.Errorf("create first user: %w", err)
	}
	if _, err := s.exec(ctx, tx,
		"INSERT INTO workspaces (id, name, locale, created_at) VALUES (?, ?, ?, ?)",
		workspaceID, workspaceName, workspaceLocale, timestamp(now),
	); err != nil {
		return BootstrapResult{}, fmt.Errorf("create first workspace: %w", err)
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)
	`, workspaceID, userID, Owner, timestamp(now)); err != nil {
		return BootstrapResult{}, fmt.Errorf("create first owner: %w", err)
	}
	if _, err := s.exec(ctx, tx,
		"INSERT INTO instance_admins (user_id, created_at) VALUES (?, ?)",
		userID, timestamp(now),
	); err != nil {
		return BootstrapResult{}, fmt.Errorf("create instance administrator: %w", err)
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType:     "identity.bootstrap.completed",
		ActorUserID:   userID,
		AuthMethod:    "bootstrap",
		WorkspaceID:   workspaceID,
		TargetType:    "workspace",
		TargetID:      workspaceID,
		Action:        "bootstrap",
		Outcome:       "success",
		RequestID:     audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      metadata(map[string]any{"instance_admin": true}),
		CreatedAt:     now,
	}); err != nil {
		return BootstrapResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return BootstrapResult{}, fmt.Errorf("commit bootstrap: %w", err)
	}
	return BootstrapResult{UserID: userID, WorkspaceID: workspaceID}, nil
}

func (s *Service) IsBootstrapped(ctx context.Context) (bool, error) {
	var one int
	err := s.queryRow(ctx, s.db,
		"SELECT 1 FROM bootstrap_state WHERE id = 1",
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) CreateWorkspace(
	ctx context.Context,
	actor Principal,
	input WorkspaceCreateInput,
	audit AuditContext,
) (string, error) {
	if actor.UserID == "" || actor.AuthMethod == "" {
		return "", ErrForbidden
	}
	name, err := validateWorkspaceName(input.Name)
	if err != nil {
		return "", err
	}
	workspaceLocale, err := defaultLocale(input.Locale)
	if err != nil {
		return "", err
	}
	var exists int
	if err := s.queryRow(ctx, s.db,
		"SELECT 1 FROM users WHERE id = ?", actor.UserID,
	).Scan(&exists); err != nil {
		return "", ErrForbidden
	}
	workspaceID, err := id.New()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := s.exec(ctx, tx,
		"INSERT INTO workspaces (id, name, locale, created_at) VALUES (?, ?, ?, ?)",
		workspaceID, name, workspaceLocale, timestamp(now),
	); err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)
	`, workspaceID, actor.UserID, Owner, timestamp(now)); err != nil {
		return "", fmt.Errorf("create workspace owner: %w", err)
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "workspace.created", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID,
		TargetType: "workspace", TargetID: workspaceID,
		Action: "workspace.create", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return workspaceID, nil
}

func (s *Service) AuthenticateLocal(
	ctx context.Context,
	email string,
	password string,
) (string, error) {
	normalized, normalizeErr := normalizeEmail(email)
	var userID, encoded string
	err := sql.ErrNoRows
	if normalizeErr == nil {
		err = s.queryRow(ctx, s.db,
			"SELECT id, password_hash FROM users WHERE email = ?",
			normalized,
		).Scan(&userID, &encoded)
	}
	if err != nil {
		encoded = s.dummyHash
	}
	valid := verifyPassword(password, encoded)
	if err != nil || !valid {
		return "", ErrInvalidCredentials
	}
	return userID, nil
}

func (s *Service) ReauthenticateLocal(
	ctx context.Context,
	userID string,
	password string,
) error {
	var encoded string
	if err := s.queryRow(ctx, s.db,
		"SELECT password_hash FROM users WHERE id = ?", userID,
	).Scan(&encoded); err != nil || encoded == "" || !verifyPassword(password, encoded) {
		return ErrInvalidCredentials
	}
	return nil
}

func (s *Service) Authorize(
	ctx context.Context,
	userID string,
	workspaceID string,
	permission Permission,
) (Principal, error) {
	return s.authorize(ctx, s.db, userID, workspaceID, permission)
}

func (s *Service) authorize(
	ctx context.Context,
	queryer rowQueryer,
	userID string,
	workspaceID string,
	permission Permission,
) (Principal, error) {
	var role Role
	if err := s.queryRow(ctx, queryer, `
		SELECT role FROM workspace_members
		WHERE workspace_id = ? AND user_id = ?
	`, workspaceID, userID).Scan(&role); err != nil {
		return Principal{}, ErrForbidden
	}
	permissions := permissionsForRole(role)
	if !permissions[permission] {
		return Principal{}, ErrForbidden
	}
	return Principal{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Role:        role,
		Permissions: permissions,
		AuthMethod:  "session",
	}, nil
}

func (s *Service) AuthorizePrincipal(
	ctx context.Context,
	actor Principal,
	workspaceID string,
	permission Permission,
) (Principal, error) {
	return s.authorizePrincipal(ctx, s.db, actor, workspaceID, permission)
}

func (s *Service) AuthorizePrincipalInTx(
	ctx context.Context,
	tx *sql.Tx,
	actor Principal,
	workspaceID string,
	permission Permission,
) (Principal, error) {
	return s.authorizePrincipal(ctx, tx, actor, workspaceID, permission)
}

func (s *Service) ListWorkspaces(
	ctx context.Context,
	userID string,
) ([]WorkspaceSummary, error) {
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT w.id, w.name, m.role, w.locale
		FROM workspace_members m
		JOIN workspaces w ON w.id = m.workspace_id
		WHERE m.user_id = ? ORDER BY lower(w.name), w.id
	`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workspaces := make([]WorkspaceSummary, 0)
	for rows.Next() {
		var workspace WorkspaceSummary
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Role, &workspace.Locale); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, workspace)
	}
	return workspaces, rows.Err()
}

func (s *Service) ListMembers(
	ctx context.Context,
	actor Principal,
) ([]MemberSummary, error) {
	if s.require(ctx, actor, MembersRead) != nil {
		return nil, ErrForbidden
	}
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT m.user_id, u.email, m.role, m.created_at
		FROM workspace_members m JOIN users u ON u.id = m.user_id
		WHERE m.workspace_id = ? ORDER BY lower(u.email), m.user_id
	`), actor.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := make([]MemberSummary, 0)
	for rows.Next() {
		var member MemberSummary
		var createdAt string
		if err := rows.Scan(&member.UserID, &member.Email, &member.Role, &createdAt); err != nil {
			return nil, err
		}
		member.CreatedAt, err = parseTimestamp(createdAt)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}
func (s *Service) authorizePrincipal(
	ctx context.Context,
	queryer rowQueryer,
	actor Principal,
	workspaceID string,
	permission Permission,
) (Principal, error) {
	if actor.WorkspaceID != workspaceID || !actor.Permissions[permission] {
		return Principal{}, ErrForbidden
	}
	current, err := s.authorize(
		ctx, queryer, actor.UserID, workspaceID, permission,
	)
	if err != nil {
		return Principal{}, ErrForbidden
	}
	current.AuthMethod = actor.AuthMethod
	current.TokenID = actor.TokenID
	if actor.AuthMethod == "api_token" {
		current.Permissions = actor.Permissions
	}
	return current, nil
}

func (s *Service) IsInstanceAdmin(ctx context.Context, userID string) bool {
	var one int
	return s.queryRow(ctx, s.db,
		"SELECT 1 FROM instance_admins WHERE user_id = ?",
		userID,
	).Scan(&one) == nil
}

func (s *Service) AddMember(
	ctx context.Context,
	actor Principal,
	userID string,
	role Role,
	audit AuditContext,
) error {
	if s.require(ctx, actor, MembersManage) != nil || !validRole(role) || role == Owner {
		return ErrForbidden
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, MembersManage) != nil {
		return ErrForbidden
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)
	`, actor.WorkspaceID, userID, role, timestamp(now)); err != nil {
		return fmt.Errorf("add workspace member: %w", err)
	}
	if err := s.syncSCIMMembership(ctx, tx, actor.WorkspaceID, userID, role, true, timestamp(now)); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "workspace.member.added", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID,
		TargetType: "user", TargetID: userID, Action: "member.add",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      metadata(map[string]any{"role": role}), CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) ChangeMemberRole(
	ctx context.Context,
	actor Principal,
	userID string,
	role Role,
	audit AuditContext,
) error {
	if s.require(ctx, actor, MembersManage) != nil || !validRole(role) || role == Owner {
		return ErrForbidden
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, MembersManage) != nil {
		return ErrForbidden
	}
	var current Role
	if err := s.queryRow(ctx, tx, `
		SELECT role FROM workspace_members
		WHERE workspace_id = ? AND user_id = ?
	`, actor.WorkspaceID, userID).Scan(&current); err != nil {
		return ErrForbidden
	}
	if current == Owner {
		return ErrLastOwner
	}
	if _, err := s.exec(ctx, tx, `
		UPDATE workspace_members SET role = ?
		WHERE workspace_id = ? AND user_id = ?
	`, role, actor.WorkspaceID, userID); err != nil {
		return err
	}
	if err := s.syncSCIMMembership(ctx, tx, actor.WorkspaceID, userID, role, true, timestamp(now)); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "workspace.member.role_changed", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID,
		TargetType: "user", TargetID: userID, Action: "member.role.change",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      metadata(map[string]any{"from": current, "to": role}), CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) TransferOwnership(
	ctx context.Context,
	actor Principal,
	newOwnerUserID string,
	audit AuditContext,
) error {
	if s.require(ctx, actor, OwnershipTransfer) != nil || actor.UserID == newOwnerUserID {
		return ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var actorRole, targetRole Role
	if err := s.queryRow(ctx, tx, `
		SELECT role FROM workspace_members
		WHERE workspace_id = ? AND user_id = ?
	`, actor.WorkspaceID, actor.UserID).Scan(&actorRole); err != nil || actorRole != Owner {
		return ErrForbidden
	}
	if err := s.queryRow(ctx, tx, `
		SELECT role FROM workspace_members
		WHERE workspace_id = ? AND user_id = ?
	`, actor.WorkspaceID, newOwnerUserID).Scan(&targetRole); err != nil {
		return ErrForbidden
	}
	if _, err := s.exec(ctx, tx, `
		UPDATE workspace_members SET role = ?
		WHERE workspace_id = ? AND user_id = ?
	`, Owner, actor.WorkspaceID, newOwnerUserID); err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, `
		UPDATE workspace_members SET role = ?
		WHERE workspace_id = ? AND user_id = ?
	`, Admin, actor.WorkspaceID, actor.UserID); err != nil {
		return err
	}
	now := s.now().UTC()
	if err := s.syncSCIMMembership(ctx, tx, actor.WorkspaceID, newOwnerUserID, Owner, true, timestamp(now)); err != nil {
		return err
	}
	if err := s.syncSCIMMembership(ctx, tx, actor.WorkspaceID, actor.UserID, Admin, true, timestamp(now)); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "workspace.ownership.transferred", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID,
		TargetType: "user", TargetID: newOwnerUserID,
		Action: "ownership.transfer", Outcome: "success",
		RequestID: audit.RequestID, SourceAddress: audit.SourceAddress,
		Metadata: metadata(map[string]any{"previous_role": targetRole}), CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) RemoveMember(
	ctx context.Context,
	actor Principal,
	userID string,
	audit AuditContext,
) error {
	if s.require(ctx, actor, MembersManage) != nil {
		return ErrForbidden
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.requireTx(ctx, tx, actor, MembersManage) != nil {
		return ErrForbidden
	}
	var role Role
	if err := s.queryRow(ctx, tx, `
		SELECT role FROM workspace_members
		WHERE workspace_id = ? AND user_id = ?
	`, actor.WorkspaceID, userID).Scan(&role); err != nil {
		return ErrForbidden
	}
	if role == Owner {
		return ErrLastOwner
	}
	if _, err := s.exec(ctx, tx, `
		DELETE FROM workspace_members
		WHERE workspace_id = ? AND user_id = ?
	`, actor.WorkspaceID, userID); err != nil {
		return err
	}
	if _, err := s.exec(ctx, tx, `
		DELETE FROM workspace_notification_preferences
		WHERE workspace_id = ? AND user_id = ?
	`, actor.WorkspaceID, userID); err != nil {
		return err
	}
	if err := s.syncSCIMMembership(ctx, tx, actor.WorkspaceID, userID, role, false, timestamp(now)); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, AuditEvent{
		EventType: "workspace.member.removed", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: actor.WorkspaceID,
		TargetType: "user", TargetID: userID, Action: "member.remove",
		Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeEmail(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || len(value) > 320 {
		return "", errors.New("email is invalid")
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value {
		return "", errors.New("email is invalid")
	}
	return value, nil
}

func validateWorkspaceName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > 120 {
		return "", errors.New("workspace name must be 1 to 120 characters")
	}
	return name, nil
}

func timestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseTimestamp(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func metadata(value map[string]any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func (s *Service) bind(query string) string {
	return database.Rebind(s.driver, query)
}

type queryExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type rowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Service) exec(
	ctx context.Context,
	executor queryExecutor,
	query string,
	args ...any,
) (sql.Result, error) {
	return executor.ExecContext(ctx, s.bind(query), args...)
}

func (s *Service) queryRow(
	ctx context.Context,
	queryer rowQueryer,
	query string,
	args ...any,
) *sql.Row {
	return queryer.QueryRowContext(ctx, s.bind(query), args...)
}

func (s *Service) audit(
	ctx context.Context,
	executor queryExecutor,
	event AuditEvent,
) error {
	eventID, err := id.New()
	if err != nil {
		return err
	}
	if event.Metadata == "" {
		event.Metadata = "{}"
	}
	_, err = s.exec(ctx, executor, `
		INSERT INTO audit_events (
			id, event_type, actor_user_id, auth_method, workspace_id,
			target_type, target_id, action, outcome, request_id,
			source_address, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, eventID, event.EventType, nullable(event.ActorUserID), event.AuthMethod,
		nullable(event.WorkspaceID), event.TargetType, nullable(event.TargetID),
		event.Action, event.Outcome, nullable(event.RequestID),
		nullable(event.SourceAddress), event.Metadata, timestamp(event.CreatedAt))
	if err != nil {
		return fmt.Errorf("record audit event: %w", err)
	}
	return nil
}

func (s *Service) RecordAuditInTx(
	ctx context.Context,
	tx *sql.Tx,
	event AuditEvent,
) error {
	return s.audit(ctx, tx, event)
}

func (s *Service) RecordAudit(ctx context.Context, event AuditEvent) error {
	return s.audit(ctx, s.db, event)
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Service) require(
	ctx context.Context,
	actor Principal,
	permission Permission,
) error {
	if _, err := s.AuthorizePrincipal(
		ctx, actor, actor.WorkspaceID, permission,
	); err != nil {
		return ErrForbidden
	}
	return nil
}

func (s *Service) requireTx(
	ctx context.Context,
	tx *sql.Tx,
	actor Principal,
	permission Permission,
) error {
	if _, err := s.authorizePrincipal(
		ctx, tx, actor, actor.WorkspaceID, permission,
	); err != nil {
		return ErrForbidden
	}
	return nil
}
