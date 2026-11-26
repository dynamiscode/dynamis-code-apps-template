package web

import (
	"context"
	"database/sql"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	nethtml "golang.org/x/net/html"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func catalogDetailLinks(t *testing.T, body string) []string {
	t.Helper()
	matches := regexp.MustCompile(`href="([^"]+/catalog\?entry=[^"]+)"`).FindAllStringSubmatch(body, -1)
	links := make([]string, 0, len(matches))
	seen := make(map[string]bool)
	for _, match := range matches {
		if !seen[match[1]] {
			links = append(links, match[1])
			seen[match[1]] = true
		}
	}
	if len(links) == 0 {
		t.Fatal("catalog detail link missing")
	}
	return links
}

func TestCatalogArchiveAndDelete(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Consultation"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, cookies, nil)
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Active entry"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	for _, want := range []string{`<td class="list-actions-cell"><details class="dropdown list-menu">`, `list-row-menu-button`, `list-action-item border border-base-300 rounded-field`, `/archive"`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatalf("catalog table menu missing %q", want)
		}
	}
	var detail string
	for _, link := range catalogDetailLinks(t, page.Body.String()) {
		candidate := request(routes, http.MethodGet, link, nil, cookies, nil)
		if strings.Contains(candidate.Body.String(), "<h1") && strings.Contains(candidate.Body.String(), "Consultation") {
			detail = link
			break
		}
	}
	if detail == "" {
		t.Fatal("consultation detail missing")
	}
	page = request(routes, http.MethodGet, detail, nil, cookies, nil)
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
	if !strings.Contains(page.Body.String(), "Consultation") || strings.Contains(page.Body.String(), "Active entry") {
		t.Fatal("archived filter must show only archived entries")
	}
	page = request(routes, http.MethodGet, path+"?includeArchived=true&sort=name&direction=desc&q=care", nil, cookies, nil)
	body := html.UnescapeString(page.Body.String())
	for _, want := range []string{"direction=asc", "includeArchived=true", "q=care"} {
		if !strings.Contains(body, want) {
			t.Fatalf("catalog sort lost %q", want)
		}
	}
	detail = html.UnescapeString(catalogDetailLinks(t, page.Body.String())[0])
	for _, want := range []string{"includeArchived=true", "sort=name", "direction=desc", "q=care"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("catalog detail link lost %q", want)
		}
	}
	detailPage := request(routes, http.MethodGet, detail, nil, cookies, nil)
	if !strings.Contains(html.UnescapeString(detailPage.Body.String()), path+"?direction=desc&includeArchived=true&q=care&sort=name") {
		t.Fatal("catalog back link lost list state")
	}
	if !strings.Contains(page.Body.String(), `/unarchive"`) || !strings.Contains(page.Body.String(), `list-action-item border border-base-300 rounded-field`) {
		t.Fatal("archived catalog row menu missing restore action")
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

func TestCatalogCreateInfersPriceMode(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	for _, item := range []struct{ name, amount string }{{"Manual service", ""}, {"Fixed service", "27"}} {
		response := request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {item.name}, "amount": {item.amount}, "currency": {"USD"}}, cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("create %s: %d", item.name, response.Code)
		}
	}
	body := request(routes, http.MethodGet, path, nil, cookies, nil).Body.String()
	if !strings.Contains(body, "Manual service") || !strings.Contains(body, "Manual price") || !strings.Contains(body, "Fixed service") || !strings.Contains(body, "27") {
		t.Fatal("created prices do not reflect the supplied amounts")
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
	page = request(routes, http.MethodGet, catalogDetailLinks(t, page.Body.String())[0], nil, cookies, nil)
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
	form := url.Values{"csrf": {session.CSRFSecret}, "name": {"Consultation"}, "currency": {"USD"}, "variant": {"Initial"}, "duration_minutes": {"30"}}
	created := request(routes, http.MethodPost, path, form, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Consultation") {
		t.Fatalf("list = %d %s", page.Code, page.Body.String())
	}
	form.Set("name", "Draft <consultation>")
	form.Set("duration_minutes", "-3")
	form.Set("amount", "0.123456789012345678")
	form.Set("currency", "BTC")
	form.Set("variant", "Special")
	form.Set("eligibility", "Student & staff")
	invalid := request(routes, http.MethodPost, path, form, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid = %d %s", invalid.Code, invalid.Body.String())
	}
	for _, want := range []string{`name="name" value="Draft &lt;consultation&gt;"`, `name="duration_minutes" value="-3"`, `name="amount" value="0.123456789012345678"`, `value="BTC" selected`, `name="variant" value="Special"`, `name="eligibility" value="Student &amp; staff"`} {
		if !strings.Contains(invalid.Body.String(), want) {
			t.Errorf("rejected create missing %s", want)
		}
	}
	if strings.Contains(invalid.Body.String(), `name="mode"`) {
		t.Error("create form exposes a price mode selector")
	}
	if !regexp.MustCompile(`(?s)<dialog[^>]* open[^>]*>`).MatchString(invalid.Body.String()) {
		t.Error("rejected create form is collapsed")
	}
	form.Set("variant", "")
	invalid = request(routes, http.MethodPost, path, form, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `name="variant" value=""`) {
		t.Errorf("rejected create did not preserve empty variant: %d", invalid.Code)
	}
}

func TestCatalogCreateDefaultsVariant(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	page := request(routes, http.MethodGet, path, nil, cookies, nil).Body.String()
	if !strings.Contains(page, `name="variant" value="Standard"`) || !strings.Contains(page, "Advanced pricing options") || !strings.Contains(page, `value="USD" selected`) || !strings.Contains(page, "2 decimal places") {
		t.Fatal("default advanced variant is missing")
	}
	created := request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Service"}, "mode": {"manual"}, "currency": {"USD"}}, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create = %d", created.Code)
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil).Body.String()
	if !strings.Contains(page, "Standard") {
		t.Fatal("default variant was not stored")
	}
}

