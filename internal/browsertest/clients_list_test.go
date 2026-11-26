//go:build browser

package browsertest

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"math"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// signIn signs the owner in on the tab and waits for the home page.
func signIn(t *testing.T, tab context.Context) {
	t.Helper()
	navigate(t, tab, "/login")
	fill(t, tab, "#email", fixtureOwnerEmail)
	fill(t, tab, "#password", fixturePassword)
	click(t, tab, `button[type="submit"]`)
	eventually(t, tab, `location.pathname === "/" || /^\/workspaces\/[^/]+$/.test(location.pathname)`, "sign-in did not reach a workspace")
}

// openClients signs in and opens the clients list of the first workspace.
func openClients(t *testing.T, tab context.Context) {
	t.Helper()
	signIn(t, tab)
	if eval[bool](t, tab, `location.pathname === "/"`) {
		click(t, tab, ".cards a")
	}
	waitVisible(t, tab, "[data-workspace-home]")
	click(t, tab, ".workspace-cards a[href$='/clients']")
	waitVisible(t, tab, "#client-list")
}

// ensureClient gives list-layout tests a row even in a freshly bootstrapped workspace.
func ensureClient(t *testing.T, tab context.Context) {
	t.Helper()
	path := eval[string](t, tab, "location.pathname")
	if eval[bool](t, tab, `Boolean(document.querySelector(".list-view-table tbody tr"))`) {
		return
	}
	navigate(t, tab, path+"/new")
	fillByLabel(t, tab, "Display name", "Browser list fixture")
	clickByText(t, tab, "Create client")
	eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "fixture creation did not open client detail")
	navigate(t, tab, path)
	waitVisible(t, tab, ".list-view-table tbody tr")
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

func TestDropdownActionsUseUnboxedTiles(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	ensureClient(t, tab)
	click(t, tab, `.list-toolbar > details:has(.list-views-panel) > summary`)
	eventually(t, tab, `Boolean(document.querySelector('.list-views-panel .list-view-option'))`, "views menu did not open")
	if !eval[bool](t, tab, `(() => {
		const item = document.querySelector('.list-views-panel .list-view-option');
		const style = getComputedStyle(item);
		return parseFloat(style.borderWidth) === 0 && item.getBoundingClientRect().height >= 44;
	})()`) {
		t.Fatal("view options must be unboxed touch-sized tiles")
	}
	click(t, tab, `a[href$="/engagements"]`)
	waitVisible(t, tab, `#engagement-view .list-create details > summary`)
	click(t, tab, `#engagement-view .list-create details > summary`)
	if !eval[bool](t, tab, `(() => {
		const items = [...document.querySelectorAll('#engagement-view .list-create .dropdown-content .menu > li > a')];
		return items.length >= 2 && items.every(item => {
			const style = getComputedStyle(item);
			return parseFloat(style.borderWidth) === 0 && item.getBoundingClientRect().height >= 44;
		});
	})()`) {
		t.Fatal("engagement settings actions must be unboxed touch-sized tiles")
	}
}

func TestClientFilterTriggerMatchesToolbarControls(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	ensureClient(t, tab)
	waitVisible(t, tab, "#list-filter-add summary")
	if !eval[bool](t, tab, `(() => {
		const filter = document.querySelector('#list-filter-add summary');
		const columns = document.querySelector('.list-toolbar > details:has(.list-columns-panel) > summary');
		const a = getComputedStyle(filter), b = getComputedStyle(columns);
		return a.height === b.height && a.borderWidth === b.borderWidth &&
			a.borderRadius === b.borderRadius && a.backgroundColor === b.backgroundColor &&
			a.fontSize === b.fontSize && getComputedStyle(filter, '::after').content === getComputedStyle(columns, '::after').content;
	})()`) {
		t.Fatal("filter trigger must match the columns control")
	}
	run(t, tab, chromedp.Focus("#list-filter-add summary"), chromedp.KeyEvent("\r"))
	eventually(t, tab, `document.querySelector('#list-filter-add details').open`, "keyboard did not open filter")
}

