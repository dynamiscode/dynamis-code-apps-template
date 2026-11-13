package identity

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestSCIMResourceIDsAreShortUserIDs(t *testing.T) {
	service, db := newTestService(t)
	ctx := context.Background()
	bootstrap := bootstrapOwner(t, service)
	owner := mustAuthorize(t, service, bootstrap.UserID, bootstrap.WorkspaceID, WorkspaceUpdate)
	token, err := service.CreateSCIMToken(ctx, owner, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	scimPrincipal, err := service.AuthenticateSCIMToken(ctx, token.Secret, bootstrap.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := service.CreateSCIMUser(ctx, scimPrincipal, SCIMUserInput{
		ExternalID: "directory-7", UserName: "short.id@example.com",
	}, AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != user.UserID.String() || user.ExternalID != "directory-7" {
		t.Fatalf("SCIM resource id = %q, externalId = %q; want short user ID %q and the IdP value", user.ID, user.ExternalID, user.UserID.String())
	}
	notFound := []string{
		user.ExternalID,                      // the IdP value is not our resource id
		"not-an-id",                          // malformed
		"0",                                  // zero key
		user.UserID.String()[:3],             // truncated short ID
		id.EncodeKey(id.TypeUser, 987654321), // well-formed, maps to no user
	}
	for _, resourceID := range notFound {
		if _, err := service.GetSCIMUser(ctx, scimPrincipal, resourceID); !errors.Is(err, ErrSCIMNotFound) {
			t.Fatalf("GetSCIMUser(%q) = %v; want ErrSCIMNotFound", resourceID, err)
		}
	}
	got, err := service.GetSCIMUser(ctx, scimPrincipal, user.ID)
	if err != nil || got.UserID != user.UserID || got.ID != user.ID {
		t.Fatalf("GetSCIMUser(short ID) = %+v, %v", got, err)
	}
	members, err := service.GetSCIMGroup(ctx, scimPrincipal, string(Member))
	if err != nil {
		t.Fatal(err)
	}
	if len(members.Members) != 1 || members.Members[0] != user.ID {
		t.Fatalf("member group members = %v; want [%s]", members.Members, user.ID)
	}
	viewers, err := service.GetSCIMGroup(ctx, scimPrincipal, string(Viewer))
	if err != nil {
		t.Fatal(err)
	}
	byExternalID := []SCIMGroupOperation{{Operation: "add", Members: []string{user.ExternalID}}}
	if _, err := service.PatchSCIMGroup(ctx, scimPrincipal, string(Viewer), byExternalID, viewers.Version, AuditContext{}); !errors.Is(err, ErrSCIMNotFound) {
		t.Fatalf("group add by externalId = %v; want ErrSCIMNotFound", err)
	}
	byShortID := []SCIMGroupOperation{{Operation: "add", Members: []string{user.ID}}}
	if _, err := service.PatchSCIMGroup(ctx, scimPrincipal, string(Viewer), byShortID, viewers.Version, AuditContext{}); err != nil {
		t.Fatalf("group add by short ID = %v", err)
	}
	var targetID string
	if err := db.QueryRow("SELECT target_id FROM audit_events WHERE event_type = 'scim.user.created'").Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if targetID != user.ID {
		t.Fatalf("scim.user.created target_id = %q; want short user ID %q", targetID, user.ID)
	}
}
