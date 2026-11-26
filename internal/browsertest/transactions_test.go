//go:build browser

package browsertest

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestTransactionsEntryJourney(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signIn(t, tab)
	if eval[bool](t, tab, `location.pathname === "/"`) {
		click(t, tab, ".cards a")
	}
	waitVisible(t, tab, "[data-workspace-home]")
	workspace := eval[string](t, tab, `location.pathname`)
	path := workspace + "/transactions"
	navigate(t, tab, path)
	eventually(t, tab, `(() => { const h = document.querySelector(".list-toolbar").getBoundingClientRect(); const a = document.querySelector("[data-transactions-actions]").getBoundingClientRect(); const s = document.querySelector("#list-search").getBoundingClientRect(); return Math.abs(h.right - a.right) < 2 && a.left > h.left + h.width / 2 && Math.abs(a.top - s.top) < 20; })()`, "transactions actions are not right aligned with the list toolbar")
	eventually(t, tab, `(() => { const a = document.querySelector("[data-transactions-create]").getBoundingClientRect(); const b = document.querySelector("[data-transactions-actions] summary").getBoundingClientRect(); return b.left - a.right >= 8 && Math.abs(a.height - b.height) < 2; })()`, "transactions header buttons need a gap and matching heights")
	click(t, tab, `a[href*="view=record"]`)
	waitVisible(t, tab, "#transactions-date")
	eventually(t, tab, `(() => { const label = document.querySelector('label[for="transactions-description"]').getBoundingClientRect(); const input = document.querySelector('#transactions-description').getBoundingClientRect(); return input.left > label.right && Math.abs(input.top - label.top) < input.height; })()`, "creation labels and inputs should share a row")
	eventually(t, tab, `getComputedStyle(document.querySelector("#transactions-description")).borderTopColor === "rgba(0, 0, 0, 0)"`, "creation input should use the quiet detail-editor style")
	date := eval[string](t, tab, `document.querySelector("#transactions-date").value`)
	before := time.Now().UTC().Add(-time.Minute).Format("2006-01-02")
	after := time.Now().UTC().Add(time.Minute).Format("2006-01-02")
	if date != before && date != after {
		t.Fatalf("default workspace date = %q, want %q or %q", date, before, after)
	}
	eventually(t, tab, `document.querySelector('[data-show-when="transactions-direction=out"]')?.hidden === true`, "expense category should be hidden for money in")
	if eval[bool](t, tab, `document.querySelector("#transactions-amount-mode").closest("details").open`) {
		t.Fatal("advanced currency options should be collapsed")
	}
	fillByLabel(t, tab, "Transaction type", "out")
	eventually(t, tab, `document.querySelector('[data-show-when="transactions-direction=out"]')?.hidden === false`, "expense category did not appear for money out")
	fillByLabel(t, tab, "Transaction type", "in")
	eventually(t, tab, `document.querySelector('[data-show-when="transactions-direction=out"]')?.hidden === true`, "expense category did not hide for money in")
	fill(t, tab, "#transactions-description", "Browser cash entry")
	fill(t, tab, "#transactions-amount", "125")
	fill(t, tab, "#transactions-currency", "XX")
	fillByLabel(t, tab, "Payment method", "cash")
	fillByLabel(t, tab, "Date", "2025-01-02")
	expectLabelValue(t, tab, "Date", "2025-01-02")
	expectLabelValue(t, tab, "Payment method", "cash")
	expectLabelValue(t, tab, "Amount", "125")
	click(t, tab, `details:has(#transactions-note) > summary`)
	fill(t, tab, "#transactions-note", "Browser cash entry")
	run(t, tab, chromedp.Focus(`[data-transactions-submit]`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `Boolean(document.querySelector('[role="alert"]'))`, "validation error not shown")
	eventually(t, tab, `document.activeElement?.matches('[role="alert"]')`, "validation error did not take focus")
	eventually(t, tab, `document.querySelector("#transactions-currency")?.getAttribute("aria-describedby") === "transactions-currency-error"`, "currency error not associated with field")
	if value := eval[string](t, tab, `document.querySelector("#transactions-note").value`); value != "Browser cash entry" {
		t.Fatalf("validation lost draft: %q", value)
	}
	expectLabelValue(t, tab, "Description", "Browser cash entry")
	expectLabelValue(t, tab, "Date", "2025-01-02")
	expectLabelValue(t, tab, "Payment method", "cash")
	expectLabelValue(t, tab, "Amount", "125")
	fill(t, tab, "#transactions-currency", "USD")
	fill(t, tab, "#transactions-client-search", "no-such-client")
	run(t, tab, chromedp.Focus(`#transactions-client-search`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `Array.from(document.querySelectorAll('[role="status"]')).some(e => e.textContent.includes("No clients match your search."))`, "client search did not run")
	eventually(t, tab, `document.activeElement?.id === "transactions-client"`, "client selector did not regain focus")
	expectLabelValue(t, tab, "Amount", "125")
	eventually(t, tab, `document.querySelector("#transactions-currency")?.value === "USD"`, "currency draft was not preserved after client search")
	if eval[bool](t, tab, `Boolean(document.querySelector("#transactions-entries-heading"))`) {
		t.Fatal("client search left the record journey")
	}
	run(t, tab, chromedp.Focus(`[data-transactions-submit]`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `document.querySelector("#transactions-entries-heading")?.parentElement.textContent.includes("Browser cash entry")`, "saved entry not shown")
	eventually(t, tab, `Array.from(document.querySelectorAll('[role="status"]')).some(e => e.textContent.includes("Entry recorded."))`, "success confirmation not shown")
	row := eval[string](t, tab, `document.querySelector("#transactions-entries-heading").parentElement.textContent`)
	for _, value := range []string{"125", "USD", "Cash", "2025-01-02"} {
		if !strings.Contains(row, value) {
			t.Fatalf("entry missing %q: %s", value, row)
		}
	}
	click(t, tab, `.list-view-table tbody tr:first-child td[data-column="description"] a`)
	eventually(t, tab, `document.querySelector("#transactions-entry-details")?.matches(":modal")`, "details must open as a modal")
	eventually(t, tab, `(() => { const dl = document.querySelector("#transactions-entry-details dl"); const label = dl.querySelector("dt").getBoundingClientRect(); const value = dl.querySelector("dd").getBoundingClientRect(); return value.left > label.right && Math.abs(value.top - label.top) < value.height; })()`, "detail labels and values should share a row")
	click(t, tab, `[data-transactions-details-close]`)
	eventually(t, tab, `!document.querySelector("#transactions-entry-details")`, "close icon did not return to the list")
	click(t, tab, `.list-view-table tbody tr:first-child td[data-column="description"] a`)
	run(t, tab, chromedp.KeyEvent("\x1b"))
	eventually(t, tab, `!document.querySelector("#transactions-entry-details")`, "Escape did not close details")
	click(t, tab, `.list-view-table tbody tr:first-child .list-row-menu-button`)
	click(t, tab, `.list-view-table tbody tr:first-child button[name="correction_action"][value^="refund:"]`)
	eventually(t, tab, `document.querySelector('input[name="refund_of_entry_id"]')?.value !== "" && document.querySelector("#transactions-direction")?.value === "out"`, "refund selection did not prefill the entry form")
	eventually(t, tab, `document.querySelector('[data-show-when="transactions-direction=out"]')?.hidden === false`, "refund category should be visible")
	navigate(t, tab, path)
	click(t, tab, `.list-view-table tbody tr:first-child .list-row-menu-button`)
	click(t, tab, `.list-view-table tbody tr:first-child a[href*="action=void"]`)
	eventually(t, tab, `document.querySelector('#transactions-entry-details details')?.open === true`, "void action did not open confirmation")
	click(t, tab, `#transactions-entry-details input[name="confirm_void"]`)
	click(t, tab, `#transactions-entry-details form[action*="/void"] button[type="submit"]`)
	eventually(t, tab, `document.querySelector('#transactions-entries-heading')?.parentElement.textContent.includes("Voided")`, "voided entry not shown")
	click(t, tab, `.list-view-table tbody tr:first-child .list-row-menu-button`)
	click(t, tab, `.list-view-table tbody tr:first-child button[name="correction_action"][value^="replace:"]`)
	eventually(t, tab, `document.querySelector('input[name="replaces_entry_id"]')?.value !== "" && document.querySelector("#transactions-direction")?.value === "in"`, "replacement selection did not prefill the entry form")
	navigate(t, tab, "/language?locale=es&return_to="+url.QueryEscape(path))
	eventually(t, tab, `document.querySelector('a[href*="view=record"]')?.textContent.includes("Registrar movimiento")`, "Spanish transactions not shown")
	if text := eval[string](t, tab, `document.querySelector("#transactions-entries-heading").parentElement.textContent`); !strings.Contains(text, "Efectivo") {
		t.Fatalf("cash method not translated: %s", text)
	}
}

func TestTransactionsExpenseCategories(t *testing.T) {
	tab := newTab(t, 390, 844, "")
	acceptDialogs(tab)
	signIn(t, tab)
	if eval[bool](t, tab, `location.pathname === "/"`) {
		click(t, tab, ".cards a")
	}
	waitVisible(t, tab, "[data-workspace-home]")
	path := eval[string](t, tab, `location.pathname`) + "/transactions"
	navigate(t, tab, path)
	eventually(t, tab, `(() => { const h = document.querySelector(".list-toolbar").getBoundingClientRect(); const a = document.querySelector("[data-transactions-actions]").getBoundingClientRect(); return Math.abs(h.right - a.right) < 2 && a.left >= h.left && a.right <= innerWidth; })()`, "mobile transactions actions are not right aligned in the viewport")
	click(t, tab, `[data-transactions-actions] > details > summary`)
	click(t, tab, `a[href$="/settings/transactions/categories"]`)
	waitVisible(t, tab, "#transactions-new-category")
	fill(t, tab, "#transactions-new-category", "   ")
	run(t, tab, chromedp.Focus(`#transactions-new-category`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `["transactions-category-error", "transactions-new-category"].includes(document.activeElement?.id) && document.querySelector("#transactions-new-category")?.getAttribute("aria-describedby") === "transactions-category-error"`, "category error did not receive accessible focus")
	if value := eval[string](t, tab, `document.querySelector("#transactions-new-category").value`); value != "   " {
		t.Fatalf("category draft lost: %q", value)
	}
	fill(t, tab, "#transactions-new-category", "Office rent")
	run(t, tab, chromedp.Focus(`#transactions-new-category`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `document.querySelector('section:has(#transactions-categories-heading)')?.textContent.includes("Office rent")`, "new category not shown")
	fill(t, tab, "#transactions-new-category", "Office rent")
	run(t, tab, chromedp.Focus(`#transactions-new-category`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `["transactions-category-error", "transactions-new-category"].includes(document.activeElement?.id) && document.querySelector("#transactions-category-error")?.textContent.includes("already exists")`, "duplicate category not explained")
	navigate(t, tab, path+"?view=record")
	waitVisible(t, tab, "#transactions-amount")
	eventually(t, tab, `(() => { const label = document.querySelector('label[for="transactions-description"]').getBoundingClientRect(); const input = document.querySelector('#transactions-description').getBoundingClientRect(); return input.top >= label.bottom; })()`, "mobile creation labels should stack above inputs")
	fill(t, tab, "#transactions-description", "Category test")
	fill(t, tab, "#transactions-amount", "125")
	fill(t, tab, "#transactions-currency", "USD")
	fillByLabel(t, tab, "Payment method", "cash")
	fillByLabel(t, tab, "Transaction type", "out")
	categoryID := eval[string](t, tab, `Array.from(document.querySelector("#transactions-category").options).find(o => o.textContent.trim() === "Office rent").value`)
	fillByLabel(t, tab, "Expense category", categoryID)
	click(t, tab, `details:has(#transactions-note) > summary`)
	fill(t, tab, "#transactions-note", "Category test")
	run(t, tab, chromedp.Focus(`[data-transactions-submit]`), chromedp.KeyEvent("\r"))
	eventually(t, tab, `document.querySelector("#transactions-entries-heading").parentElement.textContent.includes("Office rent")`, "category missing in history")
	navigate(t, tab, strings.TrimSuffix(path, "/transactions")+"/settings/transactions/categories")
	categoryInput := `#transactions-category-` + categoryID
	click(t, tab, `section:has(#transactions-categories-heading) li:has(`+categoryInput+`) summary`)
	fill(t, tab, categoryInput, "Rent")
	run(t, tab, chromedp.Focus(categoryInput), chromedp.KeyEvent("\r"))
	eventually(t, tab, `document.querySelector('section:has(#transactions-categories-heading)')?.textContent.includes("Rent")`, "rename missing")
	click(t, tab, `section:has(#transactions-categories-heading) li:has(`+categoryInput+`) summary`)
	run(t, tab, chromedp.Click(`section:has(#transactions-categories-heading) button[value="archive"]`))
	eventually(t, tab, `Array.from(document.querySelectorAll('section:has(#transactions-categories-heading) li')).some(row => row.textContent.trim() === "Rent (Archived)")`, "category not archived")
	navigate(t, tab, path+"?view=record")
	eventually(t, tab, `!Array.from(document.querySelector("#transactions-category").options).some(o => o.textContent.trim() === "Rent")`, "archived category remained selectable")
	navigate(t, tab, "/language?locale=es&return_to="+url.QueryEscape(strings.TrimSuffix(path, "/transactions")+"/settings/transactions/categories"))
	eventually(t, tab, `document.querySelector("#transactions-categories-heading")?.textContent.includes("Administrar categorías")`, "Spanish categories not shown")
}
