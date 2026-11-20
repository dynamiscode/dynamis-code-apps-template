//go:build browser

package browsertest

import (
	"database/sql"
	"fmt"
	"regexp"
	"testing"

	_ "modernc.org/sqlite"
)

// TestAccountAndSecurityPagesRender ports the account page layout checks per viewport and theme.
// The screenshot comparisons are deferred to the baseline recapture.
func TestAccountAndSecurityPagesRender(t *testing.T) {
	for _, layout := range []struct {
		name          string
		width, height int64
		theme         string
	}{
		{"desktop-light", 1280, 900, "light"}, {"desktop-dark", 1280, 900, "dark"},
		{"mobile-light", 390, 844, "light"}, {"mobile-dark", 390, 844, "dark"},
	} {
		t.Run(layout.name, func(t *testing.T) {
			tab := newTab(t, layout.width, layout.height, layout.theme)
			setTheme(t, tab, layout.theme)
			signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
			restore := setAccountTheme(t, tab, layout.theme)
			t.Cleanup(restore)
			for _, page := range []struct{ path, heading, locale string }{
				{"/account", "Account", "en"},
				{"/settings/language", "Configuración de idioma", "es"},
				{"/security", "Security", "en"},
				{"/sessions", "Sessions", "en"},
			} {
				navigate(t, tab, "/language?locale="+page.locale+"&return_to="+urlQuery(page.path))
				navigate(t, tab, page.path)
				expectAttribute(t, tab, "html", "lang", page.locale)
				expectAttribute(t, tab, "html", "data-theme", layout.theme)
				eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll("h1")].some((h) => h.textContent.includes(%s))`, jsString(page.heading)), "heading "+page.heading+" missing")
				if layout.width < 608 {
					expectNoOverflow(t, tab, layout.width)
				}
			}
		})
	}
}

// TestProfilePreferencePersistsThroughOrdinaryForm ports the profile persistence test.
func TestProfilePreferencePersistsThroughOrdinaryForm(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	navigate(t, tab, "/account")
	original := eval[map[string]string](t, tab, `(() => { const form = document.querySelector('form[action="/account/profile"]'); return { csrf: form.querySelector('[name="csrf"]').value, timezone: form.querySelector('[name="timezone"]').value, locale: form.querySelector('[name="locale"]').value, display_name: form.querySelector('[name="display_name"]').value, theme: form.querySelector('[name="theme"]').value }; })()`)
	t.Cleanup(func() {
		navigate(t, tab, "/account")
		eval[any](t, tab, fmt.Sprintf(`(async () => { const body = new URLSearchParams(%s); await fetch("/account/profile", { method: "POST", body, redirect: "manual", credentials: "same-origin" }); })()`, jsPairs(original)), awaitPromise)
	})
	fillByLabel(t, tab, "Display name", "Playwright profile")
	clickByText(t, tab, "Save")
	eventually(t, tab, `location.search.includes("saved=1")`, "profile save did not confirm")
	eventually(t, tab, `document.querySelector('form[action="/account/profile"] [name="display_name"]').value === "Playwright profile"`, "saved display name not kept")
	status := eval[string](t, tab, fmt.Sprintf(`(async () => { const body = new URLSearchParams([["csrf", %s], ["timezone", %s], ["locale", %s], ["display_name", %s], ["theme", %s]]); return String((await fetch("/account/profile", { method: "POST", body, redirect: "manual", credentials: "same-origin" })).type); })()`, jsString(original["csrf"]), jsString(original["timezone"]), jsString(original["locale"]), jsString(original["display_name"]), jsString(original["theme"])), awaitPromise)
	if status != "opaqueredirect" {
		t.Fatalf("profile post type %s, want a redirect", status)
	}
}

// jsPairs renders a map as URLSearchParams constructor input.
func jsPairs(values map[string]string) string {
	pairs := make([]string, 0, len(values))
	for key, value := range values {
		pairs = append(pairs, fmt.Sprintf("[%s, %s]", jsString(key), jsString(value)))
	}
	return "[" + joinComma(pairs) + "]"
}

func joinComma(parts []string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += ","
		}
		out += part
	}
	return out
}

// TestSessionRevokedFromOrdinaryForm ports the session revocation test. A second browser
// signs in as the same owner, and the first browser revokes that session.
func TestSessionRevokedFromOrdinaryForm(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	other := newTab(t, 1280, 900, "")
	signInAs(t, other, fixtureOwnerEmail, fixturePassword)

	navigate(t, tab, "/sessions")
	waitVisible(t, tab, ".session-card")
	clickByText(t, tab, "Revoke")
	eventually(t, tab, `location.search.includes("revoked=1")`, "revoke did not confirm")
	expectText(t, tab, `[role="status"]`, regexp.MustCompile("Session revoked"))
}

// TestExpiredSessionRejectedAndListed ports the expired session test: the database marks a
// second session expired, its next request redirects to sign-in, and the list shows it expired.
func TestExpiredSessionRejectedAndListed(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	second := newTab(t, 1280, 900, "")
	signInAs(t, second, fixtureOwnerEmail, fixturePassword)

	db, err := sql.Open("sqlite", appDBFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE sessions SET expires_at = strftime('%Y-%m-%dT%H:%M:%fZ','now','-1 minute')
		WHERE id = (
			SELECT s.id FROM sessions s JOIN users u ON u.id = s.user_id
			WHERE u.email = ? AND s.revoked_at IS NULL
			ORDER BY s.created_at DESC LIMIT 1
		)`, fixtureOwnerEmail); err != nil {
		t.Fatalf("expire session: %v", err)
	}

	navigate(t, second, "/account")
	eventually(t, second, `/\/login(\?|$)/.test(location.pathname + location.search)`, "expired session was not sent to sign-in")
	navigate(t, tab, "/sessions")
	waitVisible(t, tab, ".session-card")
	eventually(t, tab, `[...document.querySelectorAll(".session-card")].some((c) => c.textContent.includes("Expired") && !c.querySelector('form[action^="/sessions/"]'))`, "expired session not listed without a revoke control")
}
