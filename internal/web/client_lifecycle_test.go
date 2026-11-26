package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// addMemberSession invites a user with role to the env workspace and signs
// them in. It returns the cookies and the CSRF token for that session.
func addMemberSession(t *testing.T, env clientsTestEnv, email string, role identity.Role) ([]*http.Cookie, string) {
	t.Helper()
	ctx := context.Background()
	invitations, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := env.auth.CreateInvitation(ctx, invitations, email, role, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	userID, _, err := env.auth.CreateInvitedLocalUser(ctx, invitation.Secret, "member-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := env.auth.CreateSession(ctx, userID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}, session.CSRFSecret
}

// TestClientArchiveUnarchiveAndDeleteThroughForms covers the client pages.
// Members and above see archive and unarchive. Owners and admins also see a
// confirmed permanent delete. Viewers and members get no delete control. The
// list hides archived clients until the archived selector asks for them.
func TestClientArchiveUnarchiveAndDeleteThroughForms(t *testing.T) {
	env := newClientsTestEnv(t)
	memberCookies, memberCSRF := addMemberSession(t, env, "member@example.com", identity.Member)

	created := request(env.routes, http.MethodPost, env.listPath(), url.Values{
		"csrf": {env.csrf}, "display_name": {"Ada"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	var key int64
	if err := env.db.QueryRow("SELECT id FROM clients WHERE display_name = 'Ada'").Scan(&key); err != nil {
		t.Fatal(err)
	}
	detailPath := env.listPath() + "/" + clients.ClientID(key).String()
	archivePath := detailPath + "/archive"
	unarchivePath := detailPath + "/unarchive"
	deletePath := detailPath + "/delete"

	ownerDetail := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil).Body.String()
	if !strings.Contains(ownerDetail, `action="`+archivePath+`"`) || !strings.Contains(ownerDetail, `action="`+deletePath+`"`) || !strings.Contains(ownerDetail, "data-client-delete") {
		t.Fatalf("owner detail lacks archive or confirmed delete: %s", ownerDetail)
	}
	if !strings.Contains(ownerDetail, `data-confirm="Permanently delete this client, its values, engagements, and engagement history? This cannot be undone."`) || strings.Contains(ownerDetail, "data-detail-editor") {
		t.Fatalf("owner detail lacks delete confirmation or still has the full editor: %s", ownerDetail)
	}
	viewerDetail := request(env.routes, http.MethodGet, detailPath, nil, env.viewerCookies, nil).Body.String()
	if strings.Contains(viewerDetail, archivePath) || strings.Contains(viewerDetail, deletePath) {
		t.Fatalf("viewer detail offers archive or delete: %s", viewerDetail)
	}
	memberDetail := request(env.routes, http.MethodGet, detailPath, nil, memberCookies, nil).Body.String()
	if !strings.Contains(memberDetail, `action="`+archivePath+`"`) || strings.Contains(memberDetail, deletePath) {
		t.Fatalf("member detail = archive offered %v, delete offered %v; want archive only",
			strings.Contains(memberDetail, archivePath), strings.Contains(memberDetail, deletePath))
	}

	if refused := request(env.routes, http.MethodPost, archivePath, url.Values{"csrf": {env.viewerCSRF}}, env.viewerCookies, nil); refused.Code != http.StatusForbidden {
		t.Fatalf("viewer archive = %d, want 403", refused.Code)
	}
	archived := request(env.routes, http.MethodPost, archivePath, url.Values{"csrf": {memberCSRF}}, memberCookies, nil)
	if archived.Code != http.StatusSeeOther || archived.Header().Get("Location") != detailPath {
		t.Fatalf("member archive = %d location %q: %s", archived.Code, archived.Header().Get("Location"), archived.Body.String())
	}
	var archivedAt *string
	if err := env.db.QueryRow("SELECT archived_at FROM clients WHERE id = ?", key).Scan(&archivedAt); err != nil || archivedAt == nil {
		t.Fatalf("archived_at after archive = %v, %v; want set", archivedAt, err)
	}
	archivedDetail := request(env.routes, http.MethodGet, detailPath, nil, memberCookies, nil).Body.String()
	if !strings.Contains(archivedDetail, `action="`+unarchivePath+`"`) || strings.Contains(archivedDetail, `action="`+archivePath+`"`) || strings.Contains(archivedDetail, deletePath) {
		t.Fatalf("archived member detail should offer restore only: %s", archivedDetail)
	}

	defaultList := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String()
	if strings.Contains(defaultList, `href="`+detailPath+`"`) || strings.Contains(defaultList, `href="`+detailPath+`?`) {
		t.Fatalf("default list shows an archived client: %s", defaultList)
	}
	archivedList := request(env.routes, http.MethodGet, env.listPath()+"?all=1&archived=archived", nil, env.cookies, nil).Body.String()
	if !strings.Contains(archivedList, `href="`+detailPath+`?return=`) || !strings.Contains(archivedList, `aria-label="Remove: Archived"`) {
		t.Fatalf("archived filter list lacks the client or a way to clear the filter: %s", archivedList)
	}
	if bad := request(env.routes, http.MethodGet, env.listPath()+"?archived=nope", nil, env.cookies, nil); bad.Code != http.StatusBadRequest {
		t.Fatalf("unknown archived selector = %d, want 400", bad.Code)
	}

	unarchived := request(env.routes, http.MethodPost, unarchivePath, url.Values{"csrf": {memberCSRF}}, memberCookies, nil)
	if unarchived.Code != http.StatusSeeOther {
		t.Fatalf("unarchive = %d: %s", unarchived.Code, unarchived.Body.String())
	}
	if relisted := request(env.routes, http.MethodGet, env.listPath()+"?all=1", nil, env.cookies, nil).Body.String(); !strings.Contains(relisted, `href="`+detailPath+`"`) {
		t.Fatalf("list after unarchive lacks the client: %s", relisted)
	}

	if refused := request(env.routes, http.MethodPost, deletePath, url.Values{"csrf": {memberCSRF}}, memberCookies, nil); refused.Code != http.StatusForbidden {
		t.Fatalf("member delete = %d, want 403", refused.Code)
	}
	unconfirmed := request(env.routes, http.MethodPost, deletePath, url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if unconfirmed.Code != http.StatusOK || !strings.Contains(unconfirmed.Body.String(), "Ada") || !strings.Contains(unconfirmed.Body.String(), `name="confirm_delete"`) {
		t.Fatalf("unconfirmed deletion should display a confirmation: %d %s", unconfirmed.Code, unconfirmed.Body.String())
	}
	if present := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil); present.Code != http.StatusOK {
		t.Fatalf("unconfirmed deletion changed client: %d", present.Code)
	}
	deleted := request(env.routes, http.MethodPost, deletePath, url.Values{"csrf": {env.csrf}, "confirm_delete": {"yes"}}, env.cookies, nil)
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != env.listPath() {
		t.Fatalf("owner delete = %d location %q: %s", deleted.Code, deleted.Header().Get("Location"), deleted.Body.String())
	}
	if gone := request(env.routes, http.MethodGet, detailPath, nil, env.cookies, nil); gone.Code != http.StatusNotFound {
		t.Fatalf("detail after delete = %d, want 404", gone.Code)
	}
}
