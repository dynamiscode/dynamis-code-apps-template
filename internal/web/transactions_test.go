package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/transactions"
)

func TestTransactionsWebCreateAndValidation(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableTransactions(transactions.NewService(db, auth))
	h.clients = clients.NewService(db, config.SQLite, auth, 10)
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	client, err := h.clients.Create(ctx, actor, workspace, clients.Input{DisplayName: "Marina Ortega"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdateUserProfile(ctx, owner.UserID, identity.ProfileUpdateInput{DisplayName: "Alex Rivera", Timezone: "UTC", Theme: "system"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspace.String() + "/transactions"
	form := url.Values{"csrf": {session.CSRFSecret}, "description": {"Walk-in payment"}, "direction": {"in"}, "amount_minor": {"125"}, "currency": {"USD"}, "payment_method": {"cash"}, "occurred_on": {"2026-01-02"}, "note": {"Walk-in payment"}, "client_id": {client.ID.String()}}
	response := request(h.Routes(), http.MethodPost, path+"?direction=in&page=2", form, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", response.Code, response.Body)
	}
	if location := response.Header().Get("Location"); !strings.Contains(location, "direction=in&page=2&saved=1#transactions-entries-heading") {
		t.Fatalf("success location: %s", location)
	}
	page := request(h.Routes(), http.MethodGet, path+"?saved=1", nil, cookies, nil)
	for _, value := range []string{"Walk-in payment", "USD 1.25", "2026-01-02", "Cash", "Marina Ortega", "Alex Rivera", "Entry recorded."} {
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), value) {
			t.Fatalf("list missing %q: %d %s", value, page.Code, page.Body)
		}
	}
	summaryStart := strings.Index(page.Body.String(), `class="list-row`)
	if summaryStart < 0 {
		t.Fatal("transactions list row missing")
	}
	summary := strings.SplitN(page.Body.String()[summaryStart:], "<details", 2)[0]
	if !strings.Contains(summary, "Walk-in payment") || !strings.Contains(summary, "Marina Ortega") {
		t.Fatalf("entry summary must show note and client: %s", summary)
	}
	if !strings.Contains(page.Body.String(), `data-transactions-dialog`) || !strings.Contains(page.Body.String(), `data-transactions-create`) {
		t.Fatal("transactions must offer a record dialog with a full-page fallback")
	}
	if !strings.Contains(page.Body.String(), `class="list-actions-cell"`) || !strings.Contains(page.Body.String(), `name="correction_action" value="refund:`) || !strings.Contains(page.Body.String(), `action=void`) {
		t.Fatal("row menu must offer refund and void actions")
	}
	for _, target := range []string{"/settings/transactions/methods", "/settings/transactions/categories", "/settings/transactions"} {
		if !strings.Contains(page.Body.String(), target) {
			t.Fatalf("missing transactions action %s", target)
		}
	}
	columnForm := url.Values{"csrf": {session.CSRFSecret}, "visible": {"false"}}
	columnFragment := request(h.Routes(), http.MethodPost, path+"/preferences/columns/note?part=results", columnForm, cookies, map[string]string{"HX-Request": "true"})
	if columnFragment.Code != http.StatusOK || !strings.Contains(columnFragment.Body.String(), `id="list-results"`) ||
		!strings.Contains(columnFragment.Body.String(), `id="list-column-switches"`) || !strings.Contains(columnFragment.Body.String(), `hx-swap-oob="true"`) ||
		strings.Contains(columnFragment.Body.String(), `data-column="note"`) || strings.Contains(columnFragment.Body.String(), "<html") {
		t.Fatalf("column change fragment: %d %s", columnFragment.Code, columnFragment.Body)
	}
	resetColumns := request(h.Routes(), http.MethodPost, path+"/preferences/columns/reset", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if resetColumns.Code != http.StatusSeeOther {
		t.Fatalf("column reset without JavaScript: %d %s", resetColumns.Code, resetColumns.Body)
	}
	recordPage := request(h.Routes(), http.MethodGet, path+"?view=record", nil, cookies, nil)
	if !strings.Contains(recordPage.Body.String(), `id="transactions-create-form"`) || !strings.Contains(recordPage.Body.String(), `data-show-when="transactions-direction=out"`) || !strings.Contains(recordPage.Body.String(), `<noscript>`) {
		t.Fatal("record form must enhance category visibility and retain its no-JS fallback")
	}
	if !strings.Contains(recordPage.Body.String(), `class="transaction-entry-form`) || !strings.Contains(recordPage.Body.String(), `for="transactions-description"`) {
		t.Fatal("creation form must retain visible labels and shared detail layout")
	}
	if !strings.Contains(recordPage.Body.String(), `list="transactions-currencies"`) || !strings.Contains(recordPage.Body.String(), `value="USD" label="US dollar (USD)"`) || !strings.Contains(recordPage.Body.String(), `id="transactions-currency-help"`) {
		t.Fatal("currency suggestions or arbitrary-code guidance missing")
	}
	directionPage := request(h.Routes(), http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "direction": {"out"}, "picker_action": {"direction"}, "note": {"Preserved draft"}}, cookies, nil)
	if !strings.Contains(directionPage.Body.String(), `id="transactions-category"`) || !strings.Contains(directionPage.Body.String(), "Preserved draft") {
		t.Fatal("direction update should reveal category and preserve draft")
	}
	localeCookies := append(append([]*http.Cookie{}, cookies...), &http.Cookie{Name: "locale", Value: "es"})
	localeDraft := url.Values{"csrf": {session.CSRFSecret}, "direction": {"in"}, "amount": {"1,234"}, "amount_mode": {"decimal"}, "amount_locale": {"en"}, "currency": {"USD"}, "picker_action": {"search"}}
	localePicker := request(h.Routes(), http.MethodPost, path, localeDraft, localeCookies, nil)
	if !strings.Contains(localePicker.Body.String(), `name="amount_locale" value="en"`) || !strings.Contains(localePicker.Body.String(), `value="1,234"`) {
		t.Fatalf("picker must preserve amount and its locale: %d %s", localePicker.Code, localePicker.Body)
	}
	localeDraft.Del("picker_action")
	localeError := request(h.Routes(), http.MethodPost, path, localeDraft, localeCookies, nil)
	if localeError.Code != http.StatusUnprocessableEntity || !strings.Contains(localeError.Body.String(), `name="amount_locale" value="en"`) || !strings.Contains(localeError.Body.String(), `value="1,234"`) {
		t.Fatalf("validation must preserve amount and its locale: %d %s", localeError.Code, localeError.Body)
	}
	unknown := url.Values{"csrf": {session.CSRFSecret}, "direction": {"in"}, "amount": {"125"}, "amount_mode": {"decimal"}, "amount_locale": {"en"}, "currency": {"ZZZ"}, "payment_method": {"cash"}}
	unknownResult := request(h.Routes(), http.MethodPost, path, unknown, cookies, nil)
	if unknownResult.Code != http.StatusUnprocessableEntity || !strings.Contains(unknownResult.Body.String(), "This currency is not supported for decimal amounts") || !strings.Contains(unknownResult.Body.String(), `aria-describedby="transactions-amount-error"`) {
		t.Fatalf("unsupported currency must explain the explicit minor-unit fallback: %d %s", unknownResult.Code, unknownResult.Body)
	}
	for _, tc := range []struct{ amount, mode, locale, message string }{
		{"bad", "decimal", "en", "Enter a positive amount with no more decimal places than this currency allows."},
		{"1.234", "decimal", "en", "Enter a positive amount with no more decimal places than this currency allows."},
		{"0", "decimal", "en", "Enter a positive amount with no more decimal places than this currency allows."},
		{"1.5", "minor", "en", "Enter a positive whole number of minor units."},
		{"mal", "decimal", "es", "Introduce un importe positivo sin superar los decimales permitidos para esta moneda."},
	} {
		form := url.Values{"csrf": {session.CSRFSecret}, "direction": {"in"}, "amount": {tc.amount}, "amount_mode": {tc.mode}, "amount_locale": {tc.locale}, "currency": {"USD"}, "payment_method": {"cash"}, "note": {"Amount draft"}}
		locale := append(append([]*http.Cookie{}, cookies...), &http.Cookie{Name: "locale", Value: tc.locale})
		result := request(h.Routes(), http.MethodPost, path, form, locale, nil)
		if result.Code != http.StatusUnprocessableEntity || !strings.Contains(result.Body.String(), tc.message) || !strings.Contains(result.Body.String(), `aria-invalid="true"`) || !strings.Contains(result.Body.String(), `aria-describedby="transactions-amount-error"`) || !strings.Contains(result.Body.String(), `value="`+tc.amount+`"`) || !strings.Contains(result.Body.String(), "Amount draft") {
			t.Fatalf("amount %q (%s): %d %s", tc.amount, tc.locale, result.Code, result.Body)
		}
	}
	entries, err := h.transactions.List(ctx, actor, workspace, transactions.ListFilter{Page: 1})
	if err != nil || len(entries) != 1 {
		t.Fatalf("list before correction: %+v %v", entries, err)
	}
	selectRefund := url.Values{"csrf": {session.CSRFSecret}, "entry_page": {"1"}, "correction_action": {"refund:" + entries[0].ID.String()}, "note": {"Retain this draft"}, "amount_minor": {"50"}}
	selected := request(h.Routes(), http.MethodPost, path, selectRefund, cookies, nil)
	for _, text := range []string{"Retain this draft", `value="50"`, `value="` + entries[0].ID.String() + `"`, "autofocus"} {
		if selected.Code != http.StatusOK || !strings.Contains(selected.Body.String(), text) {
			t.Fatalf("refund selection missing %q: %d %s", text, selected.Code, selected.Body)
		}
	}
	if !strings.Contains(selected.Body.String(), `value="`+client.ID.String()+`" selected>Marina Ortega</option>`) {
		t.Fatalf("refund must preselect receipt client: %s", selected.Body)
	}
	selectRefund.Del("correction_action")
	selectRefund.Set("picker_action", "search")
	selectRefund.Set("client_id", client.ID.String())
	selectRefund.Set("client_search", "Marina")
	selected = request(h.Routes(), http.MethodPost, path, selectRefund, cookies, nil)
	if selected.Code != http.StatusOK || !strings.Contains(selected.Body.String(), `value="`+client.ID.String()+`" selected>Marina Ortega</option>`) {
		t.Fatalf("refund client after picker search: %d %s", selected.Code, selected.Body)
	}
	otherClient, err := h.clients.Create(ctx, actor, workspace, clients.Input{DisplayName: "Another client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	selectRefund.Set("client_id", otherClient.ID.String())
	selected = request(h.Routes(), http.MethodPost, path, selectRefund, cookies, nil)
	if selected.Code != http.StatusOK || !strings.Contains(selected.Body.String(), `value="`+otherClient.ID.String()+`" selected>Another client</option>`) {
		t.Fatalf("changed refund client: %d %s", selected.Code, selected.Body)
	}
	refund := url.Values{"csrf": {session.CSRFSecret}, "description": {"Refund"}, "direction": {"out"}, "amount_minor": {"126"}, "currency": {"USD"}, "payment_method": {"cash"}, "occurred_on": {"2026-01-02"}, "refund_of_entry_id": {entries[0].ID.String()}}
	if got := request(h.Routes(), http.MethodPost, path, refund, cookies, nil); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), "At most USD 1.25") || !strings.Contains(got.Body.String(), `aria-describedby="transactions-amount-error"`) {
		t.Fatalf("over-refund: %d %s", got.Code, got.Body)
	}
	refund.Set("client_id", client.ID.String())
	refund.Set("note", "Client refund history")
	refund.Set("amount_minor", "25")
	if got := request(h.Routes(), http.MethodPost, path, refund, cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("refund: %d %s", got.Code, got.Body)
	}
	refundEntries, err := h.transactions.List(ctx, actor, workspace, transactions.ListFilter{Page: 1})
	if err != nil || len(refundEntries) != 2 {
		t.Fatalf("refund list: %+v %v", refundEntries, err)
	}
	if refundEntries[0].ClientID == nil || *refundEntries[0].ClientID != client.ID {
		t.Fatalf("refund client: %+v", refundEntries[0].ClientID)
	}
	history := request(h.Routes(), http.MethodGet, "/workspaces/"+workspace.String()+"/clients/"+client.ID.String(), nil, cookies, nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), "Client refund history") {
		t.Fatalf("refund in client history: %d %s", history.Code, history.Body)
	}
	receiptPage := request(h.Routes(), http.MethodGet, path+"?direction=in&details="+entries[0].ID.String(), nil, cookies, nil)
	refundLink := `?refund_of_entry_id=` + entries[0].ID.String() + `#transactions-entries-heading`
	if receiptPage.Code != http.StatusOK || !strings.Contains(receiptPage.Body.String(), refundLink) || !strings.Contains(receiptPage.Body.String(), "Linked refunds (1)") || strings.Contains(receiptPage.Body.String(), "<dd>"+refundEntries[0].ID.String()+"</dd>") {
		t.Fatalf("receipt refund link outside filter: %d %s", receiptPage.Code, receiptPage.Body)
	}
	detailQuery := url.Values{"direction": {"in"}, "details": {entries[0].ID.String()}, "page": {"1"}}
	detailPage := request(h.Routes(), http.MethodGet, path+"?"+detailQuery.Encode(), nil, cookies, nil)
	if detailPage.Code != http.StatusOK || !strings.Contains(detailPage.Body.String(), `id="transactions-entry-details"`) ||
		!strings.Contains(detailPage.Body.String(), `value="refund:`+entries[0].ID.String()+`"`) ||
		!strings.Contains(detailPage.Body.String(), url.QueryEscape("direction:equals:in")) {
		t.Fatalf("entry details and return context: %d %s", detailPage.Code, detailPage.Body)
	}
	if !strings.Contains(detailPage.Body.String(), `class="modal-box detail-main`) || !strings.Contains(detailPage.Body.String(), `class="detail-values transaction-detail-values`) || !strings.Contains(detailPage.Body.String(), `data-transactions-details-close aria-label="Close"`) || !strings.Contains(detailPage.Body.String(), `aria-hidden="true">×</span>`) {
		t.Fatal("detail must retain responsive labels and an accessible close link without JavaScript")
	}
	linked := request(h.Routes(), http.MethodGet, path+"?refund_of_entry_id="+entries[0].ID.String()+"&details="+refundEntries[0].ID.String(), nil, cookies, nil)
	if linked.Code != http.StatusOK || !strings.Contains(linked.Body.String(), "<dd>"+refundEntries[0].ID.String()+"</dd>") || strings.Contains(linked.Body.String(), "<dt>Entry ID</dt><dd>"+entries[0].ID.String()+"</dd>") {
		t.Fatalf("linked refunds: %d %s", linked.Code, linked.Body)
	}
	if !strings.Contains(linked.Body.String(), "?entry_id="+entries[0].ID.String()+"#transactions-entries-heading") || !strings.Contains(linked.Body.String(), "USD 1.25") {
		t.Fatalf("refund must link to its original outside this filter: %s", linked.Body)
	}
	voidPath := path + "/entries/" + entries[0].ID.String() + "/void"
	if got := request(h.Routes(), http.MethodPost, voidPath, url.Values{"csrf": {session.CSRFSecret}, "reason": {"Keep this reason"}}, cookies, nil); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), "Confirm that you want to void") {
		t.Fatalf("unconfirmed void: %d %s", got.Code, got.Body)
	}
	if got := request(h.Routes(), http.MethodPost, voidPath, url.Values{"csrf": {session.CSRFSecret}, "reason": {"Keep this reason"}, "confirm_void": {"yes"}}, cookies, nil); got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), "Void this receipt&#39;s active refunds first") || !strings.Contains(got.Body.String(), "Keep this reason") || !strings.Contains(got.Body.String(), time.Now().UTC().Format("2006-01-02")) || !strings.Contains(got.Body.String(), `aria-describedby="transactions-void-error-`+entries[0].ID.String()+`-dialog"`) || !strings.Contains(got.Body.String(), "autofocus") {
		t.Fatalf("blocked void: %d %s", got.Code, got.Body)
	}
	if got := request(h.Routes(), http.MethodPost, path+"/entries/"+refundEntries[0].ID.String()+"/void", url.Values{"csrf": {session.CSRFSecret}, "confirm_void": {"yes"}}, cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("void refund: %d %s", got.Code, got.Body)
	}
	voidedRefund := request(h.Routes(), http.MethodGet, path+"?refund_of_entry_id="+entries[0].ID.String()+"&details="+refundEntries[0].ID.String(), nil, cookies, nil)
	if voidedRefund.Code != http.StatusOK || !strings.Contains(voidedRefund.Body.String(), "Voided at") || !strings.Contains(voidedRefund.Body.String(), refundEntries[0].ID.String()) {
		t.Fatalf("voided refund history: %d %s", voidedRefund.Code, voidedRefund.Body)
	}
	if got := request(h.Routes(), http.MethodPost, voidPath+"?direction=in&page=2", url.Values{"csrf": {session.CSRFSecret}, "reason": {"Correction"}, "confirm_void": {"yes"}}, cookies, nil); got.Code != http.StatusSeeOther || !strings.Contains(got.Header().Get("Location"), "direction=in&page=2&voided=1") {
		t.Fatalf("void: %d %s", got.Code, got.Body)
	}
	page = request(h.Routes(), http.MethodGet, path+"?voided=1&details="+entries[0].ID.String(), nil, cookies, nil)
	for _, text := range []string{"Walk-in payment", "Correction", entries[0].ID.String(), "Entry voided.", "<dt>Voided by</dt><dd>"} {
		if !strings.Contains(page.Body.String(), text) {
			t.Fatalf("correction history missing %q", text)
		}
	}
	if !strings.Contains(page.Body.String(), "<dt>Voided by</dt><dd>Alex Rivera</dd>") && !strings.Contains(page.Body.String(), "<dt>Voided by</dt><dd>owner@example.com</dd>") {
		t.Fatal("correction history must identify the member who voided the entry")
	}
	selection := url.Values{"csrf": {session.CSRFSecret}, "entry_page": {"1"}, "correction_action": {"replace:" + entries[0].ID.String()}, "note": {"Correction draft"}}
	selected = request(h.Routes(), http.MethodPost, path, selection, cookies, nil)
	if selected.Code != http.StatusOK || !strings.Contains(selected.Body.String(), "Correction draft") || !strings.Contains(selected.Body.String(), `value="`+entries[0].ID.String()+`"`) {
		t.Fatalf("replacement selection: %d %s", selected.Code, selected.Body)
	}
	second := request(h.Routes(), http.MethodGet, path+"?page=2", nil, cookies, nil)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "?page=1#transactions-entries-heading") {
		t.Fatalf("previous page: %d %s", second.Code, second.Body)
	}
	badPage := request(h.Routes(), http.MethodGet, path+"?page=0", nil, cookies, nil)
	if badPage.Code != http.StatusBadRequest {
		t.Fatalf("invalid page: %d", badPage.Code)
	}
	form.Set("amount_minor", "0")
	form.Set("note", "Keep this draft")
	invalid := request(h.Routes(), http.MethodPost, path, form, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Keep this draft") || !strings.Contains(invalid.Body.String(), `role="alert"`) {
		t.Fatalf("invalid: %d %s", invalid.Code, invalid.Body)
	}
	form.Set("amount_minor", "125")
	form.Set("currency", "XX")
	form.Set("description", `<script>alert("x")</script>`)
	escaped := request(h.Routes(), http.MethodPost, path, form, cookies, nil)
	if escaped.Code != http.StatusUnprocessableEntity || strings.Contains(escaped.Body.String(), `<script>alert("x")</script>`) || !strings.Contains(escaped.Body.String(), `&lt;script&gt;`) {
		t.Fatalf("description draft must be escaped: %d %s", escaped.Code, escaped.Body)
	}
	form.Set("description", "Walk-in payment")
	form.Set("currency", "XX")
	invalid = request(h.Routes(), http.MethodPost, path, form, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Enter a three-letter currency code") || !strings.Contains(invalid.Body.String(), `aria-describedby="transactions-currency-error"`) {
		t.Fatalf("invalid currency: %d %s", invalid.Code, invalid.Body)
	}
	form.Set("picker_action", "search")
	form.Set("client_search", "Marina")
	searched := request(h.Routes(), http.MethodPost, path, form, cookies, nil)
	if searched.Code != http.StatusOK || !strings.Contains(searched.Body.String(), "Marina Ortega") || !strings.Contains(searched.Body.String(), "Keep this draft") {
		t.Fatalf("client search: %d %s", searched.Code, searched.Body)
	}
	if !strings.Contains(searched.Body.String(), `autofocus`) {
		t.Fatal("client search must restore focus without JavaScript")
	}
	if strings.Contains(page.Body.String(), `autofocus`) || strings.Contains(invalid.Body.String(), `autofocus`) {
		t.Fatal("ordinary page and validation errors must not autofocus the client selector")
	}
	category, err := h.transactions.ChangeCategory(ctx, actor, workspace, 0, "Office", false)
	if err != nil {
		t.Fatal(err)
	}
	categoryPage := request(h.Routes(), http.MethodGet, "/workspaces/"+workspace.String()+"/settings/transactions/categories?category_saved=1", nil, cookies, nil)
	for _, marker := range []string{`id="transactions-categories-heading"`, "Expense category saved.", "Rename category: Office", "Archive category: Office", "Category name: Office"} {
		if !strings.Contains(categoryPage.Body.String(), marker) {
			t.Fatalf("category page missing %q", marker)
		}
	}
	conflictForm := url.Values{"csrf": {session.CSRFSecret}, "description": {"Conflict"}, "direction": {"in"}, "amount_minor": {"125"}, "currency": {"USD"}, "payment_method": {"cash"}, "category_id": {category.ID.String()}}
	conflict := request(h.Routes(), http.MethodPost, path, conflictForm, cookies, nil)
	if conflict.Code != http.StatusUnprocessableEntity || !strings.Contains(conflict.Body.String(), "Remove the category or choose money out.") || !strings.Contains(conflict.Body.String(), `aria-invalid="true"`) || !strings.Contains(conflict.Body.String(), `value="`+category.ID.String()+`" selected`) {
		t.Fatalf("category direction conflict: %d %s", conflict.Code, conflict.Body)
	}
	changedDirection := url.Values{"csrf": {session.CSRFSecret}, "direction": {"in"}, "category_id": {category.ID.String()}, "picker_action": {"direction"}}
	directionConflict := request(h.Routes(), http.MethodPost, path, changedDirection, cookies, nil)
	if !strings.Contains(directionConflict.Body.String(), `value="`+category.ID.String()+`" selected`) || !strings.Contains(directionConflict.Body.String(), `aria-invalid="true"`) {
		t.Fatalf("direction update must retain category conflict: %s", directionConflict.Body)
	}
	expense := url.Values{"csrf": {session.CSRFSecret}, "description": {"Office supplies"}, "direction": {"out"}, "amount_minor": {"25"}, "currency": {"USD"}, "payment_method": {"cash"}, "occurred_on": {"2026-01-02"}, "category_id": {category.ID.String()}}
	expense.Set("client_id", client.ID.String())
	expense.Set("note", "Office supplies purchase")
	expense.Set("picker_action", "search")
	expense.Set("client_search", "Marina")
	picked := request(h.Routes(), http.MethodPost, path, expense, cookies, nil)
	if picked.Code != http.StatusOK || !strings.Contains(picked.Body.String(), `value="`+category.ID.String()+`" selected`) {
		t.Fatalf("category selection after client search: %d %s", picked.Code, picked.Body)
	}
	expense.Del("picker_action")
	if got := request(h.Routes(), http.MethodPost, path, expense, cookies, nil); got.Code != http.StatusSeeOther {
		t.Fatalf("categorized expense: %d %s", got.Code, got.Body)
	}
	expensePage := request(h.Routes(), http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(expensePage.Body.String(), `<td data-column="category">Office</td>`) {
		t.Fatalf("categorized expense history: %s", expensePage.Body)
	}
	expenseEntries, err := h.transactions.List(ctx, actor, workspace, transactions.ListFilter{Page: 1, CategoryID: category.ID})
	if err != nil || len(expenseEntries) != 1 {
		t.Fatalf("expense lookup: %+v %v", expenseEntries, err)
	}
	expenseID := expenseEntries[0].ID.String()
	voidExpense := request(h.Routes(), http.MethodPost, path+"/entries/"+expenseID+"/void", url.Values{"csrf": {session.CSRFSecret}, "confirm_void": {"yes"}}, cookies, nil)
	if voidExpense.Code != http.StatusSeeOther {
		t.Fatalf("void expense: %d %s", voidExpense.Code, voidExpense.Body)
	}
	replaceExpense := request(h.Routes(), http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "entry_page": {"1"}, "correction_action": {"replace:" + expenseID}}, cookies, nil)
	if replaceExpense.Code != http.StatusOK || !strings.Contains(replaceExpense.Body.String(), `value="`+client.ID.String()+`" selected>Marina Ortega</option>`) || !strings.Contains(replaceExpense.Body.String(), `value="`+category.ID.String()+`" selected`) || !strings.Contains(replaceExpense.Body.String(), "USD 0.25") {
		t.Fatalf("replacement must retain client, category, and source: %d %s", replaceExpense.Code, replaceExpense.Body)
	}
	replacement := url.Values{"csrf": {session.CSRFSecret}, "description": {"Replacement"}, "direction": {"out"}, "amount": {"0.25"}, "amount_mode": {"decimal"}, "amount_locale": {"en"}, "currency": {" usd "}, "payment_method": {"cash"}, "client_id": {client.ID.String()}, "category_id": {category.ID.String()}, "replaces_entry_id": {expenseID}}
	if result := request(h.Routes(), http.MethodPost, path, replacement, cookies, nil); result.Code != http.StatusSeeOther {
		t.Fatalf("replace categorized expense: %d %s", result.Code, result.Body)
	}
	replacedEntries, err := h.transactions.List(ctx, actor, workspace, transactions.ListFilter{Page: 1, CategoryID: category.ID})
	if err != nil || len(replacedEntries) != 2 || replacedEntries[0].ReplacesEntryID != expenseEntries[0].ID || replacedEntries[0].ClientID == nil || *replacedEntries[0].ClientID != client.ID || replacedEntries[0].CategoryID == nil || *replacedEntries[0].CategoryID != category.ID {
		t.Fatalf("replacement persisted fields: %+v %v", replacedEntries, err)
	}
	replacementPage := request(h.Routes(), http.MethodGet, path+"?category_id="+category.ID.String()+"&details="+replacedEntries[0].ID.String(), nil, cookies, nil)
	if !strings.Contains(replacementPage.Body.String(), "?entry_id="+expenseID+"#transactions-entries-heading") || !strings.Contains(replacementPage.Body.String(), "USD 0.25") {
		t.Fatalf("replacement source link: %d %s", replacementPage.Code, replacementPage.Body)
	}
	sourceLinkStart := strings.Index(replacementPage.Body.String(), "?entry_id="+expenseID+"#transactions-entries-heading")
	sourceLink := strings.SplitN(replacementPage.Body.String()[sourceLinkStart:], "</a>", 2)[0]
	if !strings.Contains(sourceLink, "Office supplies purchase") || !strings.Contains(sourceLink, "Marina Ortega") {
		t.Fatalf("source link must distinguish the original: %s", sourceLink)
	}
	sourcePage := request(h.Routes(), http.MethodGet, path+"?entry_id="+expenseID+"&details="+expenseID, nil, cookies, nil)
	if sourcePage.Code != http.StatusOK || !strings.Contains(sourcePage.Body.String(), "<dd>"+expenseID+"</dd>") {
		t.Fatalf("source navigation: %d %s", sourcePage.Code, sourcePage.Body)
	}
	if !strings.Contains(sourcePage.Body.String(), `class="list-filter-chip-edit`) || !strings.Contains(sourcePage.Body.String(), "Entry ID") || !strings.Contains(sourcePage.Body.String(), expenseID) {
		t.Fatal("source entry filter capsule must be visible")
	}
	newEntry := url.Values{"csrf": {session.CSRFSecret}, "description": {"Recorded from source"}, "direction": {"in"}, "amount": {"1.00"}, "amount_mode": {"decimal"}, "amount_locale": {"en"}, "currency": {"USD"}, "payment_method": {"cash"}, "note": {"Recorded from source"}}
	newResult := request(h.Routes(), http.MethodPost, path+"?entry_id="+expenseID, newEntry, cookies, nil)
	if newResult.Code != http.StatusSeeOther || strings.Contains(newResult.Header().Get("Location"), "entry_id=") {
		t.Fatalf("creation must clear source filter: %d %s", newResult.Code, newResult.Header().Get("Location"))
	}
	returnPage := request(h.Routes(), http.MethodGet, newResult.Header().Get("Location"), nil, cookies, nil)
	if !strings.Contains(returnPage.Body.String(), "Recorded from source") {
		t.Fatalf("new entry hidden after creation: %s", returnPage.Body)
	}
	newEntry.Del("description")
	if result := request(h.Routes(), http.MethodPost, path, newEntry, cookies, nil); result.Code != http.StatusUnprocessableEntity || !strings.Contains(result.Body.String(), "Recorded from source") || !strings.Contains(result.Body.String(), `aria-describedby="transactions-description-error"`) {
		t.Fatalf("missing description must retain draft and show field error: %d %s", result.Code, result.Body)
	}
	duplicate := request(h.Routes(), http.MethodPost, path+"/categories", url.Values{"csrf": {session.CSRFSecret}, "name": {"Office"}}, cookies, nil)
	if duplicate.Code != http.StatusUnprocessableEntity || !strings.Contains(duplicate.Body.String(), "An active expense category with this name already exists.") || !strings.Contains(duplicate.Body.String(), `value="Office"`) {
		t.Fatalf("duplicate category: %d %s", duplicate.Code, duplicate.Body)
	}
	later, err := h.transactions.ChangeCategory(ctx, actor, workspace, 0, "Travel", false)
	if err != nil {
		t.Fatal(err)
	}
	duplicateRename := request(h.Routes(), http.MethodPost, path+"/categories/"+later.ID.String(), url.Values{"csrf": {session.CSRFSecret}, "name": {"Office"}, "action": {"rename"}}, cookies, nil)
	body := duplicateRename.Body.String()
	formStart := strings.Index(body, `action="`+path+`/categories/`+later.ID.String()+`"`)
	if formStart < 0 {
		t.Fatal("missing category edit form")
	}
	if duplicateRename.Code != http.StatusUnprocessableEntity || !strings.Contains(body[formStart:], `aria-describedby="transactions-category-error"`) || !strings.Contains(body, `id="transactions-category-error"`) {
		t.Fatalf("rename error not associated with its control: %d %s", duplicateRename.Code, body)
	}
	for _, tc := range []struct{ suffix, name, fieldID string }{
		{"/categories", strings.Repeat("x", 65), "transactions-new-category"},
		{"/categories/" + category.ID.String(), strings.Repeat("y", 65), "transactions-category-" + category.ID.String()},
	} {
		form := url.Values{"csrf": {session.CSRFSecret}, "name": {tc.name}, "action": {"rename"}}
		response := request(h.Routes(), http.MethodPost, path+tc.suffix, form, cookies, nil)
		body := response.Body.String()
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(body, `value="`+tc.name+`"`) ||
			!strings.Contains(body, `id="transactions-category-error" role="alert" tabindex="-1"`) ||
			!strings.Contains(body, `aria-describedby="transactions-category-error"`) ||
			strings.Index(body, `id="transactions-category-error"`) < strings.Index(body, `id="transactions-categories-heading"`) ||
			!strings.Contains(body, `id="`+tc.fieldID+`"`) {
			t.Fatalf("category validation: %d %s", response.Code, body)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE transactions_entries SET description = '' WHERE workspace_id = $1 AND id = $2`, workspace, entries[0].ID); err != nil {
		t.Fatal(err)
	}
	legacy := request(h.Routes(), http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(legacy.Body.String(), `data-column="description"`) || !strings.Contains(legacy.Body.String(), "No description</a>") || !strings.Contains(legacy.Body.String(), "details="+entries[0].ID.String()) {
		t.Fatalf("legacy entry must have a navigable fallback: %d %s", legacy.Code, legacy.Body)
	}
	legacySpanish := request(h.Routes(), http.MethodGet, path, nil, localeCookies, nil)
	if !strings.Contains(legacySpanish.Body.String(), "Sin descripción</a>") {
		t.Fatalf("legacy fallback must be localized: %s", legacySpanish.Body)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_members SET role = 'member' WHERE workspace_id = $1 AND user_id = $2`, workspace, owner.UserID); err != nil {
		t.Fatal(err)
	}
	memberPage := request(h.Routes(), http.MethodGet, path+"?view=record", nil, cookies, nil)
	if memberPage.Code != http.StatusOK || strings.Contains(memberPage.Body.String(), `name="replaces_entry_id"`) || !strings.Contains(memberPage.Body.String(), `name="refund_of_entry_id"`) {
		t.Fatalf("member correction controls: %d %s", memberPage.Code, memberPage.Body)
	}
}

