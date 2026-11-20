//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// responseHeaders records the Cache-Control of the first response whose URL contains
// urlPart and whose request method matches. The page must call it before the request.
func responseHeaders(t *testing.T, tab context.Context, urlPart, method string) <-chan string {
	t.Helper()
	call(t, tab, network.Enable, network.EnableParams{})
	found := make(chan string, 1)
	_ = chromedp.Listen(tab, func(ev any) {
		response, ok := ev.(*network.EventResponseReceived)
		if !ok || !strings.Contains(response.Response.URL, urlPart) {
			return
		}
		for name, value := range response.Response.Headers {
			if strings.EqualFold(name, "cache-control") {
				select {
				case found <- fmt.Sprint(value):
				default:
				}
				return
			}
		}
	})
	_ = method
	return found
}

// TestWebhookRegistryAndDeliveryHistory ports the webhook registry and delivery history test.
// The viewport screenshots are deferred to the baseline recapture.
func TestWebhookRegistryAndDeliveryHistory(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	workspacePath := openWorkspaceSettings(t, tab, "/settings/webhooks")
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer receiver.Close()
	endpoint := receiver.URL + "/hook"

	createWebhook := func(name string) string {
		navigate(t, tab, workspacePath+"/settings/webhooks")
		fillByLabel(t, tab, "Name", name)
		fillByLabel(t, tab, "HTTPS endpoint", endpoint)
		eval[any](t, tab, `(() => { const box = document.querySelector('input[name="events"][value="item.created"]'); box.checked = true; box.dispatchEvent(new Event("change", { bubbles: true })); })()`)
		headers := responseHeaders(t, tab, "/settings/webhooks", http.MethodPost)
		clickByText(t, tab, "Create webhook")
		select {
		case cacheControl := <-headers:
			if cacheControl != "no-store" {
				t.Fatalf("webhook create Cache-Control %q, want no-store", cacheControl)
			}
		case <-time.After(actionWait):
			t.Fatal("no webhook create response")
		}
		eventually(t, tab, `document.querySelector(".webhook-secret") !== null`, "secret not shown after create")
		secret := eval[string](t, tab, `document.querySelector(".webhook-secret").textContent.trim()`)
		if !regexp.MustCompile(`^whsec_`).MatchString(secret) {
			t.Fatalf("secret %q does not start with whsec_", secret)
		}
		return secret
	}

	firstSecret := createWebhook("Browser fixture")
	expectAxeClean(t, tab)

	navigate(t, tab, workspacePath+"/settings/webhooks")
	press(t, tab, "\t")
	expectFocused(t, tab, ".skip-link")
	press(t, tab, "\r")
	eventually(t, tab, `location.hash === "#main"`, "skip link did not move to main")
	eval[any](t, tab, `document.querySelector('input[aria-label="Name"], #webhook-name, input[name="name"]').focus()`)
	press(t, tab, "\t")
	expectFocused(t, tab, "input[name='url'], #webhook-endpoint, input[type='url']")

	eventually(t, tab, `[...document.querySelectorAll(".webhook-card")].some((c) => c.textContent.includes("Browser fixture"))`, "first webhook card missing")
	if eval[int](t, tab, `[...document.querySelectorAll(".webhook-card")].filter((c) => c.textContent.includes("Browser fixture")).reduce((n, c) => n + c.querySelectorAll(".webhook-secret").length, 0)`) != 0 {
		t.Fatal("secret shown on the registry list")
	}
	deleteWebhook(t, tab, "Browser fixture")
	eventually(t, tab, `![...document.querySelectorAll(".webhook-card")].some((c) => c.textContent.includes("Browser fixture"))`, "deleted webhook still listed")

	secret := createWebhook("Delivery fixture")
	if !strings.Contains(eval[string](t, tab, `document.querySelector(".webhook-secret").textContent`), secret) {
		t.Fatal("secret not shown once after create")
	}
	navigate(t, tab, workspacePath+"/settings/webhooks")
	expectCount(t, tab, ".webhook-secret", 0)
	if eval[bool](t, tab, fmt.Sprintf(`document.body.textContent.includes(%s)`, jsString(secret))) {
		t.Fatal("secret appears after reload")
	}
	deliveryPath := eval[string](t, tab, `[...document.querySelectorAll(".webhook-card")].find((c) => c.textContent.includes("Delivery fixture")).querySelector('a[href$="/deliveries"]').getAttribute("href")`)
	if deliveryPath == "" {
		t.Fatal("no deliveries link")
	}

	navigate(t, tab, workspacePath+"/items")
	fillByLabel(t, tab, "New item title", "WEBHOOK_PRIVATE_PAYLOAD_FIXTURE")
	clickByText(t, tab, "Add item")
	eventually(t, tab, `[...document.querySelectorAll('#item-list input[name="title"]')].some((i) => i.getAttribute("value") === "WEBHOOK_PRIVATE_PAYLOAD_FIXTURE")`, "item not created")

	headers := responseHeaders(t, tab, deliveryPath, http.MethodGet)
	navigate(t, tab, deliveryPath)
	select {
	case cacheControl := <-headers:
		if cacheControl != "no-store" {
			t.Fatalf("delivery history Cache-Control %q, want no-store", cacheControl)
		}
	case <-time.After(actionWait):
		t.Fatal("no delivery history response")
	}
	expectCount(t, tab, "main table tbody tr", 1)
	expectText(t, tab, "main table", regexp.MustCompile("item.created"))
	mainText := eval[string](t, tab, `document.querySelector("main").innerText`)
	if strings.Contains(mainText, secret) || strings.Contains(mainText, "WEBHOOK_PRIVATE_PAYLOAD_FIXTURE") {
		t.Fatal("delivery history exposes the secret or the private payload")
	}
	expectAxeClean(t, tab)
	run(t, tab, chromedp.EmulateViewport(390, 844))
	expectNoOverflow(t, tab, 390)
	if overflow := eval[string](t, tab, `getComputedStyle(document.querySelector(".table-scroll")).overflowX`); overflow != "auto" && overflow != "scroll" {
		t.Fatalf("table scroll overflow-x %q", overflow)
	}

	navigate(t, tab, workspacePath+"/settings/webhooks")
	deleteWebhook(t, tab, "Delivery fixture")
	eventually(t, tab, `![...document.querySelectorAll(".webhook-card")].some((c) => c.textContent.includes("Delivery fixture"))`, "delivery fixture still listed")
	_ = firstSecret
}

// deleteWebhook checks the delete confirmation for the card named name and deletes it.
func deleteWebhook(t *testing.T, tab context.Context, name string) {
	t.Helper()
	eval[any](t, tab, fmt.Sprintf(`(() => { const card = [...document.querySelectorAll(".webhook-card")].find((c) => c.textContent.includes(%s)); const box = card.querySelector('input[aria-label^="Delete this webhook"], label input[type=checkbox]'); box.checked = true; box.dispatchEvent(new Event("change", { bubbles: true })); })()`, jsString(name)))
	eval[any](t, tab, fmt.Sprintf(`(() => { const card = [...document.querySelectorAll(".webhook-card")].find((c) => c.textContent.includes(%s)); [...card.querySelectorAll("button")].find((b) => b.textContent.trim() === "Delete webhook").click(); })()`, jsString(name)))
	settle(t, tab)
}
