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

func TestResendSettingsRequireAPIKeyCheckBeforeSaving(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer re_good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"name":"example.com","status":"verified"}]}`)
	}))
	defer fake.Close()

	handler, db, auth, email, workspaceID, owner := emailWebTest(t)
	email.SetResendBaseURL(fake.URL)
	base := "/workspaces/" + workspaceID.String() + "/settings/email"
	ownerSession, err := auth.CreateSession(t.Context(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: ownerSession.Secret}, {Name: "csrf", Value: ownerSession.CSRFSecret}}

	page := request(handler, http.MethodGet, base, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `value="resend"`) || !strings.Contains(page.Body.String(), `id="email-api-key"`) {
		t.Fatalf("settings page missing Resend provider = %d", page.Code)
	}

	form := func(key string) url.Values {
		return url.Values{
			"csrf": {ownerSession.CSRFSecret}, "provider": {"resend"}, "sender_name": {"Dynamis"},
			"sender_address": {"mailer@example.com"}, "api_key": {key},
		}
	}
	bad := request(handler, http.MethodPost, base, form("re_bad"), cookies, nil)
	if bad.Code != http.StatusUnprocessableEntity || !strings.Contains(bad.Body.String(), "Resend check failed") {
		t.Fatalf("bad key save = %d, body=%s", bad.Code, bad.Body.String())
	}
	if strings.Contains(bad.Body.String(), "re_bad") {
		t.Fatal("bad key echoed into the page")
	}
	if count := countWorkspaceEmail(t, db); count != 0 {
		t.Fatalf("bad key stored %d rows", count)
	}

	good := request(handler, http.MethodPost, base, form("re_good"), cookies, nil)
	if good.Code != http.StatusSeeOther {
		t.Fatalf("good key save = %d, body=%s", good.Code, good.Body.String())
	}
	if count := countWorkspaceEmail(t, db); count != 1 {
		t.Fatalf("good key stored %d rows, want 1", count)
	}
}
