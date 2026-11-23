package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestCatalogArchiveAndDelete(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Consultation"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	matches := regexp.MustCompile(`action="([^"]+/archive)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("archive form missing")
	}
	response := request(routes, http.MethodPost, matches[1], url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d", response.Code)
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	if strings.Contains(page.Body.String(), "Consultation") {
		t.Fatal("archived entry in default list")
	}
	page = request(routes, http.MethodGet, path+"?includeArchived=true", nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "Consultation") {
		t.Fatal("archived entry missing")
	}
	unarchive := strings.TrimSuffix(matches[1], "/archive") + "/unarchive"
	response = request(routes, http.MethodPost, unarchive, url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("unarchive = %d", response.Code)
	}
	response = request(routes, http.MethodPost, strings.TrimSuffix(matches[1], "/archive")+"/delete", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("delete = %d", response.Code)
	}
}

func TestCatalogDeleteInUse(t *testing.T) {
	webHandler, auth, workspaceID, owner, db := testWebWithMFARawDB(t, 2, false)
	webHandler.EnableCatalog(catalog.NewService(db, auth, func(context.Context, *sql.Tx, identity.WorkspaceID, catalog.EntryID) (bool, error) { return true, nil }))
	routes := webHandler.Routes()
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"In use"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	matches := regexp.MustCompile(`action="([^"]+/delete)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("delete form missing")
	}
	response := request(routes, http.MethodPost, matches[1], url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "Archive it instead") {
		t.Fatalf("delete in use = %d", response.Code)
	}
}

func TestCatalogCreate(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	form := url.Values{"csrf": {session.CSRFSecret}, "name": {"Consultation"}, "duration_minutes": {"30"}}
	created := request(routes, http.MethodPost, path, form, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Consultation") || !strings.Contains(page.Body.String(), "30 min") {
		t.Fatalf("list = %d %s", page.Code, page.Body.String())
	}
	form.Set("name", " ")
	invalid := request(routes, http.MethodPost, path, form, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestCatalogMemberCannotCreate(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	ctx := context.Background()
	actor, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, actor, "member@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "long-enough-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, user, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), `name="name"`) {
		t.Fatalf("member list = %d %s", page.Code, page.Body.String())
	}
	response := request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Denied"}}, cookies, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("member create = %d", response.Code)
	}
}

func TestCatalogList(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	ctx := context.Background()
	ownerSession, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	ownerCookies := []*http.Cookie{{Name: "session", Value: ownerSession.Secret}, {Name: "csrf", Value: ownerSession.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	created := request(routes, http.MethodPost, path, url.Values{"csrf": {ownerSession.CSRFSecret}, "name": {"Viewer entry"}}, ownerCookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d", created.Code)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, actor, "viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewer, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "viewer-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, viewer, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	response := request(routes, http.MethodGet, path, nil, cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Viewer entry") {
		t.Fatalf("catalog = %d", response.Code)
	}
	if strings.Contains(response.Body.String(), `name="name"`) || strings.Contains(response.Body.String(), "/archive") || strings.Contains(response.Body.String(), "/delete") {
		t.Fatal("viewer sees write controls")
	}
	deniedWrite := request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Denied"}}, cookies, nil)
	if deniedWrite.Code != http.StatusForbidden {
		t.Fatalf("viewer write = %d", deniedWrite.Code)
	}
	home := request(routes, http.MethodGet, "/workspaces/"+workspaceID.String(), nil, cookies, nil)
	if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), path) {
		t.Fatalf("workspace home lacks catalog link: %d", home.Code)
	}
	foreignPath := "/workspaces/" + identity.WorkspaceID(999999).String() + "/catalog"
	foreign := request(routes, http.MethodGet, foreignPath, nil, cookies, nil)
	if foreign.Code == http.StatusOK {
		t.Fatal("catalog returned 200 outside workspace")
	}
	denied := request(routes, http.MethodGet, path, nil, nil, nil)
	if denied.Code == http.StatusOK {
		t.Fatal("unauthenticated catalog returned 200")
	}
}

func TestCatalogPricesWeb(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Service"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	doc := page.Body.String()
	marker := path + "/"
	start := strings.Index(doc, marker)
	if start < 0 {
		t.Fatal("missing price form")
	}
	end := strings.Index(doc[start:], "/prices")
	if end < 0 {
		t.Fatal("missing price action")
	}
	pricePath := doc[start:start+end] + "/prices"
	for _, form := range []url.Values{{"csrf": {session.CSRFSecret}, "mode": {"fixed"}, "amount": {"0.123456789012345678"}, "currency": {"BTC"}, "variant": {"Visit"}}, {"csrf": {session.CSRFSecret}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Week"}}} {
		result := request(routes, http.MethodPost, pricePath, form, cookies, nil)
		if result.Code != http.StatusSeeOther {
			t.Fatalf("price = %d %s", result.Code, result.Body.String())
		}
	}
	for _, form := range []url.Values{
		{"csrf": {session.CSRFSecret}, "mode": {"fixed"}, "amount": {"-1"}, "currency": {"USD"}, "variant": {"Visit"}},
		{"csrf": {session.CSRFSecret}, "mode": {"fixed"}, "currency": {"USD"}, "variant": {"Visit"}},
		{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "variant": {"Visit"}},
		{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "currency": {"USD"}},
	} {
		invalid := request(routes, http.MethodPost, pricePath, form, cookies, nil)
		if invalid.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid = %d", invalid.Code)
		}
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "0.123456789012345678") || !strings.Contains(page.Body.String(), "Week") {
		t.Fatal(page.Body.String())
	}
}
