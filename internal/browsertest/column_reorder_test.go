//go:build browser

package browsertest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp/kb"
)

const columnMenu = `.list-menu:has(#list-column-switches) > summary`
const columnRows = `#list-column-switches > li[data-column-key]`

func watchColumnListRefresh(t *testing.T, tab context.Context) {
	t.Helper()
	call(t, tab, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: `
		document.addEventListener('htmx:after:swap', event => {
			const ctx = event.detail?.ctx;
			if (ctx?.target?.id === 'client-list' && ctx.request?.method?.toLowerCase() === 'get') {
				window.columnInitialRefreshDone = true;
			}
		});
	`})
}

func waitColumnListRefresh(t *testing.T, tab context.Context) {
	t.Helper()
	eventually(t, tab, `window.columnInitialRefreshDone === true`, "initial realtime list refresh did not finish")
}

func columnOrder(t *testing.T, tab context.Context) []string {
	t.Helper()
	return eval[[]string](t, tab, `Array.from(document.querySelectorAll('#list-column-switches > li[data-column-key]'), row => row.dataset.columnKey)`)
}

func columnPoint(t *testing.T, tab context.Context, index int, fraction float64) []float64 {
	t.Helper()
	return eval[[]float64](t, tab, fmt.Sprintf(`(() => {
		const row = document.querySelectorAll(%s)[%d], r = row.getBoundingClientRect();
		const handle = row.querySelector('.list-column-handle').getBoundingClientRect();
		return [handle.left + handle.width / 2, r.top + r.height * %f];
	})()`, jsString(columnRows), index, fraction))
}

func columnMouse(t *testing.T, tab context.Context, kind input.DispatchMouseEventType, point []float64, down bool) {
	t.Helper()
	params := input.DispatchMouseEventParams{Type: kind, X: point[0], Y: point[1]}
	if down {
		params.Buttons = 1
		params.Button = input.MouseButtonLeft
	}
	if kind != input.DispatchMouseEventTypeMouseMoved {
		params.Button = input.MouseButtonLeft
		params.ClickCount = 1
	}
	call(t, tab, input.DispatchMouseEvent, params)
}

func moveColumnPointer(t *testing.T, tab context.Context, from, to []float64, steps int) {
	t.Helper()
	for i := 1; i <= steps; i++ {
		fraction := float64(i) / float64(steps)
		point := []float64{from[0] + (to[0]-from[0])*fraction, from[1] + (to[1]-from[1])*fraction}
		columnMouse(t, tab, input.DispatchMouseEventTypeMouseMoved, point, true)
	}
}

func startColumnDrag(t *testing.T, tab context.Context, source, destination []float64) {
	t.Helper()
	columnMouse(t, tab, input.DispatchMouseEventTypeMouseMoved, source, false)
	columnMouse(t, tab, input.DispatchMouseEventTypeMousePressed, source, true)
	lifted := []float64{source[0], source[1] + 10}
	moveColumnPointer(t, tab, source, lifted, 5)
	moveColumnPointer(t, tab, lifted, destination, 10)
	eventually(t, tab, `Boolean(document.querySelector('.list-column-placeholder'))`, "drag did not show an insertion gap")
}

