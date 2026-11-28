package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/transactions"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestTransactionsTotalsTool(t *testing.T) {
	state := newTestState(t, config.MCP{})
	if err := currency.NewService(state.db, state.auth).SetCurrencies(context.Background(), state.actor, state.workspaceKey, []currency.Currency{{Code: "USD", Decimals: 2, Default: true}, {Code: "EUR", Decimals: 2}}); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(state.auth, config.MCP{}, slog.New(slog.NewTextHandler(io.Discard, nil)), WithTransactions(transactions.NewService(state.db, state.auth)))
	owner := connectClient(t, handler, state.token.Secret, false)
	for _, entry := range []map[string]any{
		{"workspaceId": state.workspaceID, "description": "Receipt", "direction": "in", "amountMinor": 125, "currency": "USD", "paymentMethod": "cash", "occurredOn": "2026-01-02"},
		{"workspaceId": state.workspaceID, "description": "Expense", "direction": "out", "amountMinor": 25, "currency": "USD", "paymentMethod": "cash", "occurredOn": "2026-01-03"},
		{"workspaceId": state.workspaceID, "description": "Receipt", "direction": "in", "amountMinor": 50, "currency": "EUR", "paymentMethod": "cash", "occurredOn": "2026-02-01"},
	} {
		if result := callTool(t, owner, "transactions_create_v1", entry); result.IsError {
			t.Fatalf("create entry: %s", contentText(result))
		}
	}
	args := map[string]any{"workspaceId": state.workspaceID, "period": "custom", "start": "2026-01-01", "end": "2026-01-31"}
	result := callTool(t, owner, "transactions_totals_v1", args)
	if result.IsError {
		t.Fatalf("totals: %s", contentText(result))
	}
	totals := decodeOutput[transactionsTotalsOutput](t, result)
	if totals.Period != "custom" || totals.Start != "2026-01-01" || totals.End != "2026-01-31" || len(totals.Items) != 3 || totals.NextCursor != "" {
		t.Fatalf("totals = %+v", totals)
	}
	currency := totals.Items[0]
	if currency.Kind != "currency" || currency.Currency != "USD" || currency.MoneyIn != "125" || currency.MoneyOut != "25" || currency.Net != "100" {
		t.Fatalf("currency totals = %+v", currency)
	}
	if totals.Items[1].Kind != "method" || totals.Items[1].ID != "cash" || totals.Items[2].Kind != "category" || !totals.Items[2].Uncategorized {
		t.Fatalf("breakdowns = %+v", totals.Items)
	}
	args["limit"] = 1
	var items []transactionsTotalsItem
	for {
		result = callTool(t, owner, "transactions_totals_v1", args)
		if result.IsError {
			t.Fatalf("totals page: %s", contentText(result))
		}
		page := decodeOutput[transactionsTotalsOutput](t, result)
		if len(page.Items) != 1 {
			t.Fatalf("page = %+v", page)
		}
		items = append(items, page.Items...)
		if page.NextCursor == "" {
			break
		}
		args["cursor"] = page.NextCursor
		if len(items) > 3 {
			t.Fatal("pagination did not terminate")
		}
	}
	for i := range totals.Items {
		if items[i] != totals.Items[i] {
			t.Fatalf("paged items = %+v, expected %+v", items, totals.Items)
		}
	}
	delete(args, "cursor")
	delete(args, "limit")
	admin := connectClient(t, handler, tokenWithScopes(t, state, identity.Admin, identity.ResourcesRead), false)
	if result := callTool(t, admin, "transactions_totals_v1", args); result.IsError {
		t.Fatalf("admin totals: %s", contentText(result))
	}
	for _, role := range []identity.Role{identity.Member, identity.Viewer} {
		session := connectClient(t, handler, tokenWithScopes(t, state, role, identity.ResourcesRead), false)
		if result := callTool(t, session, "transactions_totals_v1", args); !result.IsError {
			t.Fatalf("%s accessed totals", role)
		}
	}
	for _, args := range []map[string]any{
		{"workspaceId": state.workspaceID, "limit": 101},
		{"workspaceId": state.workspaceID, "cursor": "not-a-cursor"},
	} {
		if result := callTool(t, owner, "transactions_totals_v1", args); !result.IsError {
			t.Fatalf("invalid pagination accepted: %v", args)
		}
	}
	if result := callTool(t, owner, "transactions_totals_v1", map[string]any{"workspaceId": state.workspaceID, "period": "invalid"}); !result.IsError {
		t.Fatal("invalid period accepted")
	}
}

