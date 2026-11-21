//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/chromedp"
)

// TestItemsCreateFormDropsOverlappingSubmissions ports the overlapping HTMX submission test:
// two submissions of the create form send one POST, which the page accepts once.
func TestItemsCreateFormDropsOverlappingSubmissions(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openItems(t, tab)
	gate := make(chan struct{})
	posts := make(chan string, 4)
	call(t, tab, fetch.Enable, fetch.EnableParams{Patterns: []*fetch.RequestPattern{{URLPattern: "*/items", RequestStage: fetch.RequestStageRequest}}})
	_ = chromedp.Listen(tab, func(ev any) {
		paused, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		go func(requestID fetch.RequestID, method string) {
			if method == "POST" {
				posts <- string(requestID)
				<-gate
			}
			_, _ = chromedp.Run[cdp.Empty](tab, func(ctx context.Context, target *chromedp.Target) (cdp.Empty, error) {
				return cdp.Call(ctx, target, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: requestID})
			})
		}(paused.RequestID, paused.Request.Method)
	})
	eval[any](t, tab, `(() => {
		const form = document.querySelector("form.create");
		const values = Object.fromEntries(new FormData(form));
		values.title = "One accepted submit";
		htmx.ajax("POST", form.action, { source: form, target: "#item-list", swap: "outerHTML", values });
		htmx.ajax("POST", form.action, { source: form, target: "#item-list", swap: "outerHTML", values });
	})()`)
	select {
	case <-posts:
	case <-time.After(actionWait):
		t.Fatal("no POST reached the server")
	}
	// A second POST would arrive shortly after the first if the overlap were not dropped.
	select {
	case <-posts:
		t.Fatal("overlapping submission sent a second POST")
	case <-time.After(time.Second):
	}
	close(gate)
	eventually(t, tab, `[...document.querySelectorAll('#item-list input[name="title"]')].some((i) => i.getAttribute("value") === "One accepted submit")`, "accepted item not listed")
}

// TestItemsAutosaveUpdatesVersionBeforeLaterDelete ports the autosave and later delete test.
func TestItemsAutosaveUpdatesVersionBeforeLaterDelete(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openItems(t, tab)
	createItem(t, tab, "Autosave seed")
	fillByLabel(t, tab, "New item title", "   ")
	eval[any](t, tab, `(() => { const input = document.querySelector("#new-title, input[name='title'].create-title, form.create input[name='title']"); input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true })); input.form.requestSubmit(); })()`)
	eventually(t, tab, `document.querySelector(".error-summary") !== null`, "blank title did not show the error summary")
	expectFocused(t, tab, ".error-summary")
	fillByLabel(t, tab, "New item title", "Unsaved create draft")

	// The row keeps its data-item-row identity across the autosave, as Playwright's currentRow did.
	rowID := eval[string](t, tab, fmt.Sprintf(`%s.getAttribute("data-item-row")`, itemRowExpr("Autosave seed")))
	byID := fmt.Sprintf(`document.querySelector('#item-list tr[data-item-row="%s"]')`, rowID)
	fillItemTitle(t, tab, "Autosave seed", "Autosave updated")
	eval[any](t, tab, fmt.Sprintf(`(() => { const row = %s; const select = row.querySelector('select[name="status"]'); select.value = "complete"; select.dispatchEvent(new Event("change", { bubbles: true })); })()`, byID))
	settle(t, tab)
	eventually(t, tab, fmt.Sprintf(`(() => { const row = %s; return Boolean(row) && row.querySelector('input[name="version"]').value === "2"; })()`, byID), "autosave did not bump the version")
	eventually(t, tab, fmt.Sprintf(`(() => { const row = %s; return Boolean(row) && row.querySelector('input[name="title"]').value === "Autosave updated"; })()`, byID), "autosaved title not kept")
	if got := eval[string](t, tab, `document.querySelector("form.create input[name='title'], #new-title").value`); got != "Unsaved create draft" {
		t.Fatalf("create draft %q lost", got)
	}
	expectCount(t, tab, ".error-summary", 1)

	acceptDialogs(tab)
	clickInItemRow(t, tab, "Autosave updated", "Delete permanently Autosave updated")
	eventually(t, tab, fmt.Sprintf("!Boolean(%s)", byID), "deleted item still listed")
}

// TestSpanishItemsListPassesAxe ports the Spanish Items axe test on the item list.
func TestSpanishItemsListPassesAxe(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	path := openItems(t, tab)
	navigate(t, tab, "/language?locale=es&return_to="+urlQuery(path))
	expectAttribute(t, tab, "html", "lang", "es")
	eventually(t, tab, `[...document.querySelectorAll("h1")].some((h) => h.textContent.trim() === "Elementos")`, "Spanish heading missing")
	eventually(t, tab, `!!document.querySelector('input[placeholder="Título del nuevo elemento"], #new-title') && [...document.querySelectorAll("label")].some((l) => l.textContent.includes("Título del nuevo elemento"))`, "Spanish create label missing")
	connected := eval[string](t, tab, `document.getElementById("item-list").getAttribute("data-live-connected") ?? ""`)
	if connected == "" {
		t.Fatal("list has no live state")
	}
	eventually(t, tab, fmt.Sprintf(`document.getElementById("realtime-status")?.textContent.trim() === %s`, jsString(connected)), "realtime status does not match")
	expectAxeClean(t, tab)
}

// TestItemsHonorsExplicitAndSystemTheme ports the theme preference test on the Items page.
func TestItemsHonorsExplicitAndSystemTheme(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	path := openItems(t, tab)
	cases := []struct{ theme, scheme, canvas string }{
		{"light", "dark", "#f3f6fa"},
		{"dark", "light", "#111826"},
		{"system", "light", "#f3f6fa"},
		{"system", "dark", "#111826"},
	}
	for _, c := range cases {
		restore := setAccountTheme(t, tab, c.theme)
		t.Cleanup(restore)
		call(t, tab, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: c.scheme}, {Name: "prefers-reduced-motion", Value: "reduce"}}})
		navigate(t, tab, path)
		expectTheme(t, tab, c.theme)
		if got := eval[string](t, tab, `getComputedStyle(document.documentElement).getPropertyValue("--canvas").trim()`); got != c.canvas {
			t.Fatalf("%s/%s canvas %q, want %q", c.theme, c.scheme, got, c.canvas)
		}
	}
}
