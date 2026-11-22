//go:build browser

package browsertest

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	_ "modernc.org/sqlite"
)

// openAdminWorkspace signs the owner in on the origin and opens the first workspace.
func openAdminWorkspace(t *testing.T, tab context.Context, origin string) string {
	t.Helper()
	navigateTo(t, tab, origin+"/login")
	fill(t, tab, "#email", fixtureOwnerEmail)
	fill(t, tab, "#password", fixturePassword)
	click(t, tab, `button[type="submit"]`)
	eventually(t, tab, `location.pathname === "/" || /^\/workspaces\/[^/]+$/.test(location.pathname)`, "sign-in did not reach a workspace")
	if eval[bool](t, tab, `location.pathname === "/"`) {
		click(t, tab, `.cards a[href^="/workspaces/"]`)
	}
	waitVisible(t, tab, "[data-workspace-home]")
	return regexp.MustCompile(`^(/workspaces/[^/]+)`).FindString(eval[string](t, tab, "location.pathname"))
}

// navigateTo loads an absolute URL in the tab.
func navigateTo(t *testing.T, tab context.Context, url string) {
	t.Helper()
	run(t, tab, chromedp.Navigate(url))
}

// TestUnconfiguredWebhookEncryptionExplainsControls ports the token secret and webhook
// encryption test, run against the server without webhook encryption configured.
func TestUnconfiguredWebhookEncryptionExplainsControls(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	workspacePath := openAdminWorkspace(t, tab, membersURL)
	navigateTo(t, tab, membersURL+workspacePath+"/settings/tokens")
	tokenName := fmt.Sprintf("browser token %d", time.Now().UnixMilli())
	fill(t, tab, "#token-name", tokenName)
	click(t, tab, `form[action$="/settings/tokens"] button[name="action"][value="create"]`)
	waitVisible(t, tab, ".success code")
	secret := eval[string](t, tab, `document.querySelector(".success code").textContent`)
	if secret == "" {
		t.Fatal("token secret is empty")
	}
	navigateTo(t, tab, membersURL+workspacePath+"/settings/tokens")
	eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll(".settings-record")].some((r) => r.textContent.includes(%s) && r.getClientRects().length > 0)`, jsString(tokenName)), "token record not visible")
	expectCount(t, tab, ".success code", 0)
	if strings.Contains(eval[string](t, tab, "document.body.textContent"), secret) {
		t.Fatal("token secret appears after reload")
	}

	navigateTo(t, tab, membersURL+workspacePath+"/settings/webhooks")
	expectText(t, tab, "#webhook-secret-key-warning", regexp.MustCompile("ENCRYPTION_KEY"))
	if !eval[bool](t, tab, `document.querySelector('form[data-persist="webhook-create"] button[value="create"]').disabled`) {
		t.Fatal("webhook create is enabled without encryption")
	}
}

// TestAuditHistoryRendersReadOnlyRoute ports the audit history read-only route test.
func TestAuditHistoryRendersReadOnlyRoute(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	workspacePath := openAdminWorkspace(t, tab, baseURL)
	navigateTo(t, tab, baseURL+workspacePath+"/settings/audit?all=1")
	eventually(t, tab, `[...document.querySelectorAll("h1")].some((h) => /Audit history|Historial de auditoría/i.test(h.textContent))`, "audit heading not shown")
	waitVisible(t, tab, "#audit-table")
	expectCount(t, tab, "#audit-table form", 0)
}

// TestNotificationsKeyboardAndSpanishNavigation ports the notifications keyboard and Spanish test.
func TestNotificationsKeyboardAndSpanishNavigation(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	navigate(t, tab, "/notifications")
	eventually(t, tab, `[...document.querySelectorAll("h1")].some((h) => h.textContent.trim() === "Notifications")`, "notifications heading not shown")
	press(t, tab, "\t")
	expectFocused(t, tab, ".skip-link")
	press(t, tab, "\r")
	eventually(t, tab, `location.hash === "#main"`, "skip link did not move to main")

	navigate(t, tab, "/language?locale=es&return_to="+urlQuery("/notifications"))
	expectAttribute(t, tab, "html", "lang", "es")
	eventually(t, tab, `[...document.querySelectorAll("h1")].some((h) => h.textContent.trim() === "Notificaciones")`, "Spanish heading not shown")
	clickByText(t, tab, "Actualizar notificaciones")
	eventually(t, tab, `[...document.querySelectorAll("h1")].some((h) => h.textContent.trim() === "Notificaciones")`, "heading lost after refresh")
}

// TestNotificationsReplayAfterNativeLifetimeReconnect ports the replay test. The unkeyed
// server ends its event streams after one minute, the configured minimum. The page's
// EventSource reports the ended stream as an error and reconnects with its cursor. A notification
// written while the stream is closed must be replayed to the page and counted.
func TestNotificationsReplayAfterNativeLifetimeReconnect(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	script := `(() => { window.__esErrors = 0; const NativeEventSource = window.EventSource; window.EventSource = class extends NativeEventSource { constructor(...args) { super(...args); this.addEventListener("error", () => { window.__esErrors++; }); } }; })();`
	call(t, tab, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: script})
	signInOn(t, tab, membersURL)
	navigateTo(t, tab, membersURL+"/notifications")
	waitVisible(t, tab, "[data-notification-cursor]")
	eventually(t, tab, `document.querySelector("[data-unread-count]")?.textContent.trim() === "0"`, "unread count did not start at zero")

	// Wait for the stream to end, then write the notification while no stream is open.
	deadline := time.Now().Add(100 * time.Second)
	for eval[int](t, tab, `window.__esErrors`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("event stream did not end within its lifetime")
		}
		time.Sleep(500 * time.Millisecond)
	}
	db, err := sql.Open("sqlite", membersDBFile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO notifications (user_id, workspace_id, notification_type, title, body, created_at)
		SELECT u.id, wm.workspace_id, 'system', 'Reconnect after lifetime', 'Replay body', strftime('%Y-%m-%dT%H:%M:%fZ','now')
		FROM users u JOIN workspace_members wm ON wm.user_id=u.id WHERE u.email=? LIMIT 1`, fixtureOwnerEmail); err != nil {
		t.Fatalf("write notification: %v", err)
	}
	eventuallyWithin(t, tab, `document.querySelector("[data-unread-count]")?.textContent.trim() === "1"`, "replayed notification not counted", 30*time.Second)
}

// signInOn signs the owner in on the origin without navigating away from the origin's login page.
func signInOn(t *testing.T, tab context.Context, origin string) {
	t.Helper()
	navigateTo(t, tab, origin+"/login")
	fill(t, tab, "#email", fixtureOwnerEmail)
	fill(t, tab, "#password", fixturePassword)
	click(t, tab, `button[type="submit"]`)
	eventually(t, tab, `location.pathname === "/" || /^\/workspaces\/[^/]+$/.test(location.pathname)`, "sign-in did not reach a workspace")
}
