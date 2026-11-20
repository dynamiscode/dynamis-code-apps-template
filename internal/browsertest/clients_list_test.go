//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// signIn signs the owner in on the tab and waits for the home page.
func signIn(t *testing.T, tab context.Context) {
	t.Helper()
	navigate(t, tab, "/login")
	fill(t, tab, "#email", fixtureOwnerEmail)
	fill(t, tab, "#password", fixturePassword)
	click(t, tab, `button[type="submit"]`)
	eventually(t, tab, fmt.Sprintf("location.href === %s", jsString(baseURL+"/")), "sign-in did not reach the home page")
}

// openClients signs in and opens the clients list of the first workspace.
func openClients(t *testing.T, tab context.Context) {
	t.Helper()
	signIn(t, tab)
	click(t, tab, ".cards a")
	waitVisible(t, tab, "[data-workspace-home]")
	click(t, tab, ".workspace-cards a[href$='/clients']")
	waitVisible(t, tab, "#client-list")
}

// clickByText clicks the first visible button, link, summary or submit whose text contains name.
func clickByText(t *testing.T, tab context.Context, name string) {
	t.Helper()
	found := eval[bool](t, tab, fmt.Sprintf(`(() => {
		const want = %s.toLowerCase();
		const element = [...document.querySelectorAll("button, a, summary, input[type=submit]")]
			.find((el) => el.getClientRects().length > 0 && (el.textContent || el.value || "").trim().toLowerCase().includes(want));
		if (!element) return false;
		window.__bt = 1;
		element.click();
		return true;
	})()`, jsString(name)))
	if !found {
		t.Fatalf("no visible control named %q", name)
	}
	settle(t, tab)
}

// labelledControl is the JavaScript that finds the control labelled by name.
func labelledControl(name string) string {
	return fmt.Sprintf(`(() => {
		const want = %s.toLowerCase();
		const label = [...document.querySelectorAll("label")].find((l) => l.textContent.trim().toLowerCase().includes(want));
		return label && (label.htmlFor ? document.getElementById(label.htmlFor) : label.querySelector("input, select, textarea"));
	})()`, jsString(name))
}

// fillByLabel sets the control labelled name and fires the input and change events.
func fillByLabel(t *testing.T, tab context.Context, name, value string) {
	t.Helper()
	eventually(t, tab, fmt.Sprintf("(() => { return Boolean(%s); })()", labelledControl(name)), fmt.Sprintf("no control labelled %q yet", name))
	if !eval[bool](t, tab, fmt.Sprintf(`(() => { const control = %s; if (!control) return false;
		control.value = %s;
		control.dispatchEvent(new Event("input", { bubbles: true }));
		control.dispatchEvent(new Event("change", { bubbles: true }));
		return true; })()`, labelledControl(name), jsString(value))) {
		t.Fatalf("no control labelled %q", name)
	}
}

func expectLabelValue(t *testing.T, tab context.Context, name, want string) {
	t.Helper()
	eventually(t, tab, fmt.Sprintf(`(() => { const control = %s; return Boolean(control) && control.value === %s; })()`,
		labelledControl(name), jsString(want)), fmt.Sprintf("label %q does not hold %q", name, want))
}

func expectRealtime(t *testing.T, tab context.Context, pattern string) {
	t.Helper()
	eventually(t, tab, fmt.Sprintf(`(() => { const el = document.querySelector("#realtime-status"); return Boolean(el) && new RegExp(%s).test(el.textContent); })()`, jsString(pattern)),
		"realtime status does not match "+pattern)
}

// setAccountTheme saves the owner's theme through the account profile endpoint, as the
// Playwright helper did, and returns a restore function.
func setAccountTheme(t *testing.T, tab context.Context, theme string) func() {
	t.Helper()
	navigate(t, tab, "/account")
	previous := eval[string](t, tab, `document.querySelector('form[action="/account/profile"] [name="theme"]').value`)
	save := func(value string) {
		status := eval[string](t, tab, fmt.Sprintf(`(async () => {
			const form = document.querySelector('form[action="/account/profile"]');
			const body = new URLSearchParams();
			for (const name of ["csrf", "timezone", "locale", "display_name"]) body.set(name, form.querySelector('[name="' + name + '"]').value);
			body.set("theme", %s);
			const response = await fetch("/account/profile", { method: "POST", body, redirect: "manual", credentials: "same-origin" });
			return response.type === "opaqueredirect" ? "303" : String(response.status);
		})()`, jsString(value)), awaitPromise)
		if status != "303" {
			t.Fatalf("account theme update returned %s", status)
		}
	}
	save(theme)
	return func() { navigate(t, tab, "/account"); save(previous) }
}