func TestTransactionsTotalsPaginationBeyondCollectionLimit(t *testing.T) {
	state := newTestState(t, config.MCP{})
	configured := []currency.Currency{{Code: "USD", Decimals: 2, Default: true}}
	for _, item := range currency.Available() {
		if item.Code != "USD" && transactions.CurrencyAvailable(item) && len(configured) < 102 {
			configured = append(configured, item)
		}
	}
	if len(configured) != 102 {
		t.Fatalf("available currencies = %d, need 102", len(configured))
	}
	if err := currency.NewService(state.db, state.auth).SetCurrencies(context.Background(), state.actor, state.workspaceKey, configured); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(state.auth, config.MCP{}, slog.New(slog.NewTextHandler(io.Discard, nil)), WithTransactions(transactions.NewService(state.db, state.auth)))
	owner := connectClient(t, handler, state.token.Secret, false)
	create := func(tool string, args map[string]any) *mcp.CallToolResult {
		result := callTool(t, owner, tool, args)
		if result.IsError {
			t.Fatalf("%s: %s", tool, contentText(result))
		}
		return result
	}
	for i := range 101 {
		category := decodeOutput[transactionsCategoryOutput](t, create("transactions_category_create_v1", map[string]any{"workspaceId": state.workspaceID, "name": fmt.Sprintf("Category %03d", i)}))
		create("transactions_create_v1", map[string]any{"workspaceId": state.workspaceID, "description": "Expense", "direction": "out", "amountMinor": 1, "currency": "USD", "paymentMethod": "cash", "categoryId": category.ID, "occurredOn": "2026-01-01"})
		if i == 0 {
			create("transactions_category_archive_v1", map[string]any{"workspaceId": state.workspaceID, "categoryId": category.ID})
		}
	}
	for _, item := range configured[1:] {
		create("transactions_create_v1", map[string]any{"workspaceId": state.workspaceID, "description": "Receipt", "direction": "in", "amountMinor": 1, "currency": item.Code, "paymentMethod": "card", "occurredOn": "2026-01-01"})
	}
	args := map[string]any{"workspaceId": state.workspaceID, "period": "custom", "start": "2026-01-01", "end": "2026-01-31", "limit": 100}
	counts := map[transactionsTotalsKind]int{}
	seen := map[string]bool{}
	archived := 0
	for pages := 0; ; pages++ {
		if pages > 4 {
			t.Fatal("pagination did not terminate")
		}
		page := decodeOutput[transactionsTotalsOutput](t, create("transactions_totals_v1", args))
		if len(page.Items) == 0 || len(page.Items) > 100 {
			t.Fatalf("invalid page size: %d", len(page.Items))
		}
		for _, item := range page.Items {
			key := string(item.Currency) + ":" + string(item.Kind) + ":" + string(item.ID)
			if seen[key] {
				t.Fatalf("duplicate item: %s", key)
			}
			seen[key] = true
			counts[item.Kind]++
			if item.Archived {
				archived++
			}
			if item.Currency == "USD" && item.Kind == "currency" && (item.MoneyOut != "101" || item.Net != "-101") {
				t.Fatalf("USD total = %+v", item)
			}
		}
		if page.NextCursor == "" {
			break
		}
		args["cursor"] = page.NextCursor
	}
	if counts["currency"] != 102 || counts["method"] != 102 || counts["category"] != 101 || archived != 1 {
		t.Fatalf("counts = %v, archived = %d", counts, archived)
	}
}