func TestTransactionsPaymentMethodsWeb(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableTransactions(transactions.NewService(db, auth))
	h.clients = clients.NewService(db, config.SQLite, auth, 10)
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	ownerSession, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	ownerCookies := []*http.Cookie{{Name: "session", Value: ownerSession.Secret}, {Name: "csrf", Value: ownerSession.CSRFSecret}}
	path := "/workspaces/" + workspace.String() + "/transactions"
	routes := h.Routes()
	save := func(id, name string, enabled bool) {
		t.Helper()
		form := url.Values{"csrf": {ownerSession.CSRFSecret}, "id": {id}, "name": {name}}
		if enabled {
			form.Set("enabled", "true")
		}
		if r := request(routes, http.MethodPost, path+"/methods", form, ownerCookies, nil); r.Code != http.StatusSeeOther {
			t.Fatalf("save method: %d %s", r.Code, r.Body)
		}
	}
	save("", "Voucher", true)
	page := request(routes, http.MethodGet, path+"?view=record", nil, ownerCookies, nil)
	match := regexp.MustCompile(`value="([^"]+)">Voucher</option>`).FindStringSubmatch(page.Body.String())
	if len(match) != 2 {
		t.Fatalf("created method missing: %d %s", page.Code, page.Body)
	}
	methodID := match[1]
	entry := url.Values{"csrf": {ownerSession.CSRFSecret}, "description": {"Voucher payment"}, "direction": {"in"}, "amount_minor": {"125"}, "currency": {"USD"}, "payment_method": {methodID}, "occurred_on": {"2026-01-02"}}
	if r := request(routes, http.MethodPost, path, entry, ownerCookies, nil); r.Code != http.StatusSeeOther {
		t.Fatalf("create entry: %d %s", r.Code, r.Body)
	}
	cashEntry := url.Values{"csrf": {ownerSession.CSRFSecret}, "description": {"Cash-only entry"}, "direction": {"in"}, "amount_minor": {"50"}, "currency": {"USD"}, "payment_method": {"cash"}, "occurred_on": {"2026-01-02"}, "note": {"Cash-only entry"}}
	if r := request(routes, http.MethodPost, path, cashEntry, ownerCookies, nil); r.Code != http.StatusSeeOther {
		t.Fatalf("create cash entry: %d %s", r.Code, r.Body)
	}
	save(methodID, "Gift certificate", true)
	page = request(routes, http.MethodGet, path, nil, ownerCookies, nil)
	if !strings.Contains(page.Body.String(), `<td data-column="method">Gift certificate</td>`) {
		t.Fatalf("renamed method missing: %s", page.Body)
	}
	save(methodID, "Gift certificate", false)
	managementPage := request(routes, http.MethodGet, "/workspaces/"+workspace.String()+"/settings/transactions/methods", nil, ownerCookies, nil)
	if managementPage.Code != http.StatusOK || !strings.Contains(managementPage.Body.String(), "Gift certificate") || !strings.Contains(managementPage.Body.String(), "Disabled") {
		t.Fatalf("disabled method status: %d %s", managementPage.Code, managementPage.Body)
	}
	for _, role := range []identity.Role{identity.Admin, identity.Member} {
		inv, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		user, _, err := auth.CreateInvitedLocalUser(ctx, inv.Secret, "long-enough-password", identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		session, err := auth.CreateSession(ctx, user, "local", "", time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
		page := request(routes, http.MethodGet, path, nil, cookies, nil)
		body := page.Body.String()
		createBody := request(routes, http.MethodGet, path+"?view=record", nil, cookies, nil).Body.String()
		createStart := strings.Index(createBody, `id="transactions-create-form"`)
		if createStart < 0 {
			t.Fatal("missing create form")
		}
		createForm := strings.SplitN(createBody[createStart:], "</form>", 2)[0]
		if page.Code != http.StatusOK || !strings.Contains(body, `<td data-column="method">Gift certificate</td>`) || strings.Contains(createForm, `value="`+methodID+`">Gift certificate</option>`) {
			t.Fatalf("%s disabled method: %d %s", role, page.Code, body)
		}
		filterBody := request(routes, http.MethodGet, path+"?filter_field=payment_method", nil, cookies, nil).Body.String()
		filterStart := strings.Index(filterBody, `id="list-filter-value"`)
		if filterStart < 0 || !strings.Contains(strings.SplitN(filterBody[filterStart:], "</select>", 2)[0], `value="`+methodID+`">Gift certificate</option>`) {
			t.Fatalf("%s cannot filter by disabled historical method: %s", role, body)
		}
		filtered := request(routes, http.MethodGet, path+"?payment_method="+url.QueryEscape(methodID), nil, cookies, nil)
		if filtered.Code != http.StatusOK || !strings.Contains(filtered.Body.String(), `<td data-column="method">Gift certificate</td>`) || !strings.Contains(filtered.Body.String(), "USD 1.25") || strings.Contains(filtered.Body.String(), "Cash-only entry") {
			t.Fatalf("%s filtered historical method: %d %s", role, filtered.Code, filtered.Body)
		}
		management := request(routes, http.MethodGet, path, nil, cookies, nil).Body.String()
		if strings.Contains(management, "/settings/transactions/methods") != (role == identity.Admin) {
			t.Fatalf("%s management visibility", role)
		}
		result := request(routes, http.MethodPost, path+"/methods", url.Values{"csrf": {session.CSRFSecret}, "id": {methodID}, "name": {"Gift certificate"}, "enabled": {"true"}}, cookies, nil)
		want := http.StatusForbidden
		if role == identity.Admin {
			want = http.StatusSeeOther
		}
		if result.Code != want {
			t.Fatalf("%s save: %d", role, result.Code)
		}
		if role == identity.Admin {
			save(methodID, "Gift certificate", false)
		}
	}
	save(methodID, "Gift certificate", true)
	page = request(routes, http.MethodGet, path+"?view=record", nil, ownerCookies, nil)
	if !strings.Contains(page.Body.String(), `value="`+methodID+`">Gift certificate</option>`) {
		t.Fatalf("reenabled method missing: %s", page.Body)
	}
	invalid := request(routes, http.MethodPost, path+"/methods", url.Values{"csrf": {ownerSession.CSRFSecret}, "name": {"Gift certificate"}, "enabled": {"true"}}, ownerCookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "unique payment method name") {
		t.Fatalf("duplicate method: %d %s", invalid.Code, invalid.Body)
	}
	if !strings.Contains(invalid.Body.String(), `aria-describedby="transactions-method-error"`) || !strings.Contains(invalid.Body.String(), `aria-label="Save payment method — Gift certificate"`) {
		t.Fatalf("method error focus, date, or accessible name missing: %s", invalid.Body)
	}
	for _, id := range []string{"check", "transfer", "cash", "card"} {
		save(id, id, false)
	}
	last := request(routes, http.MethodPost, path+"/methods", url.Values{"csrf": {ownerSession.CSRFSecret}, "id": {methodID}, "name": {"Gift certificate"}}, ownerCookies, nil)
	if last.Code != http.StatusUnprocessableEntity || !strings.Contains(last.Body.String(), "Enable another payment method before disabling this one.") {
		t.Fatalf("last method: %d %s", last.Code, last.Body)
	}
}

func TestTransactionsInputAmountFormats(t *testing.T) {
	base := transactionsDraft{direction: "in", currency: "USD", amount: "1.25", amountMode: "decimal", amountLocale: "en"}
	input, err := transactionsInput(base)
	if err != nil || input.AmountMinor != 125 {
		t.Fatalf("decimal amount: %+v %v", input, err)
	}
	base.currency = " usd "
	input, err = transactionsInput(base)
	if err != nil || input.AmountMinor != 125 || input.Currency != "USD" {
		t.Fatalf("normalized currency: %+v %v", input, err)
	}
	base.currency, base.amount, base.amountLocale = "EUR", "1,25", "es"
	input, err = transactionsInput(base)
	if err != nil || input.AmountMinor != 125 {
		t.Fatalf("localized amount: %+v %v", input, err)
	}
	base.currency, base.amount = "ZZZ", "125"
	if _, err = transactionsInput(base); err == nil {
		t.Fatal("unknown currency must not assume decimal precision")
	}
	base.amountMode = "minor"
	input, err = transactionsInput(base)
	if err != nil || input.AmountMinor != 125 {
		t.Fatalf("explicit minor units: %+v %v", input, err)
	}
}
