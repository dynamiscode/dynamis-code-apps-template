package clients

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type fixture struct {
	ctx         context.Context
	db          *sql.DB
	auth        *identity.Service
	service     *Service
	owner       identity.Principal
	member      identity.Principal
	viewer      identity.Principal
	workspaceID identity.WorkspaceID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	ownerAuth, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	owner := principalFor(t, auth, ownerAuth.UserID, ownerAuth.WorkspaceID, identity.ResourcesWrite)
	member := addMember(t, ctx, db, auth, owner, ownerAuth.WorkspaceID, "member@example.com", identity.Member)
	viewer := addMember(t, ctx, db, auth, owner, ownerAuth.WorkspaceID, "viewer@example.com", identity.Viewer)
	return fixture{
		ctx: ctx, db: db, auth: auth, service: NewService(db, config.SQLite, auth),
		owner: owner, member: member, viewer: viewer, workspaceID: ownerAuth.WorkspaceID,
	}
}

func addMember(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	auth *identity.Service,
	owner identity.Principal,
	workspaceID identity.WorkspaceID,
	email string,
	role identity.Role,
) identity.Principal {
	t.Helper()
	var userID identity.UserID
	if err := db.QueryRowContext(ctx,
		"INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id",
		email, time.Now().UTC().Format(time.RFC3339Nano),
	).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddMember(ctx, owner, userID, role, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	return principalFor(t, auth, userID, workspaceID, identity.ResourcesRead)
}

func principalFor(
	t *testing.T,
	auth *identity.Service,
	userID identity.UserID,
	workspaceID identity.WorkspaceID,
	permission identity.Permission,
) identity.Principal {
	t.Helper()
	principal, err := auth.Authorize(context.Background(), userID, workspaceID, permission)
	if err != nil {
		t.Fatal(err)
	}
	principal.AuthMethod = "test"
	return principal
}

func TestCreateAndGetClientByShortID(t *testing.T) {
	f := newFixture(t)
	created, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{
		DisplayName: "  Ada Lovelace  ", Email: "ada@example.com", Phone: "+1 555 0100", Notes: "Prefers email",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.DisplayName != "Ada Lovelace" || created.Email != "ada@example.com" || created.Version != 1 {
		t.Fatalf("Create() = %+v", created)
	}
	got, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, ParseClientIDOrZero(created.ID.String()))
	if err != nil || got.ID != created.ID || got.Notes != "Prefers email" {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	if _, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "   "}, identity.AuditContext{}); !errors.Is(err, ErrDisplayNameRequired) {
		t.Fatalf("blank display name error = %v, want ErrDisplayNameRequired", err)
	}
}