func TestClientSelectionAndNameHoverIndependently(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	ensureClient(t, tab)
	waitVisible(t, tab, ".list-view-table tbody .list-select-cell")
	moveTo := func(selector string) {
		t.Helper()
		point := eval[[]float64](t, tab, fmt.Sprintf(`(() => {
			const r = document.querySelector(%s).getBoundingClientRect();
			return [r.left + r.width / 2, r.top + r.height / 2];
		})()`, jsString(selector)))
		call(t, tab, input.DispatchMouseEvent, input.DispatchMouseEventParams{Type: input.DispatchMouseEventTypeMouseMoved, X: point[0], Y: point[1]})
	}
	moveTo(".list-view-table tbody .list-linked-cell")
	if !eval[bool](t, tab, `(() => {
		const row = document.querySelector('.list-view-table tbody tr');
		const name = row.querySelector('.list-linked-cell');
		const select = row.querySelector('.list-select-cell');
		const initials = name.querySelector('.list-initials');
		return getComputedStyle(name).backgroundColor !== getComputedStyle(select).backgroundColor &&
			getComputedStyle(initials).backgroundColor !== getComputedStyle(name).backgroundColor;
	})()`) {
		t.Fatal("name hover must not highlight selection or hide the initials")
	}
	moveTo(".list-view-table tbody .list-select-cell")
	if !eval[bool](t, tab, `(() => {
		const row = document.querySelector('.list-view-table tbody tr');
		return getComputedStyle(row.querySelector('.list-select-cell')).backgroundColor !==
			getComputedStyle(row.querySelector('.list-linked-cell')).backgroundColor;
	})()`) {
		t.Fatal("selection cell must highlight independently")
	}
}

func TestClientRowActionsCoverCellAndOverlayRows(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	ensureClient(t, tab)
	waitVisible(t, tab, ".list-view-table tbody .list-actions-cell summary")
	if !eval[bool](t, tab, `(() => {
		const cell = document.querySelector('.list-view-table tbody .list-actions-cell');
		const summary = cell.querySelector('summary');
		const a = cell.getBoundingClientRect(), b = summary.getBoundingClientRect();
		return Math.abs(a.left-b.left)<1 && Math.abs(a.right-b.right)<1 &&
			Math.abs(a.top-b.top)<1 && Math.abs(a.bottom-b.bottom)<1 &&
			parseFloat(getComputedStyle(summary).borderWidth) === 0;
	})()`) {
		t.Fatal("row actions trigger must cover the cell without a border")
	}
	idleBackground := eval[string](t, tab, `getComputedStyle(document.querySelector('.list-view-table tbody .list-actions-cell')).backgroundColor`)
	run(t, tab, chromedp.Focus(".list-view-table tbody .list-actions-cell summary"), chromedp.KeyEvent("\r"))
	eventually(t, tab, `document.querySelector('.list-view-table tbody .list-actions-cell details').open`, "keyboard did not open row actions")
	if !eval[bool](t, tab, fmt.Sprintf(`(() => {
		const panel = document.querySelector('.list-view-table tbody .list-actions-cell details[open] .dropdown-content');
		const item = panel.querySelector('.list-action-item');
		const r = item.getBoundingClientRect();
		const hit = document.elementFromPoint(r.left+r.width/2, r.top+r.height/2);
		return panel.contains(hit) && parseFloat(getComputedStyle(item).borderWidth) === 0 &&
			getComputedStyle(panel).backgroundColor === %s;
	})()`, jsString(idleBackground))) {
		details := eval[string](t, tab, `(() => {
			const cell = document.querySelector('.list-view-table tbody .list-actions-cell');
			const panel = cell.querySelector('details[open] .dropdown-content');
			const item = panel.querySelector('.list-action-item');
			const r = item.getBoundingClientRect();
			const hit = document.elementFromPoint(r.left+r.width/2, r.top+r.height/2);
			return JSON.stringify({panel: panel.getBoundingClientRect().toJSON(), item: r.toJSON(),
				hit: hit?.outerHTML, border: getComputedStyle(item).borderWidth,
				panelBackground: getComputedStyle(panel).backgroundColor, cellBackground: getComputedStyle(cell).backgroundColor});
		})()`)
		t.Fatalf("row actions must overlay rows with an unboxed item and row-matching surface (idle %s): %s", idleBackground, details)
	}
}

