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

func TestTransactionsFiltersClientHistoryViewerSpanishKeyboard(t *testing.T) {
	owner := newTab(t, 1280, 900, "")
	signIn(t, owner)
	if eval[bool](t, owner, `location.pathname === "/"`) {
		click(t, owner, ".cards a")
	}
	waitVisible(t, owner, "[data-workspace-home]")
	workspace := eval[string](t, owner, `location.pathname`)
	clientName := fmt.Sprintf("Transactions history %d", time.Now().UnixNano())
	navigate(t, owner, workspace+"/clients/new")
	fillByLabel(t, owner, "Display name", clientName)
	clickByText(t, owner, "Create client")
	eventually(t, owner, `/\/clients\/[^/?]+$/.test(location.pathname)`, "client detail did not open")
	clientPath := eval[string](t, owner, `location.pathname`)
	clientID := clientPath[strings.LastIndex(clientPath, "/")+1:]
	for i := range 5 {
		navigate(t, owner, workspace+"/clients/new")
		fillByLabel(t, owner, "Display name", fmt.Sprintf("Picker client %d %d", time.Now().UnixNano(), i))
		clickByText(t, owner, "Create client")
		eventually(t, owner, `/\/clients\/[^/?]+$/.test(location.pathname)`, "picker client did not save")
	}
	transactionsPath := workspace + "/transactions"
	navigate(t, owner, transactionsPath)
	click(t, owner, `a[href*="view=record"]`)
	click(t, owner, `.transaction-advanced > summary`)
	click(t, owner, `[data-entry-field]:has(#transactions-client) .entry-picker-trigger`)
	fill(t, owner, `[data-entry-field]:has(#transactions-client) .entry-picker-search`, clientName)
	eventually(t, owner, fmt.Sprintf(`(() => { const options = [...document.querySelectorAll('[data-entry-field]:has(#transactions-client) [role="option"]')]; return options.length === 1 && options[0].textContent === %s; })()`, jsString(clientName)), "live client search result missing")
	click(t, owner, `[data-entry-field]:has(#transactions-client) [role="option"]`)
	eventually(t, owner, fmt.Sprintf(`document.querySelector("#transactions-client").value === %s`, jsString(clientID)), "client choice was not retained")
	fill(t, owner, "#transactions-description", "Private payment")
	fill(t, owner, "#transactions-amount", "9731")
	selectByLabelID(t, owner, "transactions-currency", "USD")
	selectByLabelID(t, owner, "transactions-method", "cash")
	fillByLabel(t, owner, "Date", "2026-01-02")
	fill(t, owner, "#transactions-note", "Private browser history payment")
	click(t, owner, `[data-transactions-submit]`)
	eventually(t, owner, `document.querySelector('#transactions-entries-heading')?.parentElement.textContent.includes('Private browser history payment')`, "client payment missing")
	click(t, owner, `a[href*="view=record"]`)
	fill(t, owner, "#transactions-description", "Other payment")
	fill(t, owner, "#transactions-amount", "8752")
	selectByLabelID(t, owner, "transactions-currency", "USD")
	selectByLabelID(t, owner, "transactions-method", "card")
	fillByLabel(t, owner, "Date", "2026-01-03")
	click(t, owner, `.transaction-advanced > summary`)
	fill(t, owner, "#transactions-note", "Other browser payment")
	click(t, owner, `[data-transactions-submit]`)
	eventually(t, owner, `document.querySelector('#transactions-entries-heading')?.parentElement.textContent.includes('Other browser payment')`, "second payment missing")

	navigate(t, owner, transactionsPath+"?filter_field=client_id")
	waitVisible(t, owner, "#transactions-filter-search")
	run(t, owner, chromedp.Focus("#transactions-filter-search"))
	assertTabFocus(t, owner, `form:has(#transactions-filter-search) button[type="submit"]`)
	fill(t, owner, "#transactions-filter-search", clientName)
	run(t, owner, chromedp.Focus(`form:has(#transactions-filter-search) button[type="submit"]`), chromedp.KeyEvent("\r"))
	eventually(t, owner, fmt.Sprintf(`new URLSearchParams(location.search).get('filter_search') === %s`, jsString(clientName)), "client search did not finish navigating")
	eventually(t, owner, fmt.Sprintf(`Array.from(document.querySelectorAll('#transactions-filter-client option')).some(o => o.value === %s)`, jsString(clientID)), "filter client search result missing")
	eventually(t, owner, `document.activeElement?.id === 'transactions-filter-client'`, "filter client selector did not receive focus")
	selectByLabelID(t, owner, "transactions-filter-client", clientID)
	click(t, owner, `form:has(#transactions-filter-client) button[type="submit"]`)
	eventually(t, owner, `document.querySelector('.list-filter-chip') !== null`, "client filter capsule missing")
	values := url.Values{"filter": {"client_id:equals:" + clientID}, "filter_field": {"payment_method"}}
	navigate(t, owner, transactionsPath+"?"+values.Encode())
	selectByLabelID(t, owner, "list-filter-value", "cash")
	click(t, owner, `form:has(#list-filter-value) button[type="submit"]`)
	eventually(t, owner, `document.querySelectorAll('.list-filter-chip').length === 2`, "payment method filter capsule missing")
	text := eval[string](t, owner, `document.querySelector('#transactions-entries-heading').parentElement.textContent`)
	if !strings.Contains(text, "Private browser history payment") || strings.Contains(text, "Other browser payment") {
		t.Fatalf("filtered entries: %s", text)
	}
	navigate(t, owner, clientPath)
	waitVisible(t, owner, "#client-transactions-heading")
	text = eval[string](t, owner, `document.querySelector('#client-transactions-heading').parentElement.textContent`)
	if !strings.Contains(text, "Private browser history payment") || !strings.Contains(text, "USD 9,731.00") || strings.Contains(text, "Other browser payment") {
		t.Fatalf("client history: %s", text)
	}
	navigate(t, owner, "/language?locale=es&return_to="+url.QueryEscape(clientPath))
	eventually(t, owner, `document.querySelector('#client-transactions-heading')?.textContent.includes('Historial de movimientos del cliente')`, "Spanish client history missing")
	navigate(t, owner, "/language?locale=es&return_to="+url.QueryEscape(transactionsPath))
	eventually(t, owner, `document.querySelector('#list-search')?.placeholder.includes('movimientos')`, "Spanish search controls missing")

	navigate(t, owner, "/language?locale=en&return_to="+url.QueryEscape(workspace+"/settings/invitations"))
	fill(t, owner, "#invite-email", fmt.Sprintf("transactions-viewer-%d@example.com", time.Now().UnixNano()))
	selectByLabelID(t, owner, "invite-role", "viewer")
	click(t, owner, `form[action$="/settings/invitations"] button[name="action"][value="create"]`)
	waitVisible(t, owner, `input[aria-label="Copy this invitation link"]`)
	invitation := eval[string](t, owner, `document.querySelector('input[aria-label="Copy this invitation link"]').value`)
	viewer := newTab(t, 1280, 900, "")
	navigate(t, viewer, invitation)
	fill(t, viewer, "#invite-password", "viewer-long-password")
	click(t, viewer, `button[name="action"][value="register"]`)
	eventually(t, viewer, fmt.Sprintf(`location.pathname === %s`, jsString(workspace)), "viewer did not reach workspace")
	navigate(t, viewer, clientPath)
	waitVisible(t, viewer, "#client-detail")
	text = eval[string](t, viewer, `document.body.textContent`)
	if strings.Contains(text, "Private browser history payment") || strings.Contains(text, "USD 9,731.00") || eval[bool](t, viewer, `Boolean(document.querySelector('#client-transactions-heading'))`) {
		t.Fatalf("viewer can see transactions history: %s", text)
	}
}