func TestColumnDragPreviewDropAndCancel(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	watchColumnListRefresh(t, tab)
	openClients(t, tab)
	ensureClient(t, tab)
	waitColumnListRefresh(t, tab)
	click(t, tab, columnMenu)
	original := columnOrder(t, tab)
	if len(original) < 3 || original[0] != "displayName" {
		t.Fatalf("expected locked name and two movable columns: %v", original)
	}
	eval[bool](t, tab, `(() => {
		window.columnOrderSubmissions = 0;
		document.addEventListener('submit', event => {
			if (event.target.matches('form[action*="/preferences/columns/order"]')) window.columnOrderSubmissions++;
		}, true);
		return true;
	})()`)

	source := columnPoint(t, tab, 2, 0.5)
	locked := columnPoint(t, tab, 0, 0.2)
	startColumnDrag(t, tab, source, locked)
	if !eval[bool](t, tab, `(() => {
		const list = document.querySelector('#list-column-switches');
		const gap = list.querySelector('.list-column-placeholder');
		return list.firstElementChild.matches('[data-locked][draggable="false"]') && gap && gap !== list.firstElementChild &&
			gap.getBoundingClientRect().height > 0 && !gap.querySelector('button, input, form');
	})()`) {
		t.Fatal("drag must show an empty insertion gap after the locked column")
	}
	if got := columnOrder(t, tab); fmt.Sprint(got) != fmt.Sprint(original) {
		t.Fatalf("columns changed before drop: %v, want %v", got, original)
	}
	eventually(t, tab, `document.querySelector('#list-column-switches > [data-locked]')?.nextElementSibling?.matches('.list-column-placeholder')`, "dragging over the locked column did not place the gap after it")
	columnMouse(t, tab, input.DispatchMouseEventTypeMouseReleased, locked, false)
	want := append([]string(nil), original...)
	want[1], want[2] = want[2], want[1]
	eventually(t, tab, fmt.Sprintf(`JSON.stringify(Array.from(document.querySelectorAll(%s), r => r.dataset.columnKey)) === %s`, jsString(columnRows), jsStringToJSON(t, want)), "drop did not save reordered columns")
	if got := eval[int](t, tab, `window.columnOrderSubmissions`); got != 1 {
		t.Fatalf("drop submitted %d saves, want 1", got)
	}

	// A second drag is cancelled with Escape, without submitting a new order.
	source = columnPoint(t, tab, 2, 0.5)
	target := columnPoint(t, tab, 1, 0.2)
	startColumnDrag(t, tab, source, target)
	press(t, tab, kb.Escape)
	columnMouse(t, tab, input.DispatchMouseEventTypeMouseReleased, target, false)
	eventually(t, tab, `!document.querySelector('.list-column-placeholder, .list-column-drag-preview')`, "cancel left drag artifacts")
	if got := eval[int](t, tab, `window.columnOrderSubmissions`); got != 1 {
		t.Fatalf("cancel submitted another save: %d", got)
	}
	if got := columnOrder(t, tab); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("cancel changed order: %v, want %v", got, want)
	}
	navigate(t, tab, eval[string](t, tab, `location.pathname`))
	waitColumnListRefresh(t, tab)
	click(t, tab, columnMenu)
	if got := columnOrder(t, tab); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("reloaded order = %v, want %v", got, want)
	}
	click(t, tab, `.list-columns-panel form[action*="/preferences/columns/reset"] button`)
}

func TestColumnReorderKeyboardAndNoJavaScript(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	watchColumnListRefresh(t, tab)
	openClients(t, tab)
	ensureClient(t, tab)
	waitColumnListRefresh(t, tab)
	path := eval[string](t, tab, `location.pathname`)
	click(t, tab, columnMenu)
	before := columnOrder(t, tab)
	if len(before) < 3 {
		t.Fatalf("need at least three columns: %v", before)
	}
	button := fmt.Sprintf(`%s:nth-child(3) .list-column-move button`, columnRows)
	if !eval[bool](t, tab, fmt.Sprintf(`(() => { const form = document.querySelector(%s).closest('form'), r = form.getBoundingClientRect(), s = getComputedStyle(form); return (r.width <= 1 && r.height <= 1 && s.overflow === 'hidden' && s.clipPath !== 'none') || s.visibility === 'hidden'; })()`, jsString(button))) {
		t.Fatal("move controls should not be visible before keyboard focus")
	}
	eval[bool](t, tab, `(() => { document.querySelectorAll('#list-column-switches > li[data-column-key]')[2].querySelector('input[type=checkbox]').focus(); return true; })()`)
	press(t, tab, kb.Tab)
	if !eval[bool](t, tab, fmt.Sprintf(`(() => { const b = document.querySelector(%s), r = b.getBoundingClientRect(); return document.activeElement === b && r.width > 1 && r.height > 1 && getComputedStyle(b).visibility === 'visible'; })()`, jsString(button))) {
		t.Fatal("focused keyboard reorder control must be visible")
	}
	press(t, tab, kb.Enter)
	want := append([]string(nil), before...)
	want[1], want[2] = want[2], want[1]
	eventually(t, tab, fmt.Sprintf(`JSON.stringify(Array.from(document.querySelectorAll(%s), r => r.dataset.columnKey)) === %s`, jsString(columnRows), jsStringToJSON(t, want)), "keyboard move did not reorder")

	call(t, tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	navigate(t, tab, path)
	click(t, tab, columnMenu)
	if got := columnOrder(t, tab); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("keyboard order was not persisted: %v, want %v", got, want)
	}
	waitVisible(t, tab, button)
	click(t, tab, button)
	click(t, tab, columnMenu)
	if got := columnOrder(t, tab); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("no-JavaScript move = %v, want %v", got, before)
	}
}

func jsStringToJSON(t *testing.T, values []string) string {
	t.Helper()
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return jsString(string(data))
}