func TestTransactionsToolsCreateAndList(t *testing.T) {
	state := newTestState(t, config.MCP{})
	handler := NewHandler(state.auth, config.MCP{}, slog.New(slog.NewTextHandler(io.Discard, nil)), WithTransactions(transactions.NewService(state.db, state.auth)))
	session := connectClient(t, handler, state.token.Secret, false)
	methods := callTool(t, session, "transactions_methods_v1", map[string]any{"workspaceId": state.workspaceID})
	if methods.IsError {
		t.Fatalf("methods: %s", contentText(methods))
	}
	methodList := decodeOutput[transactionsMethodsOutput](t, methods)
	foundCash := false
	for _, method := range methodList.Methods {
		if method == "cash" {
			foundCash = true
		}
	}
	if !foundCash {
		t.Fatalf("methods = %v", methodList.Methods)
	}
	categoryResult := callTool(t, session, "transactions_category_create_v1", map[string]any{"workspaceId": state.workspaceID, "name": "Rent"})
	if categoryResult.IsError {
		t.Fatalf("category create: %s", contentText(categoryResult))
	}
	category := decodeOutput[transactionsCategoryOutput](t, categoryResult)
	expense := callTool(t, session, "transactions_create_v1", map[string]any{"workspaceId": state.workspaceID, "description": "Expense", "direction": "out", "amountMinor": 1, "currency": "USD", "paymentMethod": "cash", "categoryId": category.ID})
	if expense.IsError {
		t.Fatalf("categorized expense: %s", contentText(expense))
	}
	rename := callTool(t, session, "transactions_category_rename_v1", map[string]any{"workspaceId": state.workspaceID, "categoryId": category.ID, "name": "Office rent"})
	if rename.IsError {
		t.Fatalf("rename: %s", contentText(rename))
	}
	archive := callTool(t, session, "transactions_category_archive_v1", map[string]any{"workspaceId": state.workspaceID, "categoryId": category.ID})
	if archive.IsError {
		t.Fatalf("archive: %s", contentText(archive))
	}
	refusedExpense := callTool(t, session, "transactions_create_v1", map[string]any{"workspaceId": state.workspaceID, "description": "Expense", "direction": "out", "amountMinor": 1, "currency": "USD", "paymentMethod": "cash", "categoryId": category.ID})
	if !refusedExpense.IsError {
		t.Fatal("archived category accepted")
	}
	categories := callTool(t, session, "transactions_categories_v1", map[string]any{"workspaceId": state.workspaceID})
	categoryPage := decodeOutput[transactionsCategoriesOutput](t, categories)
	if categories.IsError || len(categoryPage.Categories) != 1 || !categoryPage.Categories[0].Archived || categoryPage.Categories[0].Name != "Office rent" {
		t.Fatalf("categories: %+v", categoryPage)
	}
	created := callTool(t, session, "transactions_create_v1", map[string]any{
		"workspaceId": state.workspaceID, "description": "  Sale  ", "direction": "in", "amountMinor": 125,
		"currency": "USD", "paymentMethod": "cash", "occurredOn": "2026-01-02", "note": "sale",
	})
	if created.IsError {
		t.Fatalf("create: %s", contentText(created))
	}
	entry := decodeOutput[transactionsEntryOutput](t, created)
	if entry.ID == "" || entry.Description != "Sale" || entry.AmountMinor != 125 || entry.PaymentMethod != "cash" || entry.Note != "sale" || entry.CreatedBy == "" {
		t.Fatalf("created entry = %+v", entry)
	}
	categoryKey, err := transactions.ParseCategoryID(string(category.ID))
	if err != nil {
		t.Fatal(err)
	}
	entryKey, err := transactions.ParseEntryID(string(entry.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []struct {
		action string
		kind   string
		key    int64
	}{
		{"transactions_category_create_v1", "transactions_category", int64(categoryKey)},
		{"transactions_create_v1", "transactions_entry", int64(entryKey)},
	} {
		var count int
		err := state.db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE event_type = 'mcp.tool.called' AND action = ? AND target_type = ? AND target_key = ?`, target.action, target.kind, target.key).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("audit %s: count %d, error %v", target.action, count, err)
		}
	}
	listed := callTool(t, session, "transactions_list_v1", map[string]any{"workspaceId": state.workspaceID})
	if listed.IsError {
		t.Fatalf("list: %s", contentText(listed))
	}
	page := decodeOutput[transactionsListOutput](t, listed)
	if len(page.Entries) != 2 || page.Entries[0].ID != entry.ID || page.Entries[0].Description != "Sale" || page.Entries[1].CategoryName != "Office rent" || page.Page != 1 {
		t.Fatalf("page = %+v", page)
	}
	filtered := callTool(t, session, "transactions_list_v1", map[string]any{"workspaceId": state.workspaceID, "direction": "in", "paymentMethod": "cash", "from": "2026-01-02", "to": "2026-01-02"})
	if filtered.IsError {
		t.Fatalf("filtered list: %s", contentText(filtered))
	}
	if got := decodeOutput[transactionsListOutput](t, filtered); len(got.Entries) != 1 || got.Entries[0].ID != entry.ID {
		t.Fatalf("filtered page = %+v", got)
	}
	filteredCategory := callTool(t, session, "transactions_list_v1", map[string]any{"workspaceId": state.workspaceID, "categoryId": category.ID})
	if filteredCategory.IsError {
		t.Fatalf("category filter: %s", contentText(filteredCategory))
	}
	if got := decodeOutput[transactionsListOutput](t, filteredCategory); len(got.Entries) != 1 || got.Entries[0].CategoryName != "Office rent" {
		t.Fatalf("category page = %+v", got)
	}
	next := callTool(t, session, "transactions_list_v1", map[string]any{"workspaceId": state.workspaceID, "page": 2})
	if next.IsError {
		t.Fatalf("next page: %s", contentText(next))
	}
	if page := decodeOutput[transactionsListOutput](t, next); page.Page != 2 || len(page.Entries) != 0 {
		t.Fatalf("next page = %+v", page)
	}
	invalid := callTool(t, session, "transactions_create_v1", map[string]any{
		"workspaceId": state.workspaceID, "description": "Entry", "direction": "in", "amountMinor": 0, "currency": "USD", "paymentMethod": "cash",
	})
	if !invalid.IsError {
		t.Fatal("zero amount accepted")
	}
	missingDescription := callTool(t, session, "transactions_create_v1", map[string]any{
		"workspaceId": state.workspaceID, "direction": "in", "amountMinor": 1, "currency": "USD", "paymentMethod": "cash",
	})
	if !missingDescription.IsError {
		t.Fatal("missing description accepted")
	}
	for _, description := range []string{"", "   ", strings.Repeat("é", 201)} {
		result := callTool(t, session, "transactions_create_v1", map[string]any{
			"workspaceId": state.workspaceID, "description": description, "direction": "in", "amountMinor": 1, "currency": "USD", "paymentMethod": "cash",
		})
		if !result.IsError {
			t.Fatalf("invalid description accepted: %q", description)
		}
	}
	viewer := tokenWithScopes(t, state, identity.Viewer, identity.ResourcesRead)
	viewerSession := connectClient(t, handler, viewer, false)
	refused := callTool(t, viewerSession, "transactions_list_v1", map[string]any{"workspaceId": state.workspaceID})
	if !refused.IsError {
		t.Fatal("viewer read transactions")
	}
	readToken, err := state.auth.CreateAPIToken(context.Background(), state.actor, "transactions read", []identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	readSession := connectClient(t, handler, readToken.Secret, false)
	_, err = readSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "transactions_create_v1", Arguments: map[string]any{
		"workspaceId": state.workspaceID, "description": "Entry", "direction": "in", "amountMinor": 125, "currency": "USD", "paymentMethod": "cash",
	}})
	if err == nil {
		t.Fatal("read-only token created transactions entry")
	}
	var createdCount int
	if err := state.db.QueryRow("SELECT COUNT(*) FROM transactions_entries WHERE workspace_id = ? AND description = ?", state.workspaceKey, "Entry").Scan(&createdCount); err != nil || createdCount != 0 {
		t.Fatalf("denied entries = %d, %v", createdCount, err)
	}
}

func TestTransactionsMethodManagement(t *testing.T) {
	state := newTestState(t, config.MCP{})
	handler := NewHandler(state.auth, config.MCP{}, slog.New(slog.NewTextHandler(io.Discard, nil)), WithTransactions(transactions.NewService(state.db, state.auth)))
	owner := connectClient(t, handler, state.token.Secret, false)
	created := callTool(t, owner, "transactions_method_create_v1", map[string]any{"workspaceId": state.workspaceID, "name": "Wire transfer"})
	if created.IsError {
		t.Fatalf("create method: %s", contentText(created))
	}
	method := decodeOutput[transactionsMethodOutput](t, created)
	if method.ID == "" || method.Name != "Wire transfer" || !method.Enabled {
		t.Fatalf("created method = %+v", method)
	}
	updated := callTool(t, owner, "transactions_method_update_v1", map[string]any{"workspaceId": state.workspaceID, "id": method.ID, "name": "Bank transfer", "enabled": true})
	if updated.IsError {
		t.Fatalf("update method: %s", contentText(updated))
	}
	renamed := decodeOutput[transactionsMethodOutput](t, updated)
	if renamed.ID != method.ID || renamed.Name != "Bank transfer" || !renamed.Enabled {
		t.Fatalf("renamed method = %+v", renamed)
	}
	entry := callTool(t, owner, "transactions_create_v1", map[string]any{"workspaceId": state.workspaceID, "description": "Entry", "direction": "in", "amountMinor": 125, "currency": "USD", "paymentMethod": method.ID})
	if entry.IsError || decodeOutput[transactionsEntryOutput](t, entry).PaymentMethod != method.ID {
		t.Fatalf("entry using method: %s", contentText(entry))
	}
	disabled := callTool(t, owner, "transactions_method_update_v1", map[string]any{"workspaceId": state.workspaceID, "id": method.ID, "name": "Bank transfer", "enabled": false})
	if disabled.IsError {
		t.Fatalf("disable method: %s", contentText(disabled))
	}
	list := callTool(t, owner, "transactions_method_list_v1", map[string]any{"workspaceId": state.workspaceID})
	if list.IsError {
		t.Fatalf("list methods: %s", contentText(list))
	}
	found := false
	for _, item := range decodeOutput[transactionsMethodListOutput](t, list).Methods {
		if item.ID == method.ID && item.Name == "Bank transfer" && !item.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatal("owner cannot see disabled method")
	}
	member := connectClient(t, handler, tokenWithScopes(t, state, identity.Member, identity.ResourcesRead, identity.ResourcesWrite), false)
	memberList := callTool(t, member, "transactions_method_list_v1", map[string]any{"workspaceId": state.workspaceID})
	if memberList.IsError {
		t.Fatalf("member list: %s", contentText(memberList))
	}
	for _, item := range decodeOutput[transactionsMethodListOutput](t, memberList).Methods {
		if item.ID == method.ID {
			t.Fatal("member can see disabled method")
		}
	}
	refused := callTool(t, member, "transactions_method_create_v1", map[string]any{"workspaceId": state.workspaceID, "name": "Member method"})
	if !refused.IsError {
		t.Fatal("member created method")
	}
	var methodCount int
	if err := state.db.QueryRow("SELECT COUNT(*) FROM transactions_payment_methods WHERE workspace_id = ? AND display_name = ?", state.workspaceKey, "Member method").Scan(&methodCount); err != nil || methodCount != 0 {
		t.Fatalf("denied methods = %d, %v", methodCount, err)
	}
	refused = callTool(t, owner, "transactions_create_v1", map[string]any{"workspaceId": state.workspaceID, "description": "Entry", "direction": "in", "amountMinor": 125, "currency": "USD", "paymentMethod": method.ID})
	if !refused.IsError {
		t.Fatal("disabled method accepted for entry")
	}
	refused = callTool(t, owner, "transactions_method_update_v1", map[string]any{"workspaceId": state.workspaceID, "id": method.ID, "name": "Bank transfer"})
	if !refused.IsError {
		t.Fatal("missing enabled accepted")
	}
}

func TestTransactionsMethodPaginationBeyondCollectionLimit(t *testing.T) {
	state := newTestState(t, config.MCP{})
	handler := NewHandler(state.auth, config.MCP{}, slog.New(slog.NewTextHandler(io.Discard, nil)), WithTransactions(transactions.NewService(state.db, state.auth)))
	owner := connectClient(t, handler, state.token.Secret, false)
	member := connectClient(t, handler, tokenWithScopes(t, state, identity.Member, identity.ResourcesRead), false)
	viewer := connectClient(t, handler, tokenWithScopes(t, state, identity.Viewer, identity.ResourcesRead), false)
	initial := callTool(t, owner, "transactions_method_list_v1", map[string]any{"workspaceId": state.workspaceID})
	if initial.IsError {
		t.Fatalf("seed methods: %s", contentText(initial))
	}
	seedCount := len(decodeOutput[transactionsMethodListOutput](t, initial).Methods)
	for i := range 103 {
		_, err := state.db.Exec(`INSERT INTO transactions_payment_methods (workspace_id, name, display_name, enabled) VALUES ($1, $2, $3, $4)`, state.workspaceKey, fmt.Sprintf("historical-%03d", i), fmt.Sprintf("Historical %03d", i), i != 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name     string
		session  *mcp.ClientSession
		tool     string
		want     int
		disabled bool
	}{
		{"owner management", owner, "transactions_method_list_v1", seedCount + 103, true},
		{"member management", member, "transactions_method_list_v1", seedCount + 102, false},
		{"owner enabled IDs", owner, "transactions_methods_v1", seedCount + 102, false},
		{"member enabled IDs", member, "transactions_methods_v1", seedCount + 102, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"workspaceId": state.workspaceID}
			seen := map[transactionsMethod]bool{}
			foundDisabled := false
			var previous transactionsMethod
			for page := 0; ; page++ {
				if page > 2 {
					t.Fatal("pagination did not terminate")
				}
				result := callTool(t, tc.session, tc.tool, args)
				if result.IsError {
					t.Fatalf("list: %s", contentText(result))
				}
				var ids []transactionsMethod
				var next transactionsMethod
				if tc.tool == "transactions_method_list_v1" {
					output := decodeOutput[transactionsMethodListOutput](t, result)
					for _, method := range output.Methods {
						ids = append(ids, method.ID)
						if !method.Enabled {
							foundDisabled = true
						}
					}
					next = output.NextCursor
				} else {
					output := decodeOutput[transactionsMethodsOutput](t, result)
					ids, next = output.Methods, output.NextCursor
				}
				if len(ids) == 0 || len(ids) > 100 || (page == 0 && len(ids) != 100) {
					t.Fatalf("page %d size = %d", page, len(ids))
				}
				for _, id := range ids {
					if seen[id] || id <= previous {
						t.Fatalf("duplicate or unordered ID %q after %q", id, previous)
					}
					seen[id], previous = true, id
				}
				if next == "" {
					break
				}
				if next != previous {
					t.Fatalf("cursor %q does not match last ID %q", next, previous)
				}
				args["cursor"] = next
			}
			if len(seen) != tc.want || foundDisabled != tc.disabled {
				t.Fatalf("count = %d, disabled visible = %v", len(seen), foundDisabled)
			}
		})
	}
	for _, tool := range []string{"transactions_method_list_v1", "transactions_methods_v1"} {
		if result := callTool(t, viewer, tool, map[string]any{"workspaceId": state.workspaceID, "cursor": "historical-099"}); !result.IsError {
			t.Fatalf("%s allowed viewer", tool)
		}
		if result := callTool(t, owner, tool, map[string]any{"workspaceId": state.workspaceID, "cursor": string(make([]byte, 65))}); !result.IsError {
			t.Fatalf("%s accepted oversized cursor", tool)
		}
	}
	listed, err := owner.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, tool := range listed.Tools {
		if tool.Name != "transactions_method_list_v1" && tool.Name != "transactions_methods_v1" {
			continue
		}
		found++
		for index, schema := range []any{tool.InputSchema, tool.OutputSchema} {
			data, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Properties map[string]struct {
					MaxLength *int `json:"maxLength"`
					MaxItems  *int `json:"maxItems"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if index == 0 {
				cursor, ok := decoded.Properties["cursor"]
				if !ok || cursor.MaxLength == nil || *cursor.MaxLength != 64 {
					t.Fatalf("%s input schema = %s", tool.Name, data)
				}
			} else {
				next, ok := decoded.Properties["nextCursor"]
				if !ok || next.MaxLength == nil || *next.MaxLength != 64 || decoded.Properties["methods"].MaxItems == nil || *decoded.Properties["methods"].MaxItems != 100 {
					t.Fatalf("%s output schema = %s", tool.Name, data)
				}
			}
		}
	}
	if found != 2 {
		t.Fatalf("found %d method tools", found)
	}
}

func TestTransactionsMethodScopesAndLastEnabled(t *testing.T) {
	state := newTestState(t, config.MCP{})
	handler := NewHandler(state.auth, config.MCP{}, slog.New(slog.NewTextHandler(io.Discard, nil)), WithTransactions(transactions.NewService(state.db, state.auth)))
	owner := connectClient(t, handler, state.token.Secret, false)
	list := callTool(t, owner, "transactions_method_list_v1", map[string]any{"workspaceId": state.workspaceID})
	if list.IsError {
		t.Fatalf("list methods: %s", contentText(list))
	}
	methods := decodeOutput[transactionsMethodListOutput](t, list).Methods
	if len(methods) == 0 {
		t.Fatal("no seeded methods")
	}
	for i, method := range methods {
		result := callTool(t, owner, "transactions_method_update_v1", map[string]any{"workspaceId": state.workspaceID, "id": method.ID, "name": method.Name, "enabled": false})
		if i == len(methods)-1 {
			if !result.IsError {
				t.Fatal("disabled last enabled method")
			}
		} else if result.IsError {
			t.Fatalf("disable method: %s", contentText(result))
		}
	}
	readToken, err := state.auth.CreateAPIToken(context.Background(), state.actor, "method read", []identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	readSession := connectClient(t, handler, readToken.Secret, false)
	_, err = readSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "transactions_method_create_v1", Arguments: map[string]any{"workspaceId": state.workspaceID, "name": "Forbidden"}})
	if err == nil {
		t.Fatal("read-only token created method")
	}
	var methodCount int
	if err := state.db.QueryRow("SELECT COUNT(*) FROM transactions_payment_methods WHERE workspace_id = ? AND display_name = ?", state.workspaceKey, "Forbidden").Scan(&methodCount); err != nil || methodCount != 0 {
		t.Fatalf("denied methods = %d, %v", methodCount, err)
	}
}