// acceptDialogs accepts every JavaScript dialog the tab opens, as page.once("dialog") did.
func acceptDialogs(tab context.Context) {
	_ = chromedp.Listen(tab, func(ev any) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			go func() {
				_, _ = chromedp.Run[cdp.Empty](tab, func(ctx context.Context, target *chromedp.Target) (cdp.Empty, error) {
					return cdp.Call(ctx, target, page.HandleJavaScriptDialog, page.HandleJavaScriptDialogParams{Accept: true})
				})
			}()
		}
	})
}

// TestClientDetailLifecycle ports the detail editing, custom value and lifecycle test.
func TestClientDetailLifecycle(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	name := fmt.Sprintf("Lifecycle %d", time.Now().UnixMilli())
	click(t, tab, "#client-create > details.client-create > summary")
	fillByLabel(t, tab, "Display name", name)
	clickByText(t, tab, "Add client")
	eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "create did not open the client detail")
	detailPath := eval[string](t, tab, "location.pathname")

	fillByLabel(t, tab, "Display name", name+" updated")
	clickByText(t, tab, "Save client")
	eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll("h1, h2")].some((h) => h.textContent.trim() === %s)`, jsString(name+" updated")), "updated heading not shown")

	clickByText(t, tab, "Manage custom fields")
	waitVisible(t, tab, ".client-create > summary")
	click(t, tab, ".client-create > summary")
	fillByLabel(t, tab, "Field label", "Lifecycle note")
	clickByText(t, tab, "Add field")
	navigate(t, tab, detailPath)
	fillByLabel(t, tab, "Lifecycle note (optional)", "Follow up")
	clickByText(t, tab, "Save value")
	expectLabelValue(t, tab, "Lifecycle note (optional)", "Follow up")
	clickByText(t, tab, "Clear value")
	eventually(t, tab, `document.querySelectorAll(".client-value-clear").length === 0`, "cleared value control still shown")

	clickByText(t, tab, "Archive")
	eventually(t, tab, `document.body.textContent.includes("This client is archived. It is hidden from the default list.")`, "archived notice not shown")
	clickByText(t, tab, "Unarchive")
	eventually(t, tab, `[...document.querySelectorAll("button")].some((b) => b.textContent.trim() === "Archive")`, "archive control not restored")

	acceptDialogs(tab)
	clickByText(t, tab, "Delete permanently")
	eventually(t, tab, `/\/clients$/.test(location.pathname)`, "delete did not return to the clients list")
	if count := eval[int](t, tab, fmt.Sprintf(`[...document.querySelectorAll("#client-list *")].filter((el) => el.childElementCount === 0 && el.textContent.trim() === %s).length`, jsString(name+" updated"))); count != 0 {
		t.Fatalf("deleted client still listed %d times", count)
	}
}

// TestClientsListAdaptsAcrossViewports ports the layout, theme and live-status checks.
// Screenshot comparisons are not ported here; see the consolidation PR body.
func TestClientsListAdaptsAcrossViewports(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	clientsPath := eval[string](t, tab, "location.pathname + location.search")
	for _, colorScheme := range []string{"light", "dark"} {
		t.Run(colorScheme, func(t *testing.T) {
			restore := setAccountTheme(t, tab, colorScheme)
			t.Cleanup(restore)
			navigate(t, tab, clientsPath)
			waitVisible(t, tab, "#client-list")
			expectAttribute(t, tab, "html", "data-theme", colorScheme)
			transition := eval[string](t, tab, `(() => {
				const wrapper = document.createElement("div");
				wrapper.className = "cards";
				const link = document.createElement("a");
				wrapper.append(link);
				document.body.append(wrapper);
				const duration = getComputedStyle(link).transitionDuration;
				wrapper.remove();
				return duration;
			})()`)
			for _, duration := range strings.Split(transition, ",") {
				if strings.TrimSpace(duration) != "0s" {
					t.Fatalf("transition duration %q, want 0s", transition)
				}
			}
			for _, width := range []int64{1280, 390} {
				run(t, tab, chromedp.EmulateViewport(width, 900))
				mobile := width <= 608
				layout := eval[struct {
					Table    string `json:"table"`
					Cards    string `json:"cards"`
					Canvas   string `json:"canvas"`
					Overflow bool   `json:"overflow"`
				}](t, tab, `({
					table: getComputedStyle(document.querySelector(".list-view-table")).display,
					cards: getComputedStyle(document.querySelector(".list-view-cards")).display,
					canvas: getComputedStyle(document.documentElement).getPropertyValue("--canvas").trim(),
					overflow: document.documentElement.scrollWidth > document.documentElement.clientWidth,
				})`)
				wantTable, wantCards := "block", "none"
				if mobile {
					wantTable, wantCards = "none", "grid"
				}
				if layout.Table != wantTable || layout.Cards != wantCards {
					t.Fatalf("width %d: table %s cards %s, want %s and %s", width, layout.Table, layout.Cards, wantTable, wantCards)
				}
				wantCanvas := "#f3f6fa"
				if colorScheme == "dark" {
					wantCanvas = "#111826"
				}
				if layout.Canvas != wantCanvas {
					t.Fatalf("canvas %q, want %q", layout.Canvas, wantCanvas)
				}
				if layout.Overflow {
					t.Fatalf("width %d overflows horizontally", width)
				}
				expectRealtime(t, tab, "Live updates connected")
			}
		})
	}
}

// TestClientsListKeepsServerDraftDuringRefresh ports the 422 draft and SSE refresh test.
func TestClientsListKeepsServerDraftDuringRefresh(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	waitVisible(t, tab, "#client-list")
	expectAxeClean(t, tab)

	click(t, tab, "#client-create > details.client-create > summary")
	expectRealtime(t, tab, "connected|connecting")
	fillByLabel(t, tab, "Display name", "Server draft survives refresh")
	fillByLabel(t, tab, "Email", "Ada <ada@example.com>")

	statuses := make(chan int64, 4)
	call(t, tab, network.Enable, network.EnableParams{})
	_ = chromedp.Listen(tab, func(ev any) {
		if response, ok := ev.(*network.EventResponseReceived); ok && response.Type == network.ResourceTypeDocument &&
			strings.HasSuffix(response.Response.URL, "/clients") {
			select {
			case statuses <- response.Response.Status:
			default:
			}
		}
	})
	eval[any](t, tab, `(() => HTMLFormElement.prototype.submit.call(document.querySelector("#client-create form.create")))()`)
	select {
	case status := <-statuses:
		if status != 422 {
			t.Fatalf("invalid POST status %d, want 422", status)
		}
	case <-time.After(actionWait):
		t.Fatal("no response to the invalid POST")
	}
	waitVisible(t, tab, "[role=alert]")
	expectLabelValue(t, tab, "Email", "Ada <ada@example.com>")
	expectAttribute(t, tab, "#client-create form.create", "data-dirty", "true")
	expectLabelValue(t, tab, "Display name", "Server draft survives refresh")

	peer := newPeer(t, tab)
	navigate(t, peer, eval[string](t, tab, "location.pathname + location.search"))
	click(t, peer, "#client-create > details.client-create > summary")
	fillByLabel(t, peer, "Display name", "SSE refresh trigger")
	clickByText(t, peer, "Add client")
	eventually(t, peer, `/\/clients\/[^/?]+$/.test(location.pathname)`, "peer create did not open the client detail")
	expectRealtime(t, tab, "New changes are waiting")
	expectLabelValue(t, tab, "Display name", "Server draft survives refresh")
}

// TestClientsListFollowsSpanishLocale ports the Spanish workspace locale test.
func TestClientsListFollowsSpanishLocale(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	here := eval[string](t, tab, "location.pathname + location.search")
	navigate(t, tab, "/language?locale=es&return_to="+url.QueryEscape(here))
	expectAttribute(t, tab, "html", "lang", "es")
	eventually(t, tab, `[...document.querySelectorAll("h1")].some((h) => h.textContent.trim() === "Clientes")`, "Spanish heading not shown")
	connected := eval[string](t, tab, `document.querySelector("#client-list")?.getAttribute("data-live-connected") ?? ""`)
	if connected == "" {
		t.Fatal("client list has no data-live-connected state")
	}
	eventually(t, tab, fmt.Sprintf(`document.querySelector("#realtime-status")?.textContent.trim() === %s`, jsString(connected)), "realtime status does not match the list state")
	click(t, tab, "#client-create > details.client-create > summary")
	eventually(t, tab, `Boolean(document.querySelector("#client-display-name"))`, "Spanish create form not shown")
	expectLabelValue(t, tab, "Nombre para mostrar", "")
}

// TestClientDetailAndFieldsResponsiveLayouts ports the accessible responsive layout test.
func TestClientDetailAndFieldsResponsiveLayouts(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	click(t, tab, "#client-create > details.client-create > summary")
	fillByLabel(t, tab, "Display name", fmt.Sprintf("Accessibility client %d", time.Now().UnixMilli()))
	clickByText(t, tab, "Add client")
	eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "create did not open the client detail")
	detailPath := eval[string](t, tab, "location.pathname")
	fieldsPath := regexp.MustCompile(`/clients/[^/]+$`).ReplaceAllString(detailPath, "/client-fields")
	for _, path := range []string{detailPath, fieldsPath} {
		navigate(t, tab, path)
		run(t, tab, chromedp.EmulateViewport(390, 900))
		if eval[bool](t, tab, `document.documentElement.scrollWidth > document.documentElement.clientWidth`) {
			t.Fatalf("%s overflows at 390px", path)
		}
		expectAxeClean(t, tab)
	}
}

// TestClientsListQueryOpenDoesNotBlockRealtime ports the query-open test onto the shipped
// add-filter disclosure. An open filter query must not stop the live refresh.
func TestClientsListQueryOpenDoesNotBlockRealtime(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	here := eval[string](t, tab, "location.pathname")
	navigate(t, tab, here+"?filter_field=displayName")
	eventually(t, tab, `Boolean(document.querySelector(".list-add-menu[open]"))`, "add-filter disclosure is not open for a query")
	expectRealtime(t, tab, "connected|connecting")

	peer := newPeer(t, tab)
	navigate(t, peer, here+"?filter_field=displayName")
	// Open the create menu only if it is closed: a click on an open summary would close it.
	eval[any](t, peer, `(() => { const menu = document.querySelector("#client-create > details.client-create"); if (!menu.open) menu.querySelector("summary").click(); })()`)
	name := fmt.Sprintf("Query disclosure refresh trigger %d", time.Now().UnixMilli())
	fillByLabel(t, peer, "Display name", name)
	clickByText(t, peer, "Add client")
	eventually(t, peer, `/\/clients\/[^/?]+$/.test(location.pathname)`, "peer create did not open the client detail")
	eventually(t, tab, fmt.Sprintf(`document.querySelector("#client-list tbody")?.textContent.includes(%s)`, jsString(name)), "open query disclosure blocked the realtime refresh")
	connected := eval[string](t, tab, `document.querySelector("#client-list")?.getAttribute("data-live-connected") ?? ""`)
	if connected == "" {
		t.Fatal("client list has no data-live-connected state")
	}
	eventually(t, tab, fmt.Sprintf(`document.querySelector("#realtime-status")?.textContent.trim() === %s`, jsString(connected)), "realtime status does not match the list state")
}

// TestClientFieldLifecycleSurvivesRefresh keeps the custom-field lifecycle that shared the
// deleted multi-select test. The multi-select filter itself is not part of the shipped design.
func TestClientFieldLifecycleSurvivesRefresh(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	clickByText(t, tab, "Manage custom fields")
	click(t, tab, ".client-create > summary")
	fillByLabel(t, tab, "Field label", "Browser multi-select")
	eval[any](t, tab, `(() => { const select = document.querySelector("#client-field-type"); select.value = "single_choice"; select.dispatchEvent(new Event("change", { bubbles: true })); })()`)
	clickByText(t, tab, "Add field")
	for _, label := range []string{"First", "Second", "Third"} {
		eval[any](t, tab, fmt.Sprintf(`(() => {
			const card = [...document.querySelectorAll(".client-field-card")].find((c) => c.textContent.includes("Browser multi-select"));
			const form = card.querySelector(".option-create");
			form.querySelector('input[name="label"]').value = %s;
			[...form.querySelectorAll("button")].find((b) => b.textContent.includes("Add option")).click();
		})()`, jsString(label)))
		eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll(".client-field-option-list li")].some((li) => li.textContent.includes(%s))`, jsString(label)), "option "+label+" not added")
	}
	clickByText(t, tab, "Back to clients")
	eventually(t, tab, `/\/clients$/.test(location.pathname)`, "back link did not return to clients")
}

// newPeer opens a second tab in the same browser, so it shares the session. Its root
// context starts the tab before any per-action timeout is derived from it.
func newPeer(t *testing.T, tab context.Context) context.Context {
	t.Helper()
	peer, cancel := chromedp.NewContext(tab)
	t.Cleanup(cancel)
	if err := chromedp.Do(peer); err != nil {
		t.Fatalf("start peer tab: %v", err)
	}
	return peer
}
