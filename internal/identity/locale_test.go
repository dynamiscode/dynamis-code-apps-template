package identity

import (
	"context"
	"errors"
	"testing"
)

func TestLocalePersistenceAndWorkspaceFallback(t *testing.T) {
	service, db := newTestService(t)
	defer db.Close()
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)

	if got, err := service.GetUserLocale(ctx, bootstrap.UserID); err != nil || got != "" {
		t.Fatalf("initial user locale = %q, %v", got, err)
	}
	if err := service.SetUserLocale(ctx, bootstrap.UserID, "es", AuditContext{RequestID: "locale-user"}); err != nil {
		t.Fatal(err)
	}
	if got, err := service.GetUserLocale(ctx, bootstrap.UserID); err != nil || got != "es" {
		t.Fatalf("saved user locale = %q, %v", got, err)
	}
	if err := service.SetUserLocale(ctx, bootstrap.UserID, "", AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if got, err := service.GetUserLocale(ctx, bootstrap.UserID); err != nil || got != "" {
		t.Fatalf("reset user locale = %q, %v", got, err)
	}
	if err := service.SetUserLocale(ctx, bootstrap.UserID, "fr", AuditContext{}); !errors.Is(err, ErrInvalidLocale) {
		t.Fatalf("invalid user locale error = %v", err)
	}

	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceUpdate)
	if err := service.UpdateWorkspaceLocale(ctx, owner, "es", AuditContext{RequestID: "locale-workspace"}); err != nil {
		t.Fatal(err)
	}
	if got, err := service.GetWorkspaceLocale(ctx, bootstrap.WorkspaceID); err != nil || got != "es" {
		t.Fatalf("saved workspace locale = %q, %v", got, err)
	}
	created, err := service.CreateWorkspace(ctx, owner, WorkspaceCreateInput{Name: "Spanish", Locale: "es"}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.GetWorkspaceLocale(ctx, created); err != nil || got != "es" {
		t.Fatalf("created workspace locale = %q, %v", got, err)
	}
	defaulted, err := service.CreateWorkspace(ctx, owner, WorkspaceCreateInput{Name: "Default"}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := service.GetWorkspaceLocale(ctx, defaulted); err != nil || got != "en" {
		t.Fatalf("default workspace locale = %q, %v", got, err)
	}
	if err := service.UpdateWorkspaceLocale(ctx, owner, "fr", AuditContext{}); !errors.Is(err, ErrInvalidLocale) {
		t.Fatalf("invalid workspace locale error = %v", err)
	}

	viewerID := insertUser(t, service, db, "viewer-locale@example.com")
	insertMembership(t, service, db, bootstrap.WorkspaceID, viewerID, Viewer)
	viewer := mustAuthorize(t, service, viewerID, bootstrap.WorkspaceID, WorkspaceRead)
	if err := service.UpdateWorkspaceLocale(ctx, viewer, "en", AuditContext{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer workspace locale error = %v", err)
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE event_type IN ('user.locale.updated', 'workspace.locale.updated')`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits < 3 {
		t.Fatalf("locale audits = %d, want at least 3", audits)
	}
}