func TestClientWritesFollowRolesAndAreAudited(t *testing.T) {
	f := newFixture(t)
	if _, err := f.service.Create(f.ctx, f.viewer, f.workspaceID, Input{DisplayName: "Blocked"}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer create error = %v, want ErrForbidden", err)
	}
	created, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Grace"}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("member create error = %v", err)
	}
	if _, err := f.service.Get(f.ctx, f.viewer, f.workspaceID, created.ID); err != nil {
		t.Fatalf("viewer get error = %v", err)
	}
	var events int
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND workspace_id = ? AND target_type = ? AND target_key = ?`,
		"client.created", f.workspaceID, "client", int64(created.ID),
	).Scan(&events); err != nil || events != 1 {
		t.Fatalf("client.created audit events = %d, %v; want 1", events, err)
	}
}

func TestEmailIsOptionalButMustBeABareAddress(t *testing.T) {
	f := newFixture(t)
	for email, ok := range map[string]bool{
		"":                      true,
		"ada@example.com":       true,
		"  ada@example.com  ":   true,
		"Ada <ada@example.com>": false,
		"not-an-email":          false,
		"ada@":                  false,
	} {
		_, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Email case", Email: email}, identity.AuditContext{})
		if ok && err != nil {
			t.Fatalf("Create(email %q) error = %v, want success", email, err)
		}
		if !ok && !errors.Is(err, ErrInvalidEmail) {
			t.Fatalf("Create(email %q) error = %v, want ErrInvalidEmail", email, err)
		}
	}
}

func TestListPagesNewestFirstAndRefusesBadCursors(t *testing.T) {
	f := newFixture(t)
	var ids []ClientID
	for _, name := range []string{"First", "Second", "Third"} {
		created, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: name}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.ID)
	}
	first, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 2})
	if err != nil || len(first.Clients) != 2 || first.Clients[0].ID != ids[2] || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	next, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 2, Cursor: first.NextCursor})
	if err != nil || len(next.Clients) != 1 || next.Clients[0].ID != ids[0] || next.NextCursor != "" {
		t.Fatalf("next page = %+v, %v", next, err)
	}
	if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 2, Cursor: "bad"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("bad cursor error = %v, want ErrInvalidCursor", err)
	}
	if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 0}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero limit error = %v, want ErrInvalidInput", err)
	}
}

func TestClientsAreIsolatedByWorkspace(t *testing.T) {
	f := newFixture(t)
	created, err := f.service.Create(f.ctx, f.owner, f.workspaceID, Input{DisplayName: "Mine"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspace, err := f.auth.CreateWorkspace(f.ctx, f.owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner := principalFor(t, f.auth, f.owner.UserID, otherWorkspace, identity.ResourcesWrite)
	if _, err := f.service.Get(f.ctx, otherOwner, otherWorkspace, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-workspace get error = %v, want ErrNotFound", err)
	}
	page, err := f.service.List(f.ctx, otherOwner, otherWorkspace, ListInput{Limit: 10})
	if err != nil || len(page.Clients) != 0 {
		t.Fatalf("cross-workspace list = %+v, %v", page, err)
	}
	if _, err := f.service.Get(f.ctx, f.owner, f.workspaceID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("zero key get error = %v, want ErrNotFound", err)
	}
}

func TestUpdateChecksVersionRoleAndInputAndIsAudited(t *testing.T) {
	f := newFixture(t)
	created, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: "Before", Phone: "123"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	newName := "After"
	newEmail := "after@example.com"
	updated, err := f.service.Update(f.ctx, f.member, f.workspaceID, created.ID, created.Version,
		UpdateInput{DisplayName: &newName, Email: &newEmail}, identity.AuditContext{})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.DisplayName != "After" || updated.Email != "after@example.com" || updated.Phone != "123" || updated.Version != 2 {
		t.Fatalf("Update() = %+v; want changed name and email, kept phone, version 2", updated)
	}
	if _, err := f.service.Update(f.ctx, f.member, f.workspaceID, created.ID, created.Version,
		UpdateInput{DisplayName: &newName}, identity.AuditContext{}); !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("stale version error = %v, want ErrPreconditionFailed", err)
	}
	blank := "  "
	if _, err := f.service.Update(f.ctx, f.member, f.workspaceID, created.ID, updated.Version,
		UpdateInput{DisplayName: &blank}, identity.AuditContext{}); !errors.Is(err, ErrDisplayNameRequired) {
		t.Fatalf("blank name update error = %v, want ErrDisplayNameRequired", err)
	}
	if _, err := f.service.Update(f.ctx, f.member, f.workspaceID, created.ID, updated.Version,
		UpdateInput{}, identity.AuditContext{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty update error = %v, want ErrInvalidInput", err)
	}
	if _, err := f.service.Update(f.ctx, f.viewer, f.workspaceID, created.ID, updated.Version,
		UpdateInput{DisplayName: &newName}, identity.AuditContext{}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer update error = %v, want ErrForbidden", err)
	}
	var events int
	if err := f.db.QueryRowContext(f.ctx,
		`SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND target_key = ?`,
		"client.updated", int64(created.ID),
	).Scan(&events); err != nil || events != 1 {
		t.Fatalf("client.updated audit events = %d, %v; want 1", events, err)
	}
}
