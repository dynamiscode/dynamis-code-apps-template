package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/transactions"
)

func TestTransactionsREST(t *testing.T) {
	_, db, workspace, ownerToken := testHandler(t)
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO workspace_currencies (workspace_id, code, decimals, is_default) VALUES ($1, 'USD', 2, TRUE), ($1, 'EUR', 2, FALSE)`, identity.ParseWorkspaceIDOrZero(workspace)); err != nil {
		t.Fatal(err)
	}
	h := &handler{identity: auth, transactions: transactions.NewService(db, auth)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/transactions/entries", h.listTransactionsEntries)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/transactions/totals", h.transactionsTotals)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/transactions/entries", h.createTransactionsEntry)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/transactions/entries/{entryId}/void", h.voidTransactionsEntry)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/transactions/methods", h.listTransactionsMethods)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/transactions/categories", h.listTransactionsCategories)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/transactions/categories", h.changeTransactionsCategory)
	mux.HandleFunc("PATCH /api/v1/workspaces/{workspaceId}/transactions/categories/{categoryId}", h.changeTransactionsCategory)
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceId}/transactions/categories/{categoryId}", h.changeTransactionsCategory)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/transactions/methods", h.saveTransactionsMethod)
	mux.HandleFunc("PUT /api/v1/workspaces/{workspaceId}/transactions/methods", h.saveTransactionsMethod)
	path := "/api/v1/workspaces/" + workspace + "/transactions"

	receipt := serveAuthorized(mux, http.MethodPost, path+"/entries", `{"description":"Receipt","direction":"in","amountMinor":1200,"currency":"USD","paymentMethod":"cash","occurredOn":"2024-01-01"}`, ownerToken, nil)
	var receiptEntry struct {
		ID string `json:"id"`
	}
	if receipt.Code != http.StatusCreated || json.Unmarshal(receipt.Body.Bytes(), &receiptEntry) != nil || receiptEntry.ID == "" {
		t.Fatalf("totals receipt = %d %s", receipt.Code, receipt.Body.String())
	}
	for _, entry := range []string{
		`{"description":"Refund","direction":"out","amountMinor":200,"currency":"USD","paymentMethod":"cash","occurredOn":"2024-01-31","refundOfEntryId":"` + receiptEntry.ID + `"}`,
		`{"description":"Receipt","direction":"in","amountMinor":500,"currency":"EUR","paymentMethod":"card","occurredOn":"2024-01-15"}`,
		`{"description":"Receipt","direction":"in","amountMinor":900,"currency":"USD","paymentMethod":"cash","occurredOn":"2024-02-01"}`,
	} {
		response := serveAuthorized(mux, http.MethodPost, path+"/entries", entry, ownerToken, nil)
		if response.Code != http.StatusCreated {
			t.Fatalf("totals entry = %d %s", response.Code, response.Body.String())
		}
	}
	voidCandidate := serveAuthorized(mux, http.MethodPost, path+"/entries", `{"description":"Receipt","direction":"in","amountMinor":777,"currency":"USD","paymentMethod":"cash","occurredOn":"2024-01-15"}`, ownerToken, nil)
	var voidEntry struct {
		ID string `json:"id"`
	}
	if voidCandidate.Code != http.StatusCreated || json.Unmarshal(voidCandidate.Body.Bytes(), &voidEntry) != nil || voidEntry.ID == "" {
		t.Fatalf("totals void candidate = %d %s", voidCandidate.Code, voidCandidate.Body.String())
	}
	if voided := serveAuthorized(mux, http.MethodPost, path+"/entries/"+voidEntry.ID+"/void", `{}`, ownerToken, nil); voided.Code != http.StatusOK {
		t.Fatalf("totals void = %d %s", voided.Code, voided.Body.String())
	}
	response := serveAuthorized(mux, http.MethodGet, path+"/totals?period=custom&start=2024-01-01&end=2024-01-31", "", ownerToken, nil)
	var totals transactions.Totals
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &totals) != nil || len(totals.Currencies) != 2 {
		t.Fatalf("totals = %d %s", response.Code, response.Body.String())
	}
	if totals.Currencies[0].Currency != "EUR" || totals.Currencies[0].MoneyIn != "500" || totals.Currencies[1].Currency != "USD" || totals.Currencies[1].MoneyIn != "1200" || totals.Currencies[1].MoneyOut != "200" || totals.Currencies[1].Net != "1000" || len(totals.Currencies[1].Categories) != 1 || !totals.Currencies[1].Categories[0].Uncategorized {
		t.Fatalf("currency totals = %+v", totals.Currencies)
	}
	if response := serveAuthorized(mux, http.MethodGet, path+"/totals?period=custom&start=2024-02-01&end=2024-01-01", "", ownerToken, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid totals period = %d %s", response.Code, response.Body.String())
	}

	empty := serveAuthorized(mux, http.MethodGet, path+"/categories", "", ownerToken, nil)
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("categories = %d %s", empty.Code, empty.Body.String())
	}
	createdCategory := serveAuthorized(mux, http.MethodPost, path+"/categories", `{"name":"Rent"}`, ownerToken, nil)
	var category transactions.Category
	if createdCategory.Code != http.StatusCreated || json.Unmarshal(createdCategory.Body.Bytes(), &category) != nil {
		t.Fatalf("create category = %d %s", createdCategory.Code, createdCategory.Body.String())
	}
	categoryPath := path + "/categories/" + category.ID.String()
	out := `{"description":"Expense","direction":"out","amountMinor":1,"currency":"USD","paymentMethod":"cash","categoryId":"` + category.ID.String() + `"}`
	in := strings.Replace(out, `"out"`, `"in"`, 1)
	if response := serveAuthorized(mux, http.MethodPost, path+"/entries", in, ownerToken, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("categorized income = %d %s", response.Code, response.Body.String())
	}
	if response := serveAuthorized(mux, http.MethodPost, path+"/entries", out, ownerToken, nil); response.Code != http.StatusCreated {
		t.Fatalf("categorized expense = %d %s", response.Code, response.Body.String())
	}
	if response := serveAuthorized(mux, http.MethodPatch, categoryPath, `{"name":"Office rent"}`, ownerToken, nil); response.Code != http.StatusOK {
		t.Fatalf("rename = %d %s", response.Code, response.Body.String())
	}
	if response := serveAuthorized(mux, http.MethodDelete, categoryPath, "", ownerToken, nil); response.Code != http.StatusOK {
		t.Fatalf("archive = %d %s", response.Code, response.Body.String())
	}
	if response := serveAuthorized(mux, http.MethodPost, path+"/entries", out, ownerToken, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("archived category = %d %s", response.Code, response.Body.String())
	}
	if response := serveAuthorized(mux, http.MethodGet, path+"/entries", "", ownerToken, nil); !strings.Contains(response.Body.String(), `"categoryName":"Office rent"`) {
		t.Fatalf("history = %d %s", response.Code, response.Body.String())
	}
	methods := serveAuthorized(mux, http.MethodGet, path+"/methods", "", ownerToken, nil)
	var available []transactions.Method
	if methods.Code != http.StatusOK || json.Unmarshal(methods.Body.Bytes(), &available) != nil || len(available) != 4 {
		t.Fatalf("methods = %d %s", methods.Code, methods.Body.String())
	}
	for _, method := range []string{"check", "transfer", "cash", "card"} {
		found := false
		for _, got := range available {
			found = found || (got.ID == method && got.Enabled)
		}
		if !found {
			t.Errorf("missing method %q", method)
		}
	}
	createdMethod := serveAuthorized(mux, http.MethodPost, path+"/methods", `{"name":"Wire"}`, ownerToken, nil)
	var wire transactions.Method
	if createdMethod.Code != http.StatusCreated || json.Unmarshal(createdMethod.Body.Bytes(), &wire) != nil || wire.ID == "" || wire.Name != "Wire" || !wire.Enabled {
		t.Fatalf("create method = %d %s", createdMethod.Code, createdMethod.Body.String())
	}
	disabledMethod := serveAuthorized(mux, http.MethodPut, path+"/methods", `{"id":"`+wire.ID+`","name":"Bank wire","enabled":false}`, ownerToken, nil)
	var disabled transactions.Method
	if disabledMethod.Code != http.StatusOK || json.Unmarshal(disabledMethod.Body.Bytes(), &disabled) != nil || disabled.ID != wire.ID || disabled.Name != "Bank wire" || disabled.Enabled {
		t.Fatalf("disable method = %d %s", disabledMethod.Code, disabledMethod.Body.String())
	}
	ownerMethods := serveAuthorized(mux, http.MethodGet, path+"/methods", "", ownerToken, nil)
	if !strings.Contains(ownerMethods.Body.String(), `"name":"Bank wire","enabled":false`) {
		t.Fatalf("owner methods = %d %s", ownerMethods.Code, ownerMethods.Body.String())
	}
	disabledEntry := serveAuthorized(mux, http.MethodPost, path+"/entries", `{"description":"Receipt","direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"`+wire.ID+`"}`, ownerToken, nil)
	if disabledEntry.Code != http.StatusBadRequest {
		t.Fatalf("disabled method entry = %d %s", disabledEntry.Code, disabledEntry.Body.String())
	}
	body := `{"description":"  Cash sale  ","direction":"in","amountMinor":1250,"currency":"USD","paymentMethod":"cash","occurredOn":"2025-01-02","note":"Sale note"}`
	created := serveAuthorized(mux, http.MethodPost, path+"/entries", body, ownerToken, nil)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"amountMinor":1250`) || !strings.Contains(created.Body.String(), `"description":"Cash sale"`) {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	var sale struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &sale); err != nil || sale.ID == "" {
		t.Fatalf("created ID = %q, %v", sale.ID, err)
	}
	for _, tc := range []struct {
		query string
		want  string
	}{
		{"?direction=in&paymentMethod=cash&from=2025-01-02&to=2025-01-02", sale.ID},
		{"?direction=out&from=2025-01-02&to=2025-01-02", ""},
		{"?clientId=" + clients.ClientID(999999).String(), ""},
		{"?categoryId=" + category.ID.String() + "&direction=in", ""},
	} {
		response := serveAuthorized(mux, http.MethodGet, path+"/entries"+tc.query, "", ownerToken, nil)
		var entries []struct {
			ID string `json:"id"`
		}
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &entries) != nil || (tc.want != "" && (len(entries) != 1 || entries[0].ID != tc.want)) || (tc.want == "" && len(entries) != 0) {
			t.Fatalf("filter %s = %d %s", tc.query, response.Code, response.Body.String())
		}
	}
	for _, query := range []string{"?clientId=bad!", "?categoryId=bad!", "?direction=bad", "?from=2025-99-99"} {
		if response := serveAuthorized(mux, http.MethodGet, path+"/entries"+query, "", ownerToken, nil); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid filter %s = %d %s", query, response.Code, response.Body.String())
		}
	}
	refundCategoryResponse := serveAuthorized(mux, http.MethodPost, path+"/categories", `{"name":"Refunds"}`, ownerToken, nil)
	var refundCategory transactions.Category
	if refundCategoryResponse.Code != http.StatusCreated || json.Unmarshal(refundCategoryResponse.Body.Bytes(), &refundCategory) != nil {
		t.Fatalf("refund category = %d %s", refundCategoryResponse.Code, refundCategoryResponse.Body.String())
	}
	refundBody := `{"description":"Refund","direction":"out","amountMinor":500,"currency":"USD","paymentMethod":"cash","refundOfEntryId":"` + sale.ID + `","categoryId":"` + refundCategory.ID.String() + `"}`
	refund := serveAuthorized(mux, http.MethodPost, path+"/entries", refundBody, ownerToken, nil)
	if refund.Code != http.StatusCreated || !strings.Contains(refund.Body.String(), `"refundOfEntryId":"`+sale.ID+`"`) || !strings.Contains(refund.Body.String(), `"categoryId":"`+refundCategory.ID.String()+`"`) {
		t.Fatalf("refund = %d %s", refund.Code, refund.Body.String())
	}
	excess := serveAuthorized(mux, http.MethodPost, path+"/entries", `{"description":"Refund","direction":"out","amountMinor":751,"currency":"USD","paymentMethod":"cash","refundOfEntryId":"`+sale.ID+`"}`, ownerToken, nil)
	if excess.Code != http.StatusBadRequest {
		t.Fatalf("excess refund = %d %s", excess.Code, excess.Body.String())
	}
	if response := serveAuthorized(mux, http.MethodPost, path+"/entries/"+sale.ID+"/void", `{}`, ownerToken, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("void receipt with active refund = %d %s", response.Code, response.Body.String())
	}
	var refundEntry struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(refund.Body.Bytes(), &refundEntry); err != nil || refundEntry.ID == "" {
		t.Fatalf("refund ID = %q, %v", refundEntry.ID, err)
	}
	if response := serveAuthorized(mux, http.MethodPost, path+"/entries/"+refundEntry.ID+"/void", `{}`, ownerToken, nil); response.Code != http.StatusOK {
		t.Fatalf("void refund = %d %s", response.Code, response.Body.String())
	}
	for _, correction := range []string{
		`{"description":"Replacement","direction":"out","amountMinor":500,"currency":"USD","paymentMethod":"cash","replacesEntryId":"` + refundEntry.ID + `"}`,
		`{"description":"Replacement","direction":"out","amountMinor":1251,"currency":"USD","paymentMethod":"cash","refundOfEntryId":"` + sale.ID + `","replacesEntryId":"` + refundEntry.ID + `"}`,
	} {
		if response := serveAuthorized(mux, http.MethodPost, path+"/entries", correction, ownerToken, nil); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid refund replacement = %d %s", response.Code, response.Body.String())
		}
	}
	replacedRefund := serveAuthorized(mux, http.MethodPost, path+"/entries", `{"description":"Replacement","direction":"out","amountMinor":500,"currency":"USD","paymentMethod":"cash","refundOfEntryId":"`+sale.ID+`","replacesEntryId":"`+refundEntry.ID+`"}`, ownerToken, nil)
	if replacedRefund.Code != http.StatusCreated {
		t.Fatalf("refund replacement = %d %s", replacedRefund.Code, replacedRefund.Body.String())
	}
	original := serveAuthorized(mux, http.MethodPost, path+"/entries", body, ownerToken, nil)
	var correction struct {
		ID string `json:"id"`
	}
	if original.Code != http.StatusCreated || json.Unmarshal(original.Body.Bytes(), &correction) != nil || correction.ID == "" {
		t.Fatalf("original = %d %s", original.Code, original.Body.String())
	}
	voided := serveAuthorized(mux, http.MethodPost, path+"/entries/"+correction.ID+"/void", `{"reason":"Wrong amount"}`, ownerToken, nil)
	if voided.Code != http.StatusOK || !strings.Contains(voided.Body.String(), `"voidReason":"Wrong amount"`) || !strings.Contains(voided.Body.String(), `"voidedBy":`) || !strings.Contains(voided.Body.String(), `"voidedAt":`) {
		t.Fatalf("void = %d %s", voided.Code, voided.Body.String())
	}
	replacement := serveAuthorized(mux, http.MethodPost, path+"/entries", `{"description":"Replacement","direction":"in","amountMinor":1000,"currency":"USD","paymentMethod":"cash","replacesEntryId":"`+correction.ID+`"}`, ownerToken, nil)
	if replacement.Code != http.StatusCreated || !strings.Contains(replacement.Body.String(), `"replacesEntryId":"`+correction.ID+`"`) {
		t.Fatalf("replacement = %d %s", replacement.Code, replacement.Body.String())
	}
	var replacementEntry struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(replacement.Body.Bytes(), &replacementEntry); err != nil || replacementEntry.ID == "" {
		t.Fatalf("replacement ID = %q, %v", replacementEntry.ID, err)
	}
	if response := serveAuthorized(mux, http.MethodPost, path+"/entries/"+replacementEntry.ID+"/void", `{}`, ownerToken, nil); response.Code != http.StatusOK {
		t.Fatalf("void replacement = %d %s", response.Code, response.Body.String())
	}
	if response := serveAuthorized(mux, http.MethodPost, path+"/entries", `{"description":"Replacement","direction":"in","amountMinor":900,"currency":"USD","paymentMethod":"cash","replacesEntryId":"`+replacementEntry.ID+`"}`, ownerToken, nil); response.Code != http.StatusCreated {
		t.Fatalf("second replacement = %d %s", response.Code, response.Body.String())
	}
	repeated := serveAuthorized(mux, http.MethodPost, path+"/entries/"+correction.ID+"/void", `{"reason":"Again"}`, ownerToken, nil)
	if repeated.Code != http.StatusBadRequest {
		t.Fatalf("repeated void = %d %s", repeated.Code, repeated.Body.String())
	}
	list := serveAuthorized(mux, http.MethodGet, path+"/entries", "", ownerToken, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"note":"Sale note"`) || !strings.Contains(list.Body.String(), `"description":"Cash sale"`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	invalidPage := serveAuthorized(mux, http.MethodGet, path+"/entries?page=0", "", ownerToken, nil)
	if invalidPage.Code != http.StatusBadRequest {
		t.Errorf("invalid page = %d %s", invalidPage.Code, invalidPage.Body.String())
	}
	nextPage := serveAuthorized(mux, http.MethodGet, path+"/entries?page=2", "", ownerToken, nil)
	if nextPage.Code != http.StatusOK || strings.TrimSpace(nextPage.Body.String()) != "[]" {
		t.Errorf("next page = %d %s", nextPage.Code, nextPage.Body.String())
	}
	for _, invalid := range []string{
		`{"direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"cash"}`,
		`{"description":"   ","direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"cash"}`,
		`{"description":"` + strings.Repeat("é", 201) + `","direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"cash"}`,
		`{"description":"Entry","direction":"in","amountMinor":0,"currency":"USD","paymentMethod":"cash"}`,
		`{"description":"Entry","direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"disabled"}`,
		`{"description":"Entry","direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"cash","clientId":"` + clients.ClientID(999999).String() + `"}`,
		`{"description":"Entry","direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"cash","extra":true}`,
	} {
		response := serveAuthorized(mux, http.MethodPost, path+"/entries", invalid, ownerToken, nil)
		if response.Code != http.StatusBadRequest {
			t.Errorf("invalid create = %d %s", response.Code, response.Body.String())
		}
	}

	ctx := context.Background()
	workspaceID := identity.ParseWorkspaceIDOrZero(workspace)
	var ownerID identity.UserID
	if err := db.QueryRowContext(ctx, "SELECT user_id FROM workspace_members WHERE workspace_id = ? AND role = 'owner'", workspaceID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, ownerID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.CreateWorkspace(ctx, owner, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherPath := "/api/v1/workspaces/" + other.String() + "/transactions"
	for _, operation := range []struct{ method, suffix, body string }{
		{http.MethodGet, "/totals", ""},
		{http.MethodGet, "/entries", ""},
		{http.MethodGet, "/methods", ""},
		{http.MethodPost, "/methods", `{"name":"Foreign"}`},
		{http.MethodPut, "/methods", `{"id":"cash","name":"Cash","enabled":true}`},
		{http.MethodPost, "/entries/" + sale.ID + "/void", `{"reason":"Wrong"}`},
		{http.MethodPost, "/entries", body},
	} {
		response := serveAuthorized(mux, operation.method, otherPath+operation.suffix, operation.body, ownerToken, nil)
		if response.Code != http.StatusNotFound {
			t.Errorf("foreign workspace %s %s = %d %s", operation.method, operation.suffix, response.Code, response.Body.String())
		}
	}
	var adminMethod transactions.Method
	for _, role := range []identity.Role{identity.Admin, identity.Member, identity.Viewer} {
		invitation, err := auth.CreateInvitation(ctx, owner, string(role)+"@transactions.example.com", role, time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		userID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "long-enough-password", identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		principal, err := auth.Authorize(ctx, userID, workspaceID, identity.ResourcesRead)
		if err != nil {
			t.Fatal(err)
		}
		scopes := []identity.Permission{identity.ResourcesRead}
		if role != identity.Viewer {
			scopes = append(scopes, identity.ResourcesWrite)
		}
		token, err := auth.CreateAPIToken(ctx, principal, "transactions", scopes, nil, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		totals := serveAuthorized(mux, http.MethodGet, path+"/totals", "", token.Secret, nil)
		wantTotals := http.StatusForbidden
		if role == identity.Admin {
			wantTotals = http.StatusOK
		}
		if totals.Code != wantTotals {
			t.Errorf("%s totals = %d %s", role, totals.Code, totals.Body.String())
		}
		want := http.StatusOK
		if role == identity.Viewer {
			want = http.StatusForbidden
		}
		for _, suffix := range []string{"/entries", "/methods", "/categories"} {
			response := serveAuthorized(mux, http.MethodGet, path+suffix, "", token.Secret, nil)
			if response.Code != want {
				t.Errorf("%s GET %s = %d %s", role, suffix, response.Code, response.Body.String())
			}
			if suffix == "/methods" && response.Code == http.StatusOK {
				hasDisabled := strings.Contains(response.Body.String(), `"name":"Bank wire"`)
				if hasDisabled != (role == identity.Admin) {
					t.Errorf("%s method visibility = %s", role, response.Body.String())
				}
			}
		}
		response := serveAuthorized(mux, http.MethodPost, path+"/entries", body, token.Secret, nil)
		want = http.StatusCreated
		if role == identity.Viewer {
			want = http.StatusForbidden
		}
		if response.Code != want {
			t.Errorf("%s create = %d %s", role, response.Code, response.Body.String())
		}
		response = serveAuthorized(mux, http.MethodPost, path+"/categories", `{"name":"Role category"}`, token.Secret, nil)
		methodResponse := serveAuthorized(mux, http.MethodPost, path+"/methods", `{"name":"Admin method"}`, token.Secret, nil)
		want = http.StatusForbidden
		if role == identity.Admin {
			want = http.StatusCreated
		}
		if response.Code != want {
			t.Errorf("%s category create = %d %s", role, response.Code, response.Body.String())
		}
		if methodResponse.Code != want {
			t.Errorf("%s create method = %d %s", role, methodResponse.Code, methodResponse.Body.String())
		}
		if role == identity.Admin && (json.Unmarshal(methodResponse.Body.Bytes(), &adminMethod) != nil || adminMethod.ID == "") {
			t.Fatalf("admin method = %s", methodResponse.Body.String())
		}
		updateResponse := serveAuthorized(mux, http.MethodPut, path+"/methods", `{"id":"`+wire.ID+`","name":"Bank wire","enabled":true}`, token.Secret, nil)
		want = http.StatusForbidden
		if role == identity.Admin {
			want = http.StatusOK
		}
		if updateResponse.Code != want {
			t.Errorf("%s update method = %d %s", role, updateResponse.Code, updateResponse.Body.String())
		}
		if role == identity.Admin {
			serveAuthorized(mux, http.MethodPut, path+"/methods", `{"id":"`+wire.ID+`","name":"Bank wire","enabled":false}`, ownerToken, nil)
		}
		target := serveAuthorized(mux, http.MethodPost, path+"/entries", body, ownerToken, nil)
		var targetEntry struct {
			ID string `json:"id"`
		}
		if target.Code != http.StatusCreated || json.Unmarshal(target.Body.Bytes(), &targetEntry) != nil || targetEntry.ID == "" {
			t.Fatalf("target = %d %s", target.Code, target.Body.String())
		}
		response = serveAuthorized(mux, http.MethodPost, path+"/entries/"+targetEntry.ID+"/void", `{"reason":"Wrong"}`, token.Secret, nil)
		if response.Code != want {
			t.Errorf("%s void = %d %s", role, response.Code, response.Body.String())
		}
		if role == identity.Member {
			refundBody := `{"description":"Refund","direction":"out","amountMinor":1,"currency":"USD","paymentMethod":"cash","refundOfEntryId":"` + sale.ID + `"}`
			if response := serveAuthorized(mux, http.MethodPost, path+"/entries", refundBody, token.Secret, nil); response.Code != http.StatusCreated {
				t.Errorf("member refund = %d %s", response.Code, response.Body.String())
			}
			correctionBody := `{"description":"Replacement","direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"cash","replacesEntryId":"` + correction.ID + `"}`
			if response := serveAuthorized(mux, http.MethodPost, path+"/entries", correctionBody, token.Secret, nil); response.Code != http.StatusForbidden {
				t.Errorf("member replacement = %d %s", response.Code, response.Body.String())
			}
		}
	}
	for _, id := range []string{"check", "transfer", "cash", "card"} {
		response := serveAuthorized(mux, http.MethodPut, path+"/methods", `{"id":"`+id+`","name":"`+id+`","enabled":false}`, ownerToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("disable %s = %d %s", id, response.Code, response.Body.String())
		}
	}
	last := serveAuthorized(mux, http.MethodPut, path+"/methods", `{"id":"`+adminMethod.ID+`","name":"Admin method","enabled":false}`, ownerToken, nil)
	if last.Code != http.StatusConflict || !strings.Contains(last.Body.String(), "last-payment-method") {
		t.Fatalf("disable last method = %d %s", last.Code, last.Body.String())
	}
	reenabled := serveAuthorized(mux, http.MethodPut, path+"/methods", `{"id":"`+wire.ID+`","name":"Bank wire","enabled":true}`, ownerToken, nil)
	if reenabled.Code != http.StatusOK || !strings.Contains(reenabled.Body.String(), `"enabled":true`) {
		t.Fatalf("reenable method = %d %s", reenabled.Code, reenabled.Body.String())
	}
	entryWithRenamedMethod := serveAuthorized(mux, http.MethodPost, path+"/entries", `{"description":"Receipt","direction":"in","amountMinor":1,"currency":"USD","paymentMethod":"`+wire.ID+`"}`, ownerToken, nil)
	if entryWithRenamedMethod.Code != http.StatusCreated || !strings.Contains(entryWithRenamedMethod.Body.String(), `"paymentMethod":"`+wire.ID+`"`) {
		t.Fatalf("renamed method entry = %d %s", entryWithRenamedMethod.Code, entryWithRenamedMethod.Body.String())
	}
}