func TestTransactionsCorrections(t *testing.T) {
	state := newTestState(t, config.MCP{})
	handler := NewHandler(state.auth, config.MCP{}, slog.New(slog.NewTextHandler(io.Discard, nil)), WithTransactions(transactions.NewService(state.db, state.auth)))
	session := connectClient(t, handler, state.token.Secret, true)
	create := func(direction string, amount int, link string, entryID transactionsEntryID) *mcp.CallToolResult {
		args := map[string]any{"workspaceId": state.workspaceID, "description": "Entry", "direction": direction, "amountMinor": amount, "currency": "USD", "paymentMethod": "cash"}
		if link != "" {
			args[link] = entryID
		}
		return callTool(t, session, "transactions_create_v1", args)
	}
	original := create("in", 125, "", "")
	if original.IsError {
		t.Fatalf("create: %s", contentText(original))
	}
	entry := decodeOutput[transactionsEntryOutput](t, original)
	refund := create("out", 75, "refundOfEntryId", entry.ID)
	if refund.IsError {
		t.Fatalf("refund: %s", contentText(refund))
	}
	refundEntry := decodeOutput[transactionsEntryOutput](t, refund)
	if refundEntry.RefundOfEntryID == nil || *refundEntry.RefundOfEntryID != entry.ID {
		t.Fatalf("refund link = %+v", refundEntry)
	}
	if result := create("out", 51, "refundOfEntryId", entry.ID); !result.IsError {
		t.Fatal("refund exceeding remainder accepted")
	}
	args := map[string]any{"workspaceId": state.workspaceID, "entryId": entry.ID, "reason": "correction"}
	declined := connectClient(t, handler, state.token.Secret, false)
	if result := callTool(t, declined, "transactions_void_v1", args); !result.IsError {
		t.Fatal("void without approval succeeded")
	}
	if result := callTool(t, session, "transactions_void_v1", args); !result.IsError {
		t.Fatal("voided receipt with active refund")
	}
	if result := callTool(t, session, "transactions_void_v1", map[string]any{"workspaceId": state.workspaceID, "entryId": refundEntry.ID}); result.IsError {
		t.Fatalf("void refund: %s", contentText(result))
	}
	voided := callTool(t, session, "transactions_void_v1", args)
	if voided.IsError {
		t.Fatalf("void: %s", contentText(voided))
	}
	entryKey, err := transactions.ParseEntryID(string(entry.ID))
	if err != nil {
		t.Fatal(err)
	}
	var count int
	err = state.db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE event_type = 'mcp.tool.called' AND action = 'transactions_void_v1' AND target_type = 'transactions_entry' AND target_key = ? AND outcome = 'success'`, int64(entryKey)).Scan(&count)
	if err != nil || count < 1 {
		t.Fatalf("void audit: count %d, error %v", count, err)
	}
	voidedEntry := decodeOutput[transactionsEntryOutput](t, voided)
	if voidedEntry.VoidedBy == nil || voidedEntry.VoidedAt == "" || voidedEntry.VoidReason != "correction" {
		t.Fatalf("void details = %+v", voidedEntry)
	}
	replacement := create("in", 125, "replacesEntryId", entry.ID)
	if replacement.IsError {
		t.Fatalf("replacement: %s", contentText(replacement))
	}
	replacementEntry := decodeOutput[transactionsEntryOutput](t, replacement)
	if replacementEntry.ReplacesEntryID == nil || *replacementEntry.ReplacesEntryID != entry.ID {
		t.Fatalf("replacement link = %+v", replacementEntry)
	}
	readToken, err := state.auth.CreateAPIToken(context.Background(), state.actor, "transactions corrections read", []identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	readSession := connectClient(t, handler, readToken.Secret, false)
	if _, err := readSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "transactions_void_v1", Arguments: map[string]any{"workspaceId": state.workspaceID, "entryId": replacementEntry.ID}}); err == nil {
		t.Fatal("read-only token invoked void")
	}
	replacementKey, err := transactions.ParseEntryID(string(replacementEntry.ID))
	if err != nil {
		t.Fatal(err)
	}
	var voidedCount int
	if err := state.db.QueryRow("SELECT COUNT(*) FROM transactions_entries WHERE id = ? AND voided_at IS NOT NULL", replacementKey).Scan(&voidedCount); err != nil || voidedCount != 0 {
		t.Fatalf("denied void changed replacement: count %d, %v", voidedCount, err)
	}
}
