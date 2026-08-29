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
	retryUser, err := service.CreateSCIMUser(ctx, scimPrincipal, SCIMUserInput{
		UserName: "retry@example.com",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	replayedWithoutExternalID, err := service.CreateSCIMUser(ctx, scimPrincipal, SCIMUserInput{
		UserName: "retry@example.com",
	}, AuditContext{})
	if err != nil || replayedWithoutExternalID.UserID != retryUser.UserID || replayedWithoutExternalID.ExternalID != retryUser.ExternalID {
		t.Fatalf("SCIM replay without external ID = %+v, %v", replayedWithoutExternalID, err)
	}
	for _, externalID := range []string{".", ".."} {
		if _, err := service.CreateSCIMUser(ctx, scimPrincipal, SCIMUserInput{
			ExternalID: externalID, UserName: "reserved@example.com",
		}, AuditContext{}); !errors.Is(err, ErrSCIMInvalid) {
			t.Fatalf("reserved SCIM external ID %q = %v", externalID, err)
		}
	}
	changedEmail := "attacker@example.com"
	if _, err := service.PatchSCIMUser(ctx, scimPrincipal, user.ExternalID, SCIMUserPatch{Email: &changedEmail}, user.Version, AuditContext{}); !errors.Is(err, ErrSCIMInvalid) {
		t.Fatalf("SCIM account email mutation = %v", err)
	}
	var unchangedEmail string
	if err := db.QueryRow("SELECT email FROM users WHERE id = ?", user.UserID).Scan(&unchangedEmail); err != nil {
		t.Fatal(err)
	}
	if unchangedEmail != "new.user@example.com" {
		t.Fatalf("SCIM changed account email to %q", unchangedEmail)
	}
	changedDisplayName := "Workspace A name"
	if _, err := service.PatchSCIMUser(ctx, scimPrincipal, user.ExternalID, SCIMUserPatch{DisplayName: &changedDisplayName}, user.Version, AuditContext{}); !errors.Is(err, ErrSCIMInvalid) {
		t.Fatalf("SCIM account display-name mutation = %v", err)
	}
	var unchangedDisplayName string
	if err := db.QueryRow("SELECT display_name FROM users WHERE id = ?", user.UserID).Scan(&unchangedDisplayName); err != nil {
		t.Fatal(err)
	}
	if unchangedDisplayName != "" {
		t.Fatalf("SCIM changed account display name to %q", unchangedDisplayName)
	}
	if _, err := service.CreateSCIMUser(ctx, scimPrincipal, SCIMUserInput{
		ExternalID: "directory-display", UserName: "display@example.com", DisplayName: changedDisplayName,
	}, AuditContext{}); !errors.Is(err, ErrSCIMInvalid) {
		t.Fatalf("SCIM create display-name mutation = %v", err)
	}
	claimedID, err := service.AuthenticateOIDC(ctx, ExternalClaims{
		ProviderID: "company", Issuer: "https://id.example.com", Subject: "scim-user-42",
		Email: "new.user@example.com",
	}, AuditContext{})
	if err != nil || claimedID != user.UserID {
		t.Fatalf("SCIM OIDC enrollment = %q, %v", claimedID, err)
	}
	if claimedAgain, err := service.AuthenticateOIDC(ctx, ExternalClaims{
		ProviderID: "company", Issuer: "https://id.example.com", Subject: "scim-user-42",
		Email: "new.user@example.com",
	}, AuditContext{}); err != nil || claimedAgain != user.UserID {
		t.Fatalf("SCIM OIDC replay = %q, %v", claimedAgain, err)
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
	var rotationRevocations int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND target_id = ?", "scim.token.revoked", token.ID).Scan(&rotationRevocations); err != nil {
		t.Fatal(err)
	}
	if rotationRevocations != 1 {
		t.Fatalf("SCIM rotation revocations = %d", rotationRevocations)
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

func TestSCIMGroupETagsTrackMembershipChanges(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceUpdate)
	token, err := service.CreateSCIMToken(ctx, owner, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	scim, err := service.AuthenticateSCIMToken(ctx, token.Secret, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.CreateSCIMUser(ctx, scim, SCIMUserInput{
		ExternalID: "etag-user", UserName: "etag@example.com",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	groups, err := service.ListSCIMGroups(ctx, scim)
	if err != nil {
		t.Fatal(err)
	}
	memberVersion, adminVersion := groups[1].Version, groups[0].Version
	if err := service.ChangeMemberRole(ctx, owner, user.UserID, Admin, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	groups, err = service.ListSCIMGroups(ctx, scim)
	if err != nil {
		t.Fatal(err)
	}
	if groups[1].Version == memberVersion || groups[0].Version == adminVersion {
		t.Fatalf("role change did not advance affected group ETags: %+v", groups)
	}

	adminVersion = groups[0].Version
	currentUser, err := service.GetSCIMUser(ctx, scim, user.ExternalID)
	if err != nil {
		t.Fatal(err)
	}
	deactivated, err := service.PatchSCIMUser(ctx, scim, user.ExternalID, SCIMUserPatch{Active: boolPtr(false)}, currentUser.Version, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	groups, err = service.ListSCIMGroups(ctx, scim)
	if err != nil {
		t.Fatal(err)
	}
	if groups[0].Version == adminVersion {
		t.Fatalf("deactivation did not advance admin group ETag: %+v", groups[0])
	}
	adminVersion = groups[0].Version
	if _, err := service.PatchSCIMUser(ctx, scim, user.ExternalID, SCIMUserPatch{Active: boolPtr(true)}, deactivated.Version, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	groups, err = service.ListSCIMGroups(ctx, scim)
	if err != nil {
		t.Fatal(err)
	}
	if groups[0].Version == adminVersion {
		t.Fatalf("reactivation did not advance admin group ETag: %+v", groups[0])
	}

	second, err := service.CreateSCIMUser(ctx, scim, SCIMUserInput{
		ExternalID: "etag-second", UserName: "etag-second@example.com",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	groups, err = service.ListSCIMGroups(ctx, scim)
	if err != nil {
		t.Fatal(err)
	}
	memberVersion = groups[1].Version
	adminVersion = groups[0].Version
	if _, err := service.PatchSCIMGroup(ctx, scim, "admin", []SCIMGroupOperation{{Operation: "add", Members: []string{second.ExternalID}}}, adminVersion, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	groups, err = service.ListSCIMGroups(ctx, scim)
	if err != nil {
		t.Fatal(err)
	}
	if groups[1].Version == memberVersion {
		t.Fatalf("SCIM group move did not advance old group ETag: %+v", groups[1])
	}
	if _, err := service.PatchSCIMGroup(ctx, scim, "member", []SCIMGroupOperation{{Operation: "remove", Members: []string{second.ExternalID}}}, memberVersion, AuditContext{}); !errors.Is(err, ErrSCIMPrecondition) {
		t.Fatalf("stale old-group ETag error = %v", err)
	}
}

func TestSCIMDeactivationScopesCredentialsToWorkspace(t *testing.T) {
	service, _ := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceUpdate)
	provisioningToken, err := service.CreateSCIMToken(ctx, owner, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	provisioner, err := service.AuthenticateSCIMToken(ctx, provisioningToken.Secret, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.CreateSCIMUser(ctx, provisioner, SCIMUserInput{
		ExternalID: "scoped-user", UserName: "scoped@example.com",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := service.CreateWorkspace(ctx, owner, WorkspaceCreateInput{Name: "Second"}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	secondOwner := mustAuthorize(t, service, bootstrap.UserID, workspaceID, MembersManage)
	if err := service.AddMember(ctx, secondOwner, user.UserID, Member, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.ChangeMemberRole(ctx, secondOwner, user.UserID, Admin, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	secondAdmin := mustAuthorize(t, service, user.UserID, workspaceID, SCIMManage)
	secondAPIToken, err := service.CreateAPIToken(ctx, secondAdmin, "second-workspace", []Permission{ResourcesRead}, nil, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	secondSCIMToken, err := service.CreateSCIMToken(ctx, secondAdmin, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PatchSCIMUser(ctx, provisioner, user.ExternalID, SCIMUserPatch{Active: boolPtr(false)}, user.Version, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateAPIToken(ctx, secondAPIToken.Secret, ResourcesRead, AuditContext{}); err != nil {
		t.Fatalf("cross-workspace API token revoked: %v", err)
	}
	if _, err := service.AuthenticateSCIMToken(ctx, secondSCIMToken.Secret, workspaceID); err != nil {
		t.Fatalf("cross-workspace SCIM token revoked: %v", err)
	}
}

func TestSCIMReadsDoNotCreateState(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, SCIMManage)
	var mappings, groups int
	if err := db.QueryRow("SELECT COUNT(*) FROM scim_users WHERE workspace_id = ?", bootstrap.WorkspaceID).Scan(&mappings); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM scim_groups WHERE workspace_id = ?", bootstrap.WorkspaceID).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if mappings != 1 || groups != 3 {
		t.Fatalf("initial SCIM state = mappings %d, groups %d", mappings, groups)
	}
	if _, err := db.Exec("DELETE FROM scim_users WHERE workspace_id = ?", bootstrap.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM scim_groups WHERE workspace_id = ?", bootstrap.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if users, total, err := service.ListSCIMUsers(ctx, owner, "", "", 1, 50); err != nil || len(users) != 0 || total != 0 {
		t.Fatalf("SCIM user read = %d, %d, %v", len(users), total, err)
	}
	if _, err := service.ListSCIMGroups(ctx, owner); !errors.Is(err, ErrSCIMNotFound) {
		t.Fatalf("SCIM group read = %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM scim_users WHERE workspace_id = ?", bootstrap.WorkspaceID).Scan(&mappings); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM scim_groups WHERE workspace_id = ?", bootstrap.WorkspaceID).Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if mappings != 0 || groups != 0 {
		t.Fatalf("SCIM reads created state = mappings %d, groups %d", mappings, groups)
	}
}

func TestSCIMGroupRemovalCleansUpAndReturnsAfterSelfDeactivation(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, SCIMManage)
	provisioningToken, err := service.CreateSCIMToken(ctx, owner, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	provisioner, err := service.AuthenticateSCIMToken(ctx, provisioningToken.Secret, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.CreateSCIMUser(ctx, provisioner, SCIMUserInput{
		ExternalID: "self-removing", UserName: "self-removing@example.com",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	groups, err := service.ListSCIMGroups(ctx, provisioner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.PatchSCIMGroup(ctx, provisioner, "admin", []SCIMGroupOperation{{Operation: "add", Members: []string{user.ExternalID}}}, groups[0].Version, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	admin := mustAuthorize(t, service, user.UserID, bootstrap.WorkspaceID, SCIMManage)
	if err := service.setNotificationPreference(ctx, user.UserID, bootstrap.WorkspaceID, "mentions", false, AuditContext{}); err != nil {
		t.Fatal(err)
	}
	adminToken, err := service.CreateSCIMToken(ctx, admin, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	adminSCIM, err := service.AuthenticateSCIMToken(ctx, adminToken.Secret, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	groups, err = service.ListSCIMGroups(ctx, adminSCIM)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := service.PatchSCIMGroup(ctx, adminSCIM, "admin", []SCIMGroupOperation{{Operation: "remove", Members: []string{user.ExternalID}}}, groups[0].Version, AuditContext{})
	if err != nil || len(removed.Members) != 0 {
		t.Fatalf("self-removing group = %+v, %v", removed, err)
	}
	var preferences, removals int
	if err := db.QueryRow("SELECT COUNT(*) FROM workspace_notification_preferences WHERE workspace_id = ? AND user_id = ?", bootstrap.WorkspaceID, user.UserID).Scan(&preferences); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE event_type = ? AND target_id = ?", "scim.group.member.removed", user.ExternalID).Scan(&removals); err != nil {
		t.Fatal(err)
	}
	if preferences != 0 || removals != 1 {
		t.Fatalf("group removal cleanup/audit = preferences %d, removals %d", preferences, removals)
	}
}
