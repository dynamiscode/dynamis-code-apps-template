package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSCIMProvisioningLifecycle(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceUpdate)
	token, err := service.CreateSCIMToken(ctx, owner, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(ctx, token.Secret, ResourcesRead, AuditContext{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("SCIM token accepted as API token: %v", err)
	}
	scimPrincipal, err := service.AuthenticateSCIMToken(ctx, token.Secret, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.CreateSCIMUser(ctx, scimPrincipal, SCIMUserInput{
		ExternalID: "directory-42", UserName: "New.User@Example.com", Email: "new.user@example.com",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != Member || !user.Active || user.Version != 1 || user.UserName != "new.user@example.com" {
		t.Fatalf("SCIM user = %+v", user)
	}
	var passwordHash string
	if err := db.QueryRow("SELECT COALESCE(password_hash, '') FROM users WHERE email = ?", "new.user@example.com").Scan(&passwordHash); err != nil {
		t.Fatal(err)
	}
	if passwordHash != "" {
		t.Fatal("SCIM provisioned a local password")
	}
	replayed, err := service.CreateSCIMUser(ctx, scimPrincipal, SCIMUserInput{
		ExternalID: "directory-42", UserName: "new.user@example.com",
	}, AuditContext{})
	if err != nil || replayed.ID != user.ID {
		t.Fatalf("SCIM replay = %+v, %v", replayed, err)
	}
	var userID string
	if err := db.QueryRow("SELECT user_id FROM scim_users WHERE workspace_id = ? AND external_id = ?", bootstrap.WorkspaceID, user.ExternalID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	memberPrincipal := mustAuthorize(t, service, userID, bootstrap.WorkspaceID, ResourcesWrite)
	session, err := service.CreateSession(ctx, userID, "oidc", "company", time.Hour, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	apiToken, err := service.CreateAPIToken(ctx, memberPrincipal, "member", []Permission{ResourcesRead}, nil, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PatchSCIMUser(ctx, scimPrincipal, user.ExternalID, SCIMUserPatch{Active: boolPtr(false)}, user.Version, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	deactivated, err := service.GetSCIMUser(ctx, scimPrincipal, user.ExternalID)
	if err != nil || deactivated.Active {
		t.Fatalf("deactivated SCIM user = %+v, %v", deactivated, err)
	}
	if _, err := service.AuthenticateSession(ctx, session.Secret); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("session after SCIM deactivation = %v", err)
	}
	if _, err := service.AuthenticateAPIToken(ctx, apiToken.Secret, ResourcesRead, AuditContext{}); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("API token after SCIM deactivation = %v", err)
	}
	groups, err := service.ListSCIMGroups(ctx, scimPrincipal)
	if err != nil || len(groups) != 3 {
		t.Fatalf("SCIM groups = %v, %v", groups, err)
	}
	if groups[1].DisplayName != "member" {
		t.Fatalf("SCIM groups order = %+v", groups)
	}
	adminGroup, err := service.PatchSCIMGroup(ctx, scimPrincipal, "admin", []SCIMGroupOperation{{Operation: "add", Members: []string{user.ExternalID}}}, groups[0].Version, AuditContext{})
	if err != nil || len(adminGroup.Members) != 1 || adminGroup.Members[0] != user.ExternalID {
		t.Fatalf("SCIM admin group = %+v, %v", adminGroup, err)
	}
	role, err := service.Authorize(ctx, userID, bootstrap.WorkspaceID, SCIMManage)
	if err != nil || role.Role != Admin {
		t.Fatalf("SCIM group role = %+v, %v", role, err)
	}
	rotated, err := service.CreateSCIMToken(ctx, owner, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateSCIMToken(ctx, token.Secret, bootstrap.WorkspaceID); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("rotated SCIM token = %v", err)
	}
	if err := service.RevokeSCIMToken(ctx, owner, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateSCIMToken(ctx, rotated.Secret, bootstrap.WorkspaceID); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("revoked SCIM token = %v", err)
	}
	if _, err := service.PatchSCIMUser(ctx, scimPrincipal, bootstrap.UserID, SCIMUserPatch{Active: boolPtr(false)}, 1, AuditContext{}); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("owner deactivation = %v", err)
	}
	assertAuditEvent(t, db, "scim.user.created")
	assertAuditEvent(t, db, "scim.user.updated")
	assertAuditEvent(t, db, "scim.token.created")
	assertDatabaseDoesNotContain(t, db, token.Secret)
	assertDatabaseDoesNotContain(t, db, rotated.Secret)
}
