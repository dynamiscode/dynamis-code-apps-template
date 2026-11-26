package web

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestCurrencyNavigationAndViewerPermissions(t *testing.T) {
	_, _, workspaceID, _ := testWeb(t, 10)
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en", Workspace: identity.WorkspaceSummary{ID: workspaceID}}
	for _, canWrite := range []bool{false, true} {
		var body bytes.Buffer
		if err := currencySettingsView(data, nil, nil, "", "", "", false, canWrite).Render(context.Background(), &body); err != nil {
			t.Fatal(err)
		}
		page := body.String()
		if !strings.Contains(page, `class="btn btn-outline btn-sm" href="/workspaces/`+workspaceID.String()+`/settings/general"`) {
			t.Error("currency settings must provide a visibly bounded native back link")
		}
		if strings.Contains(page, `name="new_code"`) != canWrite {
			t.Errorf("currency write controls visibility differs from canWrite=%v", canWrite)
		}
		if strings.Contains(page, `name="new_decimals"`) || strings.Contains(page, `name="decimals_USD"`) {
			t.Error("currency precision must not be editable")
		}
	}
}

func TestCurrencySettingsSaveAndValidation(t *testing.T) {
	routes, auth, workspaceID, owner := testWeb(t, 2)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/settings/currencies"
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "USD") {
		t.Fatalf("initial currencies: %d %s", page.Code, page.Body.String())
	}
	form := url.Values{"csrf": {session.CSRFSecret}, "default": {"__new"}, "new_code": {"EUR"}}
	response := request(routes, http.MethodPost, path, form, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("save = %d: %s", response.Code, response.Body.String())
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "EUR") || !strings.Contains(page.Body.String(), "USD") {
		t.Fatal("saved currencies missing")
	}
	form = url.Values{"csrf": {session.CSRFSecret}, "default": {"USD"}, "remove_USD": {"1"}}
	response = request(routes, http.MethodPost, path, form, cookies, nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("disabled default = %d: %s", response.Code, response.Body.String())
	}
	form = url.Values{"csrf": {session.CSRFSecret}, "default": {"EUR"}}
	form.Set("new_code", "NOTACURRENCY")
	response = request(routes, http.MethodPost, path, form, cookies, nil)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "EUR") {
		t.Fatalf("invalid save = %d: %s", response.Code, response.Body.String())
	}
	page = request(routes, http.MethodGet, path+"?q=JPY", nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `value="JPY"`) || strings.Contains(page.Body.String(), `value="GBP"`) {
		t.Fatalf("filtered currencies = %d: %s", page.Code, page.Body.String())
	}
}
