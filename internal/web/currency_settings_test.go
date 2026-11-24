package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

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
	form := url.Values{"csrf": {session.CSRFSecret}, "code": {"USD"}, "decimals_USD": {"2"}, "default": {"__new"}, "new_code": {"EUR"}, "new_decimals": {"2"}}
	response := request(routes, http.MethodPost, path, form, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("save = %d: %s", response.Code, response.Body.String())
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "EUR") || !strings.Contains(page.Body.String(), "USD") {
		t.Fatal("saved currencies missing")
	}
	form.Set("new_decimals", "19")
	response = request(routes, http.MethodPost, path, form, cookies, nil)
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "EUR") {
		t.Fatalf("invalid save = %d: %s", response.Code, response.Body.String())
	}
}
