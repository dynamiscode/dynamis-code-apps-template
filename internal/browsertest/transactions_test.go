//go:build browser

package browsertest

import (
	"fmt"
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
	clientPrefix := fmt.Sprintf("Journey picker %d", time.Now().UnixNano())
	for i := range 6 {
		navigate(t, tab, workspace+"/clients/new")
		fillByLabel(t, tab, "Display name", fmt.Sprintf("%s %d", clientPrefix, i))
		clickByText(t, tab, "Create client")
		eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "picker client did not save")
	}
	for _, code := range []string{"EUR", "GBP", "JPY", "CAD", "AUD", "BHD"} {
		navigate(t, tab, workspace+"/settings/currencies")
		if eval[bool](t, tab, `Array.from(document.querySelector('select[name="new_code"]').options).some(option => option.value === `+jsString(code)+`)`) {
			fillByLabel(t, tab, "Currency to enable", code)
			click(t, tab, `.settings-page form[method="post"] button[type="submit"]`)
			eventually(t, tab, `!Array.from(document.querySelector('select[name="new_code"]')?.options || []).some(option => option.value === `+jsString(code)+`)`, "currency was not added")
		}
	}
	path := workspace + "/transactions"
	navigate(t, tab, path)
	if eval[bool](t, tab, `!!document.querySelector(".list-toolbar")`) {
		eventually(t, tab, `(() => { const a = document.querySelector("[data-transactions-actions]"); const controls = [a.querySelector("a[data-transactions-create]"), a.querySelector("summary")].filter(Boolean); return controls.length > 0 && controls.every(e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.left >= 0 && r.right <= innerWidth; }); })()`, "transactions actions are not visible")
	} else {
		eventually(t, tab, `document.querySelectorAll("[data-transactions-create]").length === 1 && !document.querySelector("#list-search")`, "empty transactions should offer one create action without search")
	}
	click(t, tab, `a[href*="view=record"]`)
	waitVisible(t, tab, `#transactions-date`)
	eventually(t, tab, `document.querySelector('[data-transactions-submit]').disabled`, "empty required fields must disable create")
	eventually(t, tab, `document.querySelector('[data-transactions-dialog] .transaction-advanced')?.open === false`, "optional creation fields should start collapsed")
	eventually(t, tab, `document.querySelector('label[for="transactions-description"]')?.textContent.includes("Description")`, "description must have an accessible label")
	fill(t, tab, "#transactions-description", "Draft description")
	click(t, tab, "#transactions-amount")
	fill(t, tab, "#transactions-amount", "1x2")
	eventually(t, tab, `document.querySelector("#transactions-amount").value === "12"`, "amount must reject letters while retaining digits")
	eventually(t, tab, `document.querySelector("#transactions-description").value === "Draft description"`, "outside click must preserve the draft")
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
	selectByLabelID(t, tab, "transactions-direction", "out")
	eventually(t, tab, `document.querySelector('[data-show-when="transactions-direction=out"]')?.hidden === false`, "expense category did not appear for money out")
	selectByLabelID(t, tab, "transactions-direction", "in")
	eventually(t, tab, `document.querySelector('[data-show-when="transactions-direction=out"]')?.hidden === true`, "expense category did not hide for money in")
	click(t, tab, `#transactions-description`)
	fill(t, tab, "#transactions-description", "   ")
	fill(t, tab, "#transactions-amount", "125")
	click(t, tab, `[data-entry-field]:has(#transactions-currency) .entry-picker-trigger`)
	fill(t, tab, `[data-entry-field]:has(#transactions-currency) .entry-picker-search`, "USD")
	eventually(t, tab, `Array.from(document.querySelectorAll('[data-entry-field]:has(#transactions-currency) [role="option"]')).some(o => o.textContent.includes("USD")) && !Array.from(document.querySelectorAll('[data-entry-field]:has(#transactions-currency) [role="option"]')).some(o => o.textContent.includes("EUR"))`, "currency search did not filter live")
	run(t, tab, chromedp.KeyEvent("\x1b"))
	click(t, tab, `[data-entry-field]:has(#transactions-direction) .entry-picker-trigger`)
	eventually(t, tab, `!document.querySelector('[data-entry-field]:has(#transactions-direction) .entry-picker-search')`, "two-choice picker must not show search")
	run(t, tab, chromedp.KeyEvent("\x1b"))
	selectByLabelID(t, tab, "transactions-currency", "USD")
	selectByLabelID(t, tab, "transactions-method", "cash")
	fillByLabel(t, tab, "Date", "2025-01-02")
	expectLabelValue(t, tab, "Date", "2025-01-02")
	expectLabelValue(t, tab, "Payment method", "cash")
	expectLabelValue(t, tab, "Amount", "125")
	click(t, tab, `.transaction-advanced > summary`)
	fill(t, tab, "#transactions-note", "Browser cash entry")
	eventually(t, tab, `document.querySelector('[data-transactions-submit]').disabled`, "blank description must disable create")
	fill(t, tab, "#transactions-description", "Browser cash entry")
	eventually(t, tab, `!document.querySelector('[data-transactions-submit]').disabled`, "correcting required fields must enable create")
	fill(t, tab, "#transactions-amount", "1.234")
	eventually(t, tab, `document.querySelector('[data-transactions-submit]').disabled`, "USD amount with excess precision must disable create")
	selectByLabelID(t, tab, "transactions-currency", "BHD")
	eventually(t, tab, `!document.querySelector('[data-transactions-submit]').disabled`, "currency change must revalidate amount precision")
	selectByLabelID(t, tab, "transactions-currency", "USD")
	eventually(t, tab, `document.querySelector('[data-transactions-submit]').disabled`, "changing back to USD must reject excess precision")
	fill(t, tab, "#transactions-amount", "125")
	eventually(t, tab, `!document.querySelector('[data-transactions-submit]').disabled`, "correcting the amount must enable create")
	click(t, tab, `[data-entry-field]:has(#transactions-client) .entry-picker-trigger`)
	eventually(t, tab, `document.querySelector('[data-entry-field]:has(#transactions-client) .entry-picker-trigger')?.getAttribute("aria-expanded") === "true"`, "client picker did not open")
	fill(t, tab, `[data-entry-field]:has(#transactions-client) .entry-picker-search`, clientPrefix)
	eventually(t, tab, fmt.Sprintf(`document.querySelector('[data-transactions-dialog]')?.matches(":modal") && [...document.querySelectorAll('[data-entry-field]:has(#transactions-client) [role="option"]')].some(option => option.textContent.includes(%s))`, jsString(clientPrefix)), "client search did not return results in the record dialog")
	expectLabelValue(t, tab, "Amount", "125")
	eventually(t, tab, `document.querySelector("#transactions-currency")?.value === "USD" && document.querySelector("#transactions-note")?.value === "Browser cash entry"`, "client search did not preserve the draft")
	run(t, tab, chromedp.KeyEvent("\x1b"))
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
	eventually(t, tab, `document.querySelector('#transactions-entry-details .transaction-advanced')?.open === false && !document.querySelector('#transaction-inline-note')?.closest('.transaction-advanced')`, "saved optional values should stay visible while empty values are collapsed")
	click(t, tab, `#transactions-entry-details details:has(#transaction-inline-description) > summary`)
	fill(t, tab, "#transaction-inline-description", "Canceled draft")
	click(t, tab, `#transactions-entry-details details:has(#transaction-inline-description) [data-inline-cancel]`)
	eventually(t, tab, `document.querySelector('#transactions-entry-details details:has(#transaction-inline-description) > summary')?.textContent.includes("Browser cash entry")`, "cancel did not restore the saved description")
	click(t, tab, `#transactions-entry-details details:has(#transaction-inline-description) > summary`)
	fill(t, tab, "#transaction-inline-description", "Updated cash entry")
	click(t, tab, `#transactions-entry-details details:has(#transaction-inline-description) button[type="submit"]`)
	eventually(t, tab, `document.querySelector('#transactions-entry-details details:has(#transaction-inline-description) > summary')?.textContent.includes("Updated cash entry")`, "description edit did not persist")
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
	eventually(t, tab, `document.querySelector('#transactions-entry-details details:has(input[name="confirm_void"])')?.open === true`, "void action did not open confirmation")
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
	eventually(t, tab, `(() => { const h = document.querySelector(".feature-list-heading"); const a = document.querySelector("[data-transactions-actions]"); const controls = [a.querySelector("a[data-transactions-create]"), a.querySelector("summary")].filter(Boolean); return !!h && h.contains(a) && controls.length > 0 && controls.every(e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.left >= 0 && r.right <= innerWidth; }); })()`, "mobile transactions actions are not visible in the title area")
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
	waitVisible(t, tab, `#transactions-amount`)
	fill(t, tab, "#transactions-description", "Category test")
	click(t, tab, `#transactions-amount`)
	fill(t, tab, "#transactions-amount", "125")
	click(t, tab, `[data-entry-field]:has(#transactions-currency) .entry-picker-trigger`)
	selectByLabelID(t, tab, "transactions-currency", "USD")
	run(t, tab, chromedp.KeyEvent("\x1b"))
	selectByLabelID(t, tab, "transactions-method", "cash")
	selectByLabelID(t, tab, "transactions-direction", "out")
	click(t, tab, `.transaction-advanced > summary`)
	eventually(t, tab, `document.querySelector(".transaction-advanced")?.open`, "advanced transaction fields did not open")
	categoryID := eval[string](t, tab, `Array.from(document.querySelector("#transactions-category").options).find(o => o.textContent.trim() === "Office rent").value`)
	selectByLabelID(t, tab, "transactions-category", categoryID)
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