func TestCatalogCurrencySelectionUsesWorkspaceConfiguration(t *testing.T) {
	webHandler, auth, workspaceID, owner := testWebRaw(t, 2)
	principal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := webHandler.currencyService.SetCurrencies(context.Background(), principal, workspaceID, []currency.Currency{{Code: "USD", Decimals: 2}, {Code: "EUR", Decimals: 2, Default: true}}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	page := request(webHandler.Routes(), http.MethodGet, path, nil, cookies, nil).Body.String()
	if !strings.Contains(page, `value="EUR" selected`) || !strings.Contains(page, `value="USD"`) || strings.Contains(page, `value="BTC"`) {
		t.Fatal("create form must offer workspace currencies and select its default")
	}
	for _, want := range []string{
		`for="catalog-create-name"`, `id="catalog-create-name"`,
		`for="catalog-create-duration"`, `id="catalog-create-duration"`,
		`for="catalog-create-amount"`, `id="catalog-create-amount"`,
		`for="catalog-create-variant"`, `id="catalog-create-variant"`,
		`for="catalog-create-eligibility"`, `id="catalog-create-eligibility"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("catalog create form missing %q", want)
		}
	}
}

func TestCatalogCurrencySearchPreservesDrafts(t *testing.T) {
	handler, auth, workspaceID, owner := testWebRaw(t, 2)
	ctx := context.Background()
	principal, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.currencyService.SetCurrencies(ctx, principal, workspaceID, []currency.Currency{{Code: "USD", Decimals: 2, Default: true}, {Code: "EUR", Decimals: 2}, {Code: "BHD", Decimals: 3}}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	routes := handler.Routes()
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	created := request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Seed"}, "currency": {"USD"}, "variant": {"Original"}}, cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatal(created.Code)
	}
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	detail := catalogDetailLinks(t, page.Body.String())[0]
	page = request(routes, http.MethodGet, detail, nil, cookies, nil)
	add := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/prices)"`).FindStringSubmatch(page.Body.String())
	edit := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/prices/[^"/]+/edit)"`).FindStringSubmatch(page.Body.String())
	if len(add) != 2 || len(edit) != 2 {
		t.Fatal("price forms missing")
	}
	for _, action := range []string{path, add[1], edit[1]} {
		form := url.Values{"csrf": {session.CSRFSecret}, "name": {"Draft name"}, "mode": {"fixed"}, "amount": {"1.23"}, "currency": {"USD"}, "variant": {"Draft variant"}, "currency_search": {"eur"}, "picker_action": {"currency"}}
		response := request(routes, http.MethodPost, action, form, cookies, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("search %s: %d", action, response.Code)
		}
		matches := regexp.MustCompile(`(?s)<form[^>]*method="post"[^>]*action="` + regexp.QuoteMeta(action) + `"[^>]*>.*?</form>`).FindString(response.Body.String())
		if matches == "" {
			t.Fatalf("search %s missing POST form", action)
		}
		for _, want := range []string{`name="currency_search" value="eur"`, `name="amount" value="1.23"`, `name="variant" value="Draft variant"`} {
			if !strings.Contains(matches, want) {
				t.Errorf("search %s missing %s", action, want)
			}
		}
		doc, err := nethtml.Parse(strings.NewReader(matches))
		if err != nil {
			t.Fatal(err)
		}
		type optionState struct{ selected, hidden bool }
		options := make(map[string]optionState)
		var visit func(*nethtml.Node, bool)
		visit = func(node *nethtml.Node, inCurrency bool) {
			if node.Type == nethtml.ElementNode && node.Data == "select" {
				inCurrency = false
				for _, attr := range node.Attr {
					if attr.Key == "name" && attr.Val == "currency" {
						inCurrency = true
					}
				}
			}
			if inCurrency && node.Type == nethtml.ElementNode && node.Data == "option" {
				var value string
				var state optionState
				for _, attr := range node.Attr {
					switch attr.Key {
					case "value":
						value = attr.Val
					case "selected":
						state.selected = true
					case "hidden":
						state.hidden = true
					}
				}
				options[value] = state
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child, inCurrency)
			}
		}
		visit(doc, false)
		if usd, ok := options["USD"]; !ok || !usd.selected || usd.hidden {
			t.Errorf("search %s did not preserve the visible USD selection", action)
		}
		if eur, ok := options["EUR"]; !ok || eur.hidden {
			t.Errorf("search %s did not offer the matching EUR option", action)
		}
		if bhd, ok := options["BHD"]; ok && !bhd.hidden {
			t.Errorf("search %s offered nonmatching BHD", action)
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
	ownerPage = request(routes, http.MethodGet, catalogDetailLinks(t, ownerPage.Body.String())[0], nil, ownerCookies, nil)
	edit := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/field)"`).FindStringSubmatch(ownerPage.Body.String())
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
	if denied := request(routes, http.MethodPost, strings.TrimSuffix(edit[1], "/field")+"/edit", url.Values{"csrf": {session.CSRFSecret}, "name": {"Denied"}}, cookies, nil); denied.Code != http.StatusForbidden {
		t.Fatalf("viewer edit = %d", denied.Code)
	}
	if denied := request(routes, http.MethodPost, edit[1], url.Values{"csrf": {session.CSRFSecret}, "field": {"name"}, "version": {"Viewer entry"}, "value": {"Denied"}}, cookies, nil); denied.Code != http.StatusForbidden {
		t.Fatalf("viewer inline edit = %d", denied.Code)
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
	webHandler, auth, workspaceID, owner := testWebRaw(t, 2)
	enableTestDinar(t, webHandler, auth, workspaceID, owner.UserID)
	routes := webHandler.Routes()
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Service"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	detail := catalogDetailLinks(t, page.Body.String())[0]
	page = request(routes, http.MethodGet, detail, nil, cookies, nil)
	for _, want := range []string{`data-detail-editor`, `data-detail-dialog`, `data-detail-cancel`, `data-detail-fallback-cancel`, `group-open:hidden`, `Initial`, `USD`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("catalog detail missing %s", want)
		}
	}
	matches := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/field)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("entry inline form missing")
	}
	entryPath := strings.TrimSuffix(matches[1], "/field")
	if strings.Contains(page.Body.String(), `action="`+entryPath+`/edit"`) {
		t.Fatal("redundant entry edit form present")
	}
	response := request(routes, http.MethodPost, entryPath+"/edit", url.Values{"csrf": {session.CSRFSecret}, "version": {"1"}, "name": {"Updated"}, "duration_minutes": {"45"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("edit: %d", response.Code)
	}
	response = request(routes, http.MethodPost, entryPath+"/prices", url.Values{"csrf": {session.CSRFSecret}, "mode": {"fixed"}, "amount": {"0.123"}, "currency": {"BHD"}, "variant": {"Visit"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create price: %d", response.Code)
	}
	page = request(routes, http.MethodGet, detail, nil, cookies, nil)
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
	page = request(routes, http.MethodGet, detail, nil, cookies, nil)
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
	links := catalogDetailLinks(t, page.Body.String())
	var entries [][]string
	for _, link := range links {
		entryPage := request(routes, http.MethodGet, link, nil, cookies, nil)
		match := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/field)"`).FindStringSubmatch(entryPage.Body.String())
		if len(match) == 2 {
			match[1] = strings.TrimSuffix(match[1], "/field") + "/edit"
		}
		entries = append(entries, match)
	}
	if len(entries) != 2 || len(entries[0]) != 2 || len(entries[1]) != 2 {
		t.Fatal("entry forms missing")
	}
	entryPath := strings.TrimSuffix(entries[0][1], "/edit")
	added := request(routes, http.MethodPost, entryPath+"/prices", url.Values{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Other"}}, cookies, nil)
	if added.Code != http.StatusSeeOther {
		t.Fatalf("add price: %d", added.Code)
	}
	page = request(routes, http.MethodGet, links[0], nil, cookies, nil)
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
		{"entry", entries[0][1], url.Values{"csrf": {session.CSRFSecret}, "version": {"1"}, "name": {"Draft <entry>"}, "duration_minutes": {"-3"}}, []string{`name="name" value="Draft &lt;entry&gt;"`, `name="duration_minutes" value="-3"`}},
		{"edit price", priceForms[0][1], url.Values{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "amount": {"0.123456789012345678"}, "currency": {"BTC"}, "variant": {"Draft <edit>"}, "eligibility": {"Student & staff"}}, []string{`value="manual" selected`, `name="amount" value="0.123456789012345678"`, `value="BTC" selected`, `name="variant" value="Draft &lt;edit&gt;"`, `name="eligibility" value="Student &amp; staff"`}},
		{"add price", entryPath + "/prices", url.Values{"csrf": {session.CSRFSecret}, "mode": {"manual"}, "amount": {"0.123456789012345678"}, "currency": {"EUR"}, "variant": {"Draft <add>"}, "eligibility": {"Student & staff"}}, []string{`value="manual" selected`, `name="amount" value="0.123456789012345678"`, `value="EUR" selected`, `name="variant" value="Draft &lt;add&gt;"`, `name="eligibility" value="Student &amp; staff"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := request(routes, http.MethodPost, tc.action, tc.values, cookies, nil)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("rejected = %d", response.Code)
			}
			body := response.Body.String()
			openForm := regexp.MustCompile(`(?s)<details[^>]*data-detail-dirty-on-load[^>]* open[^>]*>.*?<dialog open[^>]*data-detail-dialog[^>]*>.*?<form[^>]*action="` + regexp.QuoteMeta(tc.action) + `"`)
			if !openForm.MatchString(body) {
				t.Error("rejected form is not open in its editor")
			}
			form := formBody(body, tc.action)
			for _, want := range []string{`role="alert" tabindex="-1"`, `name="csrf"`, `data-detail-cancel`, `data-detail-fallback-cancel`} {
				if !strings.Contains(form, want) {
					t.Errorf("rejected editor missing %s", want)
				}
			}
			for _, want := range tc.want {
				if !strings.Contains(formBody(body, tc.action), want) {
					t.Errorf("submitted form missing %s", want)
				}
			}
			for _, other := range []string{priceForms[1][1]} {
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
	webHandler, auth, workspaceID, owner := testWebRaw(t, 2)
	enableTestDinar(t, webHandler, auth, workspaceID, owner.UserID)
	routes := webHandler.Routes()
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Service"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Initial"}}, cookies, nil)
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	page = request(routes, http.MethodGet, catalogDetailLinks(t, page.Body.String())[0], nil, cookies, nil)
	matches := regexp.MustCompile(`action="([^"]+/prices)"`).FindStringSubmatch(page.Body.String())
	if len(matches) != 2 {
		t.Fatal("missing price form")
	}
	pricePath := matches[1]
	for _, form := range []url.Values{{"csrf": {session.CSRFSecret}, "mode": {"fixed"}, "amount": {"0.123"}, "currency": {"BHD"}, "variant": {"Visit"}}, {"csrf": {session.CSRFSecret}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Week"}}} {
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
	if !strings.Contains(page.Body.String(), "0.123") || !strings.Contains(page.Body.String(), "Week") {
		t.Fatal(page.Body.String())
	}
}

func enableTestDinar(t *testing.T, handler *Handler, auth *identity.Service, workspaceID identity.WorkspaceID, userID identity.UserID) {
	t.Helper()
	principal, err := auth.Authorize(context.Background(), userID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.currencyService.SetCurrencies(context.Background(), principal, workspaceID, []currency.Currency{{Code: "USD", Decimals: 2, Default: true}, {Code: "BHD", Decimals: 3}}); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogScopeSwapsAndFallsBackToFullPage(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), `hx-target="#catalog-view"`) || !strings.Contains(page.Body.String(), `name="includeArchived"`) || !strings.Contains(page.Body.String(), `value="false" selected`) {
		t.Fatal("catalog status filter lacks default or enhancement")
	}
	fragment := request(routes, http.MethodGet, path+"?includeArchived=true", nil, cookies, map[string]string{"HX-Request": "true"})
	if fragment.Code != http.StatusOK || !strings.HasPrefix(fragment.Body.String(), `<div id="catalog-view">`) || strings.Contains(fragment.Body.String(), "<!doctype") || !strings.Contains(fragment.Body.String(), "No archived catalog entries.") {
		t.Fatal("catalog scope did not return the archived fragment")
	}
	full := request(routes, http.MethodGet, path+"?includeArchived=true", nil, cookies, nil)
	if !strings.Contains(full.Body.String(), "<!doctype html>") {
		t.Fatal("normal navigation must return the full page")
	}
	restored := request(routes, http.MethodGet, path+"?includeArchived=true", nil, cookies, map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"})
	if !strings.Contains(restored.Body.String(), "<!doctype html>") {
		t.Fatal("history restoration must return the full page")
	}
	filtered := request(routes, http.MethodGet, path+"?includeArchived=true&sort=name&direction=desc&q=care", nil, cookies, nil)
	body := filtered.Body.String()
	for _, want := range []string{`value="true" selected`, `name="sort" value="name"`, `name="direction" value="desc"`, `name="q" value="care"`, `Status: Archived`} {
		if !strings.Contains(body, want) {
			t.Fatalf("catalog filter lost %q", want)
		}
	}
}

func TestCatalogDetailUsesEntryAsOnlyMainHeading(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Consultation"}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Standard"}}, cookies, nil)
	list := request(routes, http.MethodGet, path, nil, cookies, nil)
	detail := request(routes, http.MethodGet, catalogDetailLinks(t, list.Body.String())[0], nil, cookies, nil).Body.String()
	if strings.Count(detail, "<h1") != 1 || !strings.Contains(detail, "<h1>Consultation</h1>") || !strings.Contains(detail, ">Price options</h2>") {
		t.Fatal("detail must have one entry heading followed by the price section")
	}
	if strings.Index(detail, "<main") > strings.Index(detail, "<h1") {
		t.Fatal("main heading must be inside page content")
	}
}

func TestWorkspacePagesUseOneContentHeading(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	base := "/workspaces/" + workspaceID.String()
	for path, title := range map[string]string{base: "Example", base + "/catalog": "Catalog", base + "/settings/general": "General workspace settings", base + "/settings/members": "Members"} {
		response := request(routes, http.MethodGet, path, nil, cookies, nil)
		body := response.Body.String()
		if response.Code != http.StatusOK || strings.Count(body, "<h1") != 1 || !strings.Contains(body, "<h1>"+title+"</h1>") || strings.Index(body, "<main") > strings.Index(body, "<h1") {
			t.Errorf("%s must have a single content heading %q (status %d)", path, title, response.Code)
		}
	}
}

func TestCatalogInlineFieldsAndListSorting(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/catalog"
	for _, entry := range []struct{ name, duration string }{{"Zulu", "30"}, {"Alpha", "60"}} {
		response := request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {entry.name}, "duration_minutes": {entry.duration}, "mode": {"manual"}, "currency": {"USD"}, "variant": {"Standard"}}, cookies, nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("create = %d", response.Code)
		}
	}
	sorted := request(routes, http.MethodGet, path+"?sort=name&direction=asc", nil, cookies, nil)
	body := sorted.Body.String()
	if !strings.Contains(body, `aria-sort="ascending"`) || strings.Index(body, "Alpha") >= strings.Index(body, "Zulu") || !strings.Contains(body, "<table") {
		t.Fatalf("sorted catalog table missing or out of order: %s", body)
	}
	desc := request(routes, http.MethodGet, path+"?sort=duration&direction=desc", nil, cookies, nil).Body.String()
	if !strings.Contains(desc, `aria-sort="descending"`) || strings.Index(desc, "Alpha") >= strings.Index(desc, "Zulu") {
		t.Fatalf("duration sort missing or out of order: %s", desc)
	}
	links := catalogDetailLinks(t, body)
	detail := request(routes, http.MethodGet, links[0], nil, cookies, nil).Body.String()
	match := regexp.MustCompile(`action="([^"]+/catalog/[^"/]+/field)"`).FindStringSubmatch(detail)
	if len(match) != 2 || !strings.Contains(detail, `data-inline-saved="Alpha"`) {
		t.Fatal("inline editor missing")
	}
	action := match[1]
	post := func(field, version, value string) *httptest.ResponseRecorder {
		return request(routes, http.MethodPost, action, url.Values{"csrf": {session.CSRFSecret}, "field": {field}, "version": {version}, "value": {value}}, cookies, nil)
	}
	if response := request(routes, http.MethodPost, action, url.Values{"csrf": {"wrong"}, "field": {"name"}, "version": {"1"}, "value": {"Denied"}}, cookies, nil); response.Code != http.StatusForbidden {
		t.Fatalf("invalid csrf = %d", response.Code)
	}
	if response := post("invalid", "1", "Denied"); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid field = %d", response.Code)
	}
	if response := post("duration_minutes", "1", "45"); response.Code != http.StatusSeeOther {
		t.Fatalf("duration update = %d: %s", response.Code, response.Body.String())
	}
	if response := post("name", "2", "Renamed"); response.Code != http.StatusSeeOther {
		t.Fatalf("name update = %d: %s", response.Code, response.Body.String())
	}
	conflict := post("name", "1", "Draft")
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `value="Draft"`) || !strings.Contains(conflict.Body.String(), `name="version" value="3"`) || !strings.Contains(conflict.Body.String(), `data-inline-explicit-retry`) {
		t.Fatalf("stale update = %d: %s", conflict.Code, conflict.Body.String())
	}
	if response := post("name", "3", "Alpha"); response.Code != http.StatusSeeOther {
		t.Fatalf("restore name = %d", response.Code)
	}
	if response := post("name", "4", "Renamed"); response.Code != http.StatusSeeOther {
		t.Fatalf("repeat name = %d", response.Code)
	}
	if response := post("name", "3", "ABA draft"); response.Code != http.StatusConflict {
		t.Fatalf("ABA inline = %d", response.Code)
	}
	full := request(routes, http.MethodPost, strings.TrimSuffix(action, "/field")+"/edit", url.Values{"csrf": {session.CSRFSecret}, "version": {"3"}, "name": {"Full draft"}, "duration_minutes": {"30"}}, cookies, nil)
	if full.Code != http.StatusConflict || !strings.Contains(full.Body.String(), `value="Full draft"`) || !strings.Contains(full.Body.String(), `name="version" value="5"`) {
		t.Fatalf("stale full edit = %d: %s", full.Code, full.Body.String())
	}
	invalid := post("duration_minutes", "5", "-1")
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `value="-1"`) || !strings.Contains(invalid.Body.String(), `id="catalog-inline-duration_minutes-error" role="alert"`) || !strings.Contains(invalid.Body.String(), `data-inline-dirty-on-load open`) || !strings.Contains(invalid.Body.String(), `data-inline-cancel`) {
		t.Fatalf("invalid duration = %d: %s", invalid.Code, invalid.Body.String())
	}
	page := request(routes, http.MethodGet, links[0], nil, cookies, nil)
	if !strings.Contains(page.Body.String(), `data-inline-saved="Renamed"`) || !strings.Contains(page.Body.String(), `data-inline-saved="45"`) {
		t.Fatalf("saved values missing: %s", page.Body.String())
	}
}
