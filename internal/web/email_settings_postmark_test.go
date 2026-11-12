package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestPostmarkFirstSaveWithoutAccountTokenExplainsWhatIsMissing(t *testing.T) {
	handler, _, auth, _, workspaceID, owner := emailWebTest(t)
	ownerSession, err := auth.CreateSession(t.Context(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: ownerSession.Secret}, {Name: "csrf", Value: ownerSession.CSRFSecret}}
	base := "/workspaces/" + workspaceID.String() + "/settings/email"
	form := url.Values{
		"csrf": {ownerSession.CSRFSecret}, "provider": {"postmark"}, "sender_name": {"Dynamis"},
		"sender_address": {"mailer@example.com"}, "server_token": {"pm_server_good"}, "account_token": {""},
	}
	res := request(handler, http.MethodPost, base, form, cookies, nil)
	want := "Enter the Postmark account token on the first save. Nothing was saved."
	if res.Code != http.StatusUnprocessableEntity || !strings.Contains(res.Body.String(), want) {
		t.Fatalf("first save without account token = %d, want message %q; body=%s", res.Code, want, res.Body.String())
	}
}

func TestPostmarkSettingsRequireTokenChecksBeforeSaving(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/server":
			if r.Header.Get("X-Postmark-Server-Token") != "pm_server_good" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"ID":1}`)
		case "/domains":
			if r.Header.Get("X-Postmark-Account-Token") != "pm_account_good" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"TotalCount":1,"Domains":[{"Name":"example.com","DKIMVerified":true,"ReturnPathDomainVerified":true}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()

	handler, db, auth, email, workspaceID, owner := emailWebTest(t)
	email.SetPostmarkBaseURL(fake.URL)
	base := "/workspaces/" + workspaceID.String() + "/settings/email"
	ownerSession, err := auth.CreateSession(t.Context(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: ownerSession.Secret}, {Name: "csrf", Value: ownerSession.CSRFSecret}}

	page := request(handler, http.MethodGet, base, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `value="postmark"`) ||
		!strings.Contains(page.Body.String(), `id="email-server-token"`) || !strings.Contains(page.Body.String(), `id="email-account-token"`) {
		t.Fatalf("settings page missing Postmark provider = %d", page.Code)
	}

	form := func(serverToken, accountToken string) url.Values {
		return url.Values{
			"csrf": {ownerSession.CSRFSecret}, "provider": {"postmark"}, "sender_name": {"Dynamis"},
			"sender_address": {"mailer@example.com"}, "server_token": {serverToken}, "account_token": {accountToken},
		}
	}
	bad := request(handler, http.MethodPost, base, form("pm_server_bad", "pm_account_good"), cookies, nil)
	if bad.Code != http.StatusUnprocessableEntity || !strings.Contains(bad.Body.String(), "Postmark check failed") {
		t.Fatalf("bad server token save = %d, body=%s", bad.Code, bad.Body.String())
	}
	if strings.Contains(bad.Body.String(), "pm_server_bad") {
		t.Fatal("bad token echoed into the page")
	}
	if count := countWorkspaceEmail(t, db); count != 0 {
		t.Fatalf("bad token stored %d rows", count)
	}

	good := request(handler, http.MethodPost, base, form("pm_server_good", "pm_account_good"), cookies, nil)
	if good.Code != http.StatusSeeOther {
		t.Fatalf("good token save = %d, body=%s", good.Code, good.Body.String())
	}
	if count := countWorkspaceEmail(t, db); count != 1 {
		t.Fatalf("good tokens stored %d rows, want 1", count)
	}
}
