package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/identity"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestWebThemePreferenceAppliesToFirstPaint(t *testing.T) {
	mailer := &recordingMailer{}
	handler, auth, _, owner := testWeb(t, 10, mailer)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	saved := request(handler, http.MethodPost, "/account/profile", url.Values{
		"csrf": {session.CSRFSecret}, "display_name": {"Owner"}, "locale": {""},
		"timezone": {"UTC"}, "theme": {"dark"},
	}, cookies, nil)
	if saved.Code != http.StatusSeeOther {
		t.Fatalf("save theme = %d, %s", saved.Code, saved.Body.String())
	}
	if mirror := responseCookie(saved, themeCookie); mirror == nil || mirror.Value != "dark" {
		t.Fatalf("theme mirror cookie = %+v, want dark", mirror)
	}

	account := request(handler, http.MethodGet, "/account", nil, cookies, nil)
	if !strings.Contains(account.Body.String(), `data-theme="dark"`) {
		t.Fatalf("signed-in page missing data-theme dark: %s", account.Body.String())
	}
	workspaces, err := auth.ListWorkspaces(context.Background(), owner.UserID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := request(handler, http.MethodGet, "/workspaces/"+workspaces[0].ID.String(), nil, cookies, nil)
	if !strings.Contains(workspace.Body.String(), `data-theme="dark"`) {
		t.Fatalf("workspace home missing data-theme dark: %s", workspace.Body.String())
	}

	stale := append([]*http.Cookie{}, cookies...)
	stale = append(stale, &http.Cookie{Name: themeCookie, Value: "light"})
	resynced := request(handler, http.MethodGet, "/account", nil, stale, nil)
	if !strings.Contains(resynced.Body.String(), `data-theme="dark"`) {
		t.Fatalf("stored preference must win over stale cookie: %s", resynced.Body.String())
	}
	if mirror := responseCookie(resynced, themeCookie); mirror == nil || mirror.Value != "dark" {
		t.Fatalf("stale mirror not refreshed: %+v", mirror)
	}

	anonymous := request(handler, http.MethodGet, "/login", nil, []*http.Cookie{{Name: themeCookie, Value: "light"}}, nil)
	if !strings.Contains(anonymous.Body.String(), `data-theme="light"`) {
		t.Fatalf("anonymous page should use mirrored cookie: %s", anonymous.Body.String())
	}

	invalid := request(handler, http.MethodGet, "/login", nil, []*http.Cookie{{Name: themeCookie, Value: "blue"}}, nil)
	if !strings.Contains(invalid.Body.String(), `<html lang="en">`) {
		t.Fatalf("invalid cookie should fall back to system: %s", invalid.Body.String())
	}
}

func TestAccountAndLanguageStatesRenderAcrossLocalesAndThemes(t *testing.T) {
	handler, auth, _, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	for _, locale := range []string{"en", "es"} {
		for _, theme := range []string{"light", "dark", "system"} {
			if err := auth.UpdateUserProfile(context.Background(), owner.UserID, identity.ProfileUpdateInput{
				DisplayName: "Owner", Locale: locale, Timezone: "UTC", Theme: theme,
			}, identity.AuditContext{}); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/account", "/settings/language"} {
				response := request(handler, http.MethodGet, path, nil, cookies, nil)
				if response.Code != http.StatusOK || response.Header().Get("Content-Language") != locale ||
					!hasHTMLRootLang(response.Body.String(), locale) ||
					(theme != "system" && !strings.Contains(response.Body.String(), `data-theme="`+theme+`"`)) ||
					(theme == "system" && strings.Contains(response.Body.String(), `data-theme=`)) {
					t.Fatalf("%s locale=%s theme=%s: status=%d language=%q", path, locale, theme, response.Code, response.Header().Get("Content-Language"))
				}
			}
		}
	}
}