func TestClientsSortableHeaderCoversCell(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	ensureClient(t, tab)
	header := `.list-view-table th[data-column="displayName"]`
	waitVisible(t, tab, header+` > a`)
	if !eval[bool](t, tab, `(() => {
		const th = document.querySelector('.list-view-table th[data-column="displayName"]');
		const link = th.querySelector('a');
		const cell = th.getBoundingClientRect(), target = link.getBoundingClientRect();
		const style = getComputedStyle(link), icon = getComputedStyle(link, '::after');
		return Math.abs(cell.left - target.left) < 1 && Math.abs(cell.right - target.right) < 1 &&
			Math.abs(cell.top - target.top) < 1 && Math.abs(cell.bottom - target.bottom) <= 1 &&
			parseFloat(style.borderWidth) === 0 && icon.content !== 'none' &&
			style.justifyContent === 'space-between';
	})()`) {
		details := eval[string](t, tab, `(() => {
			const cell = document.querySelector('.list-view-table th[data-column="displayName"]');
			const link = cell.querySelector('a');
			const style = getComputedStyle(link);
			return JSON.stringify({cell: cell.getBoundingClientRect().toJSON(), link: link.getBoundingClientRect().toJSON(),
				border: style.borderWidth, indicator: getComputedStyle(link, '::after').content, justify: style.justifyContent});
		})()`)
		t.Fatalf("sort link must cover its header cell without a border, with the indicator on the right: %s", details)
	}
	run(t, tab, chromedp.Focus(header+` > a`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `document.querySelector('.list-view-table th[data-column="displayName"]').getAttribute('aria-sort') === 'ascending'`, "keyboard sorting did not activate ascending order")
	run(t, tab, chromedp.Focus(header+` > a`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `document.querySelector('.list-view-table th[data-column="displayName"]').getAttribute('aria-sort') === 'descending'`, "keyboard sorting did not activate descending order")
}

func TestClientTimelineDialog(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	click(t, tab, `a[href$="/clients/new"]`)
	fillByLabel(t, tab, "Display name", fmt.Sprintf("Timeline %d", time.Now().UnixMilli()))
	clickByText(t, tab, "Create client")
	waitVisible(t, tab, "[data-timeline-open]")
	click(t, tab, "[data-timeline-open]")
	eventually(t, tab, `document.querySelector('[data-timeline-dialog]').matches(':modal')`, "timeline dialog did not open")
	click(t, tab, "#timeline-text")
	call(t, tab, input.InsertText, input.InsertTextParams{Text: strings.Repeat("x", 2001)})
	click(t, tab, `[data-timeline-dialog] button[type="submit"]`)
	eventually(t, tab, `document.querySelector('[data-timeline-dialog]').matches(':modal') && document.querySelector('#timeline-text').value.length === 2001 && document.querySelector('[data-timeline-composer] [role=alert]') !== null`, "rejected draft did not reopen with error")
	click(t, tab, "[data-timeline-close]")
	eventually(t, tab, `!document.querySelector('[data-timeline-dialog]').open && document.activeElement.matches('[data-timeline-open]')`, "rejected dialog close did not restore focus")
	click(t, tab, "[data-timeline-open]")
	fill(t, tab, "#timeline-text", "Followed up by phone")
	click(t, tab, `[data-timeline-dialog] button[type="submit"]`)
	eventually(t, tab, `document.querySelector('.timeline').textContent.includes('Followed up by phone')`, "saved timeline entry missing")
	click(t, tab, "[data-timeline-open]")
	click(t, tab, "[data-timeline-close]")
	eventually(t, tab, `!document.querySelector('[data-timeline-dialog]').open && document.activeElement.matches('[data-timeline-open]')`, "dialog close did not restore focus")
	path := eval[string](t, tab, `location.pathname`)
	call(t, tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	navigate(t, tab, path)
	waitVisible(t, tab, "#timeline-text")
	click(t, tab, "#timeline-text")
	call(t, tab, input.InsertText, input.InsertTextParams{Text: strings.Repeat("x", 2001)})
	click(t, tab, `[data-timeline-composer] button[type="submit"]`)
	eventually(t, tab, `document.querySelector('[data-timeline-error-summary] [role=alert]') && document.querySelector('[data-timeline-error-summary] a').getAttribute('href') === '#timeline-text' && document.querySelector('#timeline-text').value.length === 2001`, "no-JavaScript error summary or draft missing")
	fill(t, tab, "#timeline-text", "No JavaScript entry")
	click(t, tab, `[data-timeline-composer] button[type="submit"]`)
	eventually(t, tab, `document.querySelector('.timeline').textContent.includes('No JavaScript entry')`, "no-JavaScript submission failed")
}

// TestClientDetailLifecycle ports the detail editing, custom value and lifecycle test.
func TestClientDetailLifecycle(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	name := fmt.Sprintf("Lifecycle %d", time.Now().UnixMilli())
	click(t, tab, `a[href$="/clients/new"]`)
	fillByLabel(t, tab, "Display name", name)
	clickByText(t, tab, "Create client")
	eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "create did not open the client detail")
	detailPath := eval[string](t, tab, "location.pathname")

	click(t, tab, `#client-detail details:has(#inline-display_name) > summary`)
	fill(t, tab, "#inline-display_name", name+" updated")
	click(t, tab, `#inline-display_name ~ div button[aria-label="Save value"]`)
	eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll("h1, h2")].some((h) => h.textContent.trim() === %s)`, jsString(name+" updated")), "updated heading not shown")

	click(t, tab, `summary[aria-label="Row actions"]`)
	clickByText(t, tab, "Manage custom fields")
	waitVisible(t, tab, ".client-create > summary")
	click(t, tab, ".client-create > summary")
	fillByLabel(t, tab, "Field label", "Lifecycle note")
	clickByText(t, tab, "Add field")
	navigate(t, tab, detailPath)
	eval[any](t, tab, `(() => { const label = [...document.querySelectorAll("#client-values dt")].find(el => el.textContent.trim() === "Lifecycle note"); label.nextElementSibling.querySelector("details > summary").click(); })()`)
	fillByLabel(t, tab, "Lifecycle note (optional)", "Follow up")
	click(t, tab, `#client-values details[open] button[aria-label="Save value"]`)
	eventually(t, tab, `([...document.querySelectorAll("#client-values dt")].find(el => el.textContent.trim() === "Lifecycle note")?.nextElementSibling.textContent || "").includes("Follow up")`, "saved custom value not shown")
	eval[any](t, tab, `(() => { const label = [...document.querySelectorAll("#client-values dt")].find(el => el.textContent.trim() === "Lifecycle note"); label.nextElementSibling.querySelector("details > summary").click(); })()`)
	click(t, tab, `#client-values details[open] button[aria-label="Clear value"]`)
	eventually(t, tab, fmt.Sprintf(`!%s.closest("form").querySelector('button[aria-label="Clear value"]')`, labelledControl("Lifecycle note (optional)")), "cleared value control still shown")

	click(t, tab, `summary[aria-label="Row actions"]`)
	clickByText(t, tab, "Archive")
	eventually(t, tab, `document.body.textContent.includes("This client is archived. It is hidden from the default list.")`, "archived notice not shown")
	click(t, tab, `summary[aria-label="Row actions"]`)
	clickByText(t, tab, "Restore")
	eventually(t, tab, `document.querySelector('summary[aria-label="Row actions"]') !== null && document.body.textContent.includes("Archive")`, "archive control not restored")

	click(t, tab, `summary[aria-label="Row actions"]`)
	clickByText(t, tab, "Delete permanently")
	waitVisible(t, tab, "[data-client-delete-dialog]:modal")
	click(t, tab, "[data-client-delete-confirm]")
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
	click(t, tab, `a[href$="/clients/new"]`)
	fillByLabel(t, tab, "Display name", "Responsive client")
	clickByText(t, tab, "Create client")
	eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "create did not open the client detail")
	for _, colorScheme := range []string{"light", "dark"} {
		t.Run(colorScheme, func(t *testing.T) {
			restore := setAccountTheme(t, tab, colorScheme)
			t.Cleanup(restore)
			navigate(t, tab, clientsPath)
			waitVisible(t, tab, "#client-list")
			expectTheme(t, tab, colorScheme)
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

// TestClientsListKeepsServerDraftDuringRefresh checks a rejected draft while another user creates a client.
func TestClientsListKeepsServerDraftDuringRefresh(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	waitVisible(t, tab, "#client-list")
	expectAxeClean(t, tab)

	listPath := eval[string](t, tab, "location.pathname")
	navigate(t, tab, listPath+"/new")
	fillByLabel(t, tab, "Display name", "Server draft survives refresh")
	fillByLabel(t, tab, "Email", "Ada <ada@example.com>")

	statuses := make(chan int64, 4)
	call(t, tab, network.Enable, network.EnableParams{})
	_ = chromedp.Listen(tab, func(ev any) {
		if response, ok := ev.(*network.EventResponseReceived); ok && response.Type == network.ResourceTypeDocument &&
			strings.HasSuffix(response.Response.URL, "/clients/new") {
			select {
			case statuses <- response.Response.Status:
			default:
			}
		}
	})
	eval[any](t, tab, `(() => HTMLFormElement.prototype.submit.call(document.querySelector("#client-new-page form.create")))()`)
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
	expectAttribute(t, tab, "#client-new-page form.create", "data-dirty", "true")
	expectLabelValue(t, tab, "Display name", "Server draft survives refresh")

	peer := newPeer(t, tab)
	navigate(t, peer, listPath)
	click(t, peer, `a[href$="/clients/new"]`)
	fillByLabel(t, peer, "Display name", "SSE refresh trigger")
	clickByText(t, peer, "Create client")
	eventually(t, peer, `/\/clients\/[^/?]+$/.test(location.pathname)`, "peer create did not open the client detail")
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
	click(t, tab, `a[href$="/clients/new"]`)
	eventually(t, tab, `Boolean(document.querySelector("#client-display-name"))`, "Spanish create form not shown")
	expectLabelValue(t, tab, "Nombre para mostrar", "")
}

// TestClientDetailAndFieldsResponsiveLayouts ports the accessible responsive layout test.
func TestClientDetailAndFieldsResponsiveLayouts(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	click(t, tab, `a[href$="/clients/new"]`)
	fillByLabel(t, tab, "Display name", fmt.Sprintf("Accessibility client %d", time.Now().UnixMilli()))
	clickByText(t, tab, "Create client")
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
	eventually(t, tab, `Boolean(document.querySelector(".list-menu[open]"))`, "add-filter disclosure is not open for a query")
	expectRealtime(t, tab, "connected|connecting")

	peer := newPeer(t, tab)
	navigate(t, peer, here+"?filter_field=displayName")
	// Create a client in another tab while the list filter is open.
	click(t, peer, `a[href$="/clients/new"]`)
	name := fmt.Sprintf("Query disclosure refresh trigger %d", time.Now().UnixMilli())
	fillByLabel(t, peer, "Display name", name)
	clickByText(t, peer, "Create client")
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
	clickByText(t, tab, "Manage")
	clickByText(t, tab, "Manage custom fields")
	click(t, tab, ".client-create > summary")
	fillByLabel(t, tab, "Field label", "Browser multi-select")
	eval[any](t, tab, `(() => { const select = document.querySelector("#client-field-type"); select.value = "single_choice"; select.dispatchEvent(new Event("change", { bubbles: true })); })()`)
	clickByText(t, tab, "Add field")
	for _, label := range []string{"First", "Second", "Third"} {
		inputID := eval[string](t, tab, `(() => {
			const card = [...document.querySelectorAll(".client-field-card")].find((c) => c.textContent.includes("Browser multi-select"));
			return card.querySelector('.option-create input[name="label"]').id;
		})()`)
		fill(t, tab, "#"+inputID, label)
		click(t, tab, fmt.Sprintf(`form:has(#%s) button[type="submit"]`, inputID))
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

func TestClientCreateEntryFieldKeepsDraftOnDeselect(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	path := eval[string](t, tab, "location.pathname")
	navigate(t, tab, path+"/new")
	waitVisible(t, tab, "#client-display-name")
	click(t, tab, "#client-display-name")
	fill(t, tab, "#client-display-name", "Draft client")
	click(t, tab, "#client-new-page")
	if got := eval[string](t, tab, `document.querySelector("#client-display-name").value`); got != "Draft client" {
		t.Fatalf("deselect lost draft: %q", got)
	}
	if !eval[bool](t, tab, `document.querySelector("#client-display-name").closest("[data-entry-field]") !== null`) {
		t.Fatal("client name does not use the shared entry field")
	}
}

func TestEntryLabelsAndNativeSelectFallback(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	clickByText(t, tab, "Manage")
	clickByText(t, tab, "Manage custom fields")
	click(t, tab, ".client-create > summary")
	click(t, tab, `label[for="client-field-label"]`)
	eventually(t, tab, `document.activeElement.id === "client-field-label"`, "entry label did not focus its input")
	click(t, tab, `label[for="client-field-type"]`)
	eventually(t, tab, `document.querySelector("#client-field-type").closest("[data-entry-field]").querySelector(".entry-picker-trigger").getAttribute("aria-expanded") === "true"`, "picker label did not open the picker")

	path := eval[string](t, tab, "location.pathname")
	call(t, tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	navigate(t, tab, path)
	click(t, tab, ".client-create > summary")
	if !eval[bool](t, tab, `(() => {
		const select = document.querySelector("#client-field-type");
		const style = getComputedStyle(select);
		return select.value === "text" && style.backgroundImage !== "none" &&
			style.borderTopColor === "rgba(0, 0, 0, 0)" && style.boxShadow === "none";
	})()`) {
		t.Fatal("native selected value must retain its arrow without a box")
	}
	eval[any](t, tab, `document.querySelector('label[for="client-field-type"]').scrollIntoView({block: "center"})`)
	state := eval[struct {
		Enhanced, Associated, Visible, LabelHit bool
		SelectCount                             int
		Hit                                     string
	}](t, tab, `(() => {
		const label = document.querySelector('label[for="client-field-type"]');
		const select = document.querySelector("#client-field-type");
		const r = label.getBoundingClientRect();
		const target = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
		return {enhanced: document.documentElement.classList.contains("js-enhanced"),
			selectCount: document.querySelectorAll("#client-field-type").length,
			associated: label.control === select, visible: select.getClientRects().length > 0,
			hit: target?.outerHTML?.slice(0, 300) || "", labelHit: target === label || label.contains(target)};
	})()`)
	if state.Enhanced || state.SelectCount != 1 || !state.Associated || !state.Visible || !state.LabelHit {
		t.Fatalf("native label preconditions: %+v", state)
	}
	click(t, tab, `label[for="client-field-type"]`)
	press(t, tab, kb.ArrowDown)
	press(t, tab, "\r")
	eventually(t, tab, `document.querySelector("#client-field-type").value === "number"`, "native select label did not allow keyboard selection")
}

func TestEntryLabelAndValueAlignment(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	path := eval[string](t, tab, "location.pathname")
	navigate(t, tab, path+"/new")
	waitVisible(t, tab, "#client-display-name")
	eventually(t, tab, `document.querySelector("#client-display-name").closest("[data-entry-field]").hasAttribute("data-entry-bound")`, "client entry field was not enhanced")

	measure := func(selector string) {
		t.Helper()
		position := eval[struct {
			Label, Value, Border, Padding float64
			Bound                         bool
		}](t, tab, fmt.Sprintf(`(() => {
			const control = document.querySelector(%s);
			const field = control.closest("[data-entry-field]");
			const label = field.querySelector("label");
			const visible = field.querySelector(".entry-picker-trigger:not([hidden])") || control;
			const range = document.createRange();
			range.selectNodeContents(label);
			const text = range.getBoundingClientRect();
			const rect = visible.getBoundingClientRect();
			const style = getComputedStyle(visible);
			const border = parseFloat(style.borderLeftWidth), padding = parseFloat(style.paddingLeft);
			return {label: text.left, value: rect.left + border + padding, border, padding, bound: field.hasAttribute("data-entry-bound")};
		})()`, jsString(selector)))
		if math.Abs(position.Label-position.Value) >= 0.1 {
			t.Fatalf("%s label and value text origins differ: %+v", selector, position)
		}
	}
	measure("#client-display-name")
	click(t, tab, `label[for="client-display-name"]`)
	measure("#client-display-name")
	fill(t, tab, "#client-display-name", "Display name")
	measure("#client-display-name")

	// Compare painted glyphs as well as layout coordinates, using the same text
	// before and after entry so font side bearings cannot conceal a shift.
	captureStart := func(selector string, label bool) int {
		t.Helper()
		bounds := eval[[]float64](t, tab, fmt.Sprintf(`(() => {
			const control = document.querySelector(%s);
			const label = control.closest("[data-entry-field]").querySelector("label");
			const r = (%t ? label : control).getBoundingClientRect();
			const controlRect = control.getBoundingClientRect();
			return [controlRect.left + 9, controlRect.left + 28, r.top + (%t ? 0 : 18), r.bottom - (%t ? 0 : 2)];
		})()`, jsString(selector), label, label, label))
		data, err := chromedp.Run(tab, chromedp.CaptureScreenshot())
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		x0, x1 := int(bounds[0]), int(bounds[1])
		y0, y1 := int(bounds[2]), int(bounds[3])
		br, bg, bb, _ := img.At(x0, y1-1).RGBA()
		for x := x0; x < x1; x++ {
			for y := y0; y < y1; y++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if math.Abs(float64(r)-float64(br))+math.Abs(float64(g)-float64(bg))+math.Abs(float64(b)-float64(bb)) > 12000 {
					return x
				}
			}
		}
		t.Fatalf("no painted text found for %s", selector)
		return 0
	}
	valueX := captureStart("#client-display-name", false)
	fill(t, tab, "#client-display-name", "")
	click(t, tab, "#client-new-page")
	labelX := captureStart("#client-display-name", true)
	if math.Abs(float64(labelX-valueX)) > 1 {
		t.Fatalf("placeholder text starts at %d; entered text starts at %d", labelX, valueX)
	}

	measure("#client-notes")
	click(t, tab, `label[for="client-notes"]`)
	measure("#client-notes")
	fill(t, tab, "#client-notes", "Notes")
	measure("#client-notes")

	navigate(t, tab, strings.TrimSuffix(path, "/clients")+"/transactions")
	click(t, tab, `a[href*="view=record"]`)
	waitVisible(t, tab, "#transactions-description")
	measure("#transactions-description")
	measure("#transactions-direction")
	click(t, tab, `label[for="transactions-description"]`)
	measure("#transactions-description")
	descriptionLabel := eval[string](t, tab, `document.querySelector('label[for="transactions-description"]').textContent.trim()`)
	fill(t, tab, "#transactions-description", descriptionLabel)
	measure("#transactions-description")
	valueX = captureStart("#transactions-description", false)
	fill(t, tab, "#transactions-description", "")
	click(t, tab, "#transactions-amount")
	labelX = captureStart("#transactions-description", true)
	if math.Abs(float64(labelX-valueX)) > 1 {
		t.Fatalf("transaction placeholder starts at %d; entered text starts at %d", labelX, valueX)
	}
}
