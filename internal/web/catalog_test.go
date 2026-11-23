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
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Consultation"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, cookies, nil)
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
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"In use"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	matches := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/delete)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("delete form missing")
	}
	response := request(routes, http.MethodPost, matches[1], url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "Archive it instead") {
		t.Fatalf("delete in use = %d %s", response.Code, response.Body.String())
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
	form := url.Values{"csrf": {session.CSRFSecret}, "name": {"Consultation"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}, "duration_minutes": {"30"}}
	created := request(routes, http.MethodPost, path, form, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Consultation") || !strings.Contains(page.Body.String(), "30 min") {
		t.Fatalf("list = %d %s", page.Code, page.Body.String())
	}
	form.Set("name", " ")
	form.Set("mode", "manual")
	form.Set("amount", "0.123456789012345678")
	form.Set("currency", "BTC")
	form.Set("variant", "Special")
	form.Set("eligibility", "Student & staff")
	invalid := request(routes, http.MethodPost, path, form, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid = %d %s", invalid.Code, invalid.Body.String())
	}
	for _, want := range []string{`value="manual" selected`, `name="amount" value="0.123456789012345678"`, `name="currency" value="BTC"`, `name="variant" value="Special"`, `name="eligibility" value="Student &amp; staff"`} {
		if !strings.Contains(invalid.Body.String(), want) {
			t.Errorf("rejected create missing %s", want)
		}
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
	created := request(routes, http.MethodPost, path, url.Values{"csrf": {ownerSession.CSRFSecret}, "name": {"Viewer entry"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, ownerCookies, nil)
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
	ownerPage := request(routes, http.MethodGet, path, nil, ownerCookies, nil)
	edit := regexp.MustCompile(`action="([^"]+/edit)"`).FindStringSubmatch(ownerPage.Body.String())
	if len(edit) != 2 {
		t.Fatal("missing owner edit form")
	}
	response := request(routes, http.MethodGet, path, nil, cookies, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Viewer entry") {
		t.Fatalf("catalog = %d", response.Code)
	}
	if strings.Contains(response.Body.String(), `name="name"`) || strings.Contains(response.Body.String(), "/archive") || strings.Contains(response.Body.String(), "/delete") {
		t.Fatal("viewer sees write controls")
	}
	if strings.Contains(response.Body.String(), "/edit") {
		t.Fatal("viewer sees edit controls")
	}
	if denied := request(routes, http.MethodPost, edit[1], url.Values{"csrf": {session.CSRFSecret}, "name": {"Denied"}}, cookies, nil); denied.Code != http.StatusForbidden {
		t.Fatalf("viewer edit = %d", denied.Code)
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

func TestCatalogEditWeb(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Service"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	matches := regexp.MustCompile(`action="([^"]+/edit)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("entry edit form missing")
	}
	response := request(routes, http.MethodPost, matches[1], url.Values{"csrf": {session.CSRFSecret}, "name": {"Updated"}, "duration_minutes": {"45"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit: %d", response.Code)
	}
	entryPath := strings.TrimSuffix(matches[1], "/edit")
	response = request(routes, http.MethodPost, entryPath+"/prices", url.Values{"csrf": {session.CSRFSecret}, "mode": {"fixed"}, "amount": {"0.123456789012345678"}, "currency": {"BTC"}, "variant": {"Visit"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create price: %d", response.Code)
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	matches = regexp.MustCompile(`action="([^"]+/prices/[^"]+/edit)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("price edit form missing")
	}
	invalid := request(routes, http.MethodPost, matches[1], url.Values{"csrf": {session.CSRFSecret}, "mode": {"fixed"}, "amount": {"-1"}, "currency": {"USD"}, "variant": {"Week"}}, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid price: %d", invalid.Code)
	}
	response = request(routes, http.MethodPost, matches[1], url.Values{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Week"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit price: %d", response.Code)
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "Updated") || !strings.Contains(page.Body.String(), "Week") {
		t.Fatal("edits missing")
	}
	response = request(routes, http.MethodPost, strings.TrimSuffix(matches[1], "/edit")+"/delete", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("delete price: %d", response.Code)
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	matches = regexp.MustCompile(`action="([^"]+/prices/[^"]+/edit)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("remaining price form missing")
	}
	response = request(routes, http.MethodPost, strings.TrimSuffix(matches[1], "/edit")+"/delete", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "must have at least one price") {
		t.Fatalf("delete last price: %d %s", response.Code, response.Body.String())
	}
}

func TestCatalogRejectedFormsKeepOnlyTheirValues(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	for _, name := range []string{"First", "Second"} {
		response := request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {name}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Original"}}, cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("create: %d", response.Code)
		}
	}
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	entries := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/edit)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(entries) != 2 {
		t.Fatalf("entry forms: %d", len(entries))
	}
	entryPath := strings.TrimSuffix(entries[0][1], "/edit")
	added := request(routes, http.MethodPost, entryPath+"/prices", url.Values{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Other"}}, cookies, nil)
	if added.Code != http.StatusSeeOther {
		t.Fatalf("add price: %d", added.Code)
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	priceForms := regexp.MustCompile(`action="(`+regexp.QuoteMeta(entryPath)+`/prices/[^"/]+/edit)"`).FindAllStringSubmatch(page.Body.String(), -1)
	if len(priceForms) != 2 {
		t.Fatal("price edit forms missing")
	}
	formBody := func(body, action string) string {
		t.Helper()
		match := regexp.MustCompile(`(?s)<form[^>]*action="` + regexp.QuoteMeta(action) + `"[^>]*>.*?</form>`).FindString(body)
		if match == "" {
			t.Fatalf("form missing: %s", action)
		}
		return match
	}
	cases := []struct {
		name, action string
		values       url.Values
		want         []string
	}{
		{"entry", entries[0][1], url.Values{"csrf": {session.CSRFSecret}, "name": {"Draft <entry>"}, "duration_minutes": {"-3"}}, []string{`name="name" value="Draft &lt;entry&gt;"`, `name="duration_minutes" value="-3"`}},
		{"edit price", priceForms[0][1], url.Values{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "amount": {"0.123456789012345678"}, "currency": {"BTC"}, "variant": {"Draft <edit>"}, "eligibility": {"Student & staff"}}, []string{`value="manual" selected`, `name="amount" value="0.123456789012345678"`, `name="currency" value="BTC"`, `name="variant" value="Draft &lt;edit&gt;"`, `name="eligibility" value="Student &amp; staff"`}},
		{"add price", entryPath + "/prices", url.Values{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "amount": {"0.123456789012345678"}, "currency": {"EUR"}, "variant": {"Draft <add>"}, "eligibility": {"Student & staff"}}, []string{`value="manual" selected`, `name="amount" value="0.123456789012345678"`, `name="currency" value="EUR"`, `name="variant" value="Draft &lt;add&gt;"`, `name="eligibility" value="Student &amp; staff"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := request(routes, http.MethodPost, tc.action, tc.values, cookies, nil)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("rejected = %d", response.Code)
			}
			body := response.Body.String()
			for _, want := range tc.want {
				if !strings.Contains(formBody(body, tc.action), want) {
					t.Errorf("submitted form missing %s", want)
				}
			}
			for _, other := range []string{path, entries[1][1], priceForms[1][1]} {
				if strings.Contains(formBody(body, other), "Draft") || strings.Contains(formBody(body, other), "Student &amp; staff") {
					t.Errorf("unrelated form contains submitted values: %s", other)
				}
			}
			if tc.name == "edit price" && strings.Contains(formBody(body, entryPath+"/prices"), "Draft") {
				t.Error("add form contains edit price values")
			}
			if tc.name == "add price" && strings.Contains(formBody(body, priceForms[0][1]), "Draft") {
				t.Error("edit form contains add price values")
			}
		})
	}
	archived := request(routes, http.MethodPost, entryPath+"/archive", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if archived.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d", archived.Code)
	}
	rejected := request(routes, http.MethodPost, entries[0][1], cases[0].values, cookies, nil)
	if rejected.Code != http.StatusUnprocessableEntity || !strings.Contains(formBody(rejected.Body.String(), entries[0][1]), `Draft &lt;entry&gt;`) {
		t.Fatalf("archived entry form not restored: %d", rejected.Code)
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
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Service"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	matches := regexp.MustCompile(`action="([^"]+/prices)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("missing price form")
	}
	pricePath := matches[1]
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
