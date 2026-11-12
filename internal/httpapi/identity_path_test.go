package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// Member, invitation, and API token paths take short IDs only. Malformed,
// unknown, and foreign IDs must return the same not-found response, and
// integer keys must never be accepted in a path.
func TestIdentityRecordPathsAreShortIDOnlyAndUniform(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)
	var ownerUserID string
	if err := db.QueryRow("SELECT id FROM users ORDER BY created_at LIMIT 1").Scan(&ownerUserID); err != nil {
		t.Fatal(err)
	}
	const now = "2026-11-11T00:00:00Z"
	const later = "2026-12-11T00:00:00Z"
	var foreignWorkspace int64
	if err := db.QueryRow(
		"INSERT INTO workspaces (name, created_at) VALUES (?, ?) RETURNING id",
		"Foreign", now,
	).Scan(&foreignWorkspace); err != nil {
		t.Fatal(err)
	}
	var foreignMember, foreignInvitation, foreignToken int64
	if err := db.QueryRow(
		"INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?) RETURNING id",
		foreignWorkspace, ownerUserID, "member", now,
	).Scan(&foreignMember); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`
		INSERT INTO invitations (workspace_id, invited_by_user_id, email, active_email,
			role, secret_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		foreignWorkspace, ownerUserID, "foreign@example.com", "foreign@example.com",
		"member", "foreign-invitation-hash", now, later,
	).Scan(&foreignInvitation); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`
		INSERT INTO api_tokens (user_id, workspace_id, name, secret_hash, scopes, created_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		ownerUserID, foreignWorkspace, "foreign", "foreign-token-hash", "resources:read", now,
	).Scan(&foreignToken); err != nil {
		t.Fatal(err)
	}

	routes := []struct {
		name   string
		method string
		path   string
		body   string
		kind   func(int64) string
		key    int64
	}{
		{"change member", http.MethodPatch, "/members/%s", `{"role":"viewer"}`, func(k int64) string { return identity.MemberID(k).String() }, foreignMember},
		{"remove member", http.MethodDelete, "/members/%s", "", func(k int64) string { return identity.MemberID(k).String() }, foreignMember},
		{"resend invitation", http.MethodPost, "/invitations/%s/resend", "", func(k int64) string { return identity.InvitationID(k).String() }, foreignInvitation},
		{"revoke invitation", http.MethodDelete, "/invitations/%s", "", func(k int64) string { return identity.InvitationID(k).String() }, foreignInvitation},
		{"update token", http.MethodPatch, "/tokens/%s", `{"scopes":["resources:read"]}`, func(k int64) string { return identity.APITokenID(k).String() }, foreignToken},
		{"revoke token", http.MethodDelete, "/tokens/%s", "", func(k int64) string { return identity.APITokenID(k).String() }, foreignToken},
	}

	base := "/api/v1/workspaces/" + workspaceID
	for _, route := range routes {
		probes := map[string]string{
			"malformed":        "1",
			"integer key":      "12",
			"decimal owner":    strconv.FormatInt(route.key, 10),
			"hex legacy form":  "00000000000000000000000000000000",
			"foreign short ID": route.kind(route.key),
			"unknown short ID": route.kind(route.key + 1000),
		}
		var reference map[string]any
		for name, probe := range probes {
			target := base + fmt.Sprintf(route.path, probe)
			response := serveAuthorized(handler, route.method, target, route.body, token, nil)
			if response.Code != http.StatusNotFound {
				t.Fatalf("%s %s (%s) status = %d, want 404: %s", route.name, name, probe, response.Code, response.Body.String())
			}
			assertProblem(t, response, http.StatusNotFound, "not-found")
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			delete(body, "instance")
			delete(body, "requestId")
			if reference == nil {
				reference = body
				continue
			}
			if !reflect.DeepEqual(reference, body) {
				t.Fatalf("%s %s response = %v, want the same body as %v", route.name, name, body, reference)
			}
		}
	}
}
