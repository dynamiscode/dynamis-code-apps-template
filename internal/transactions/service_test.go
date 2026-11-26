package transactions

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestEntriesAndWorkspaceIsolation(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "transactions@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		workspace identity.WorkspaceID
		principal identity.Principal
	}{{owner.WorkspaceID, actor}, {other, otherActor}} {
		methods, err := s.Methods(ctx, pair.principal, pair.workspace)
		if err != nil || !reflect.DeepEqual(methods, []string{"card", "cash", "check", "transfer"}) {
			t.Fatalf("methods %v: %v %v", pair.workspace, methods, err)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspaces SET time_zone = 'Pacific/Kiritimati' WHERE id = $1`, owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	local, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	totals, err := s.Totals(ctx, actor, owner.WorkspaceID, "this_month", "", "")
	if err != nil {
		t.Fatal(err)
	}
	localNow := time.Now().In(local)
	expectedStart := time.Date(localNow.Year(), localNow.Month(), 1, 0, 0, 0, 0, local)
	if totals.Start != expectedStart.Format("2006-01-02") || totals.End != expectedStart.AddDate(0, 1, -1).Format("2006-01-02") {
		t.Fatalf("workspace-local month = %s to %s", totals.Start, totals.End)
	}
	before := time.Now().In(local).Format("2006-01-02")
	entry, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "  Café  ", Direction: "in", AmountMinor: 1250, Currency: "usd", PaymentMethod: "cash", Note: " sale "})
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now().In(local).Format("2006-01-02")
	if entry.OccurredOn != before && entry.OccurredOn != after {
		t.Fatalf("local date %s, expected %s or %s", entry.OccurredOn, before, after)
	}
	if entry.Description != "Café" || entry.Currency != "USD" || entry.Note != "sale" || entry.CreatedBy != owner.UserID || entry.ID.String() == "" || entry.CreatedAt.IsZero() {
		t.Fatalf("entry: %+v", entry)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO workspace_currencies (workspace_id, code, decimals, is_default) VALUES ($1, 'USD', 2, TRUE), ($1, 'EUR', 2, FALSE)`, owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	backdated, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: strings.Repeat("é", 200), Direction: "out", AmountMinor: 1, Currency: "EUR", PaymentMethod: "card", OccurredOn: "2020-02-29"})
	if err != nil || backdated.OccurredOn != "2020-02-29" || backdated.Description != strings.Repeat("é", 200) {
		t.Fatalf("backdated: %+v %v", backdated, err)
	}
	if _, err := s.Create(ctx, otherActor, other, Input{Description: "Wrong workspace", Direction: "in", AmountMinor: 1, Currency: "EUR", PaymentMethod: "cash"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("currency from another workspace: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_currencies SET decimals = 3 WHERE workspace_id = $1 AND code = 'EUR'`, owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Wrong precision", Direction: "in", AmountMinor: 1, Currency: "EUR", PaymentMethod: "cash"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("currency with noncanonical precision: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM workspace_currencies WHERE workspace_id = $1 AND code = 'EUR'`, owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"EUR", "ZZZ"} {
		_, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Rejected", Direction: "in", AmountMinor: 1, Currency: code, PaymentMethod: "cash"})
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("currency %s: expected invalid input, got %v", code, err)
		}
	}
	entries, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
	if err != nil || len(entries) != 2 || entries[0].ID != backdated.ID || entries[1].ID != entry.ID || entries[1].Description != "Café" {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	byDescription, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, Search: "Caf"})
	if err != nil || len(byDescription) != 1 || byDescription[0].ID != entry.ID {
		t.Fatalf("description search: %+v %v", byDescription, err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE transactions_entries SET description = '' WHERE workspace_id = $1 AND id = $2`, owner.WorkspaceID, backdated.ID); err != nil {
		t.Fatal(err)
	}
	historical, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, EntryID: backdated.ID})
	if err != nil || len(historical) != 1 || historical[0].Description != "" || historical[0].Currency != "EUR" || historical[0].AmountMinor != 1 {
		t.Fatalf("historical entry: %+v %v", historical, err)
	}
	historicalBatch, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, []EntryID{backdated.ID})
	if err != nil || len(historicalBatch) != 1 || historicalBatch[backdated.ID].Description != "" {
		t.Fatalf("historical detail: %+v %v", historicalBatch, err)
	}
	entries, err = s.List(ctx, otherActor, other, ListFilter{Page: 1})
	if err != nil || len(entries) != 0 {
		t.Fatalf("other workspace entries: %+v %v", entries, err)
	}
	var events int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = $1 AND event_type = 'transactions.entry.created'`, owner.WorkspaceID).Scan(&events); err != nil || events != 2 {
		t.Fatalf("audit: %d %v", events, err)
	}

	valid := Input{Description: "Valid", Direction: "in", AmountMinor: 1, Currency: "USD", PaymentMethod: "cash"}
	invalid := []Input{
		{Direction: "in", Currency: "USD", PaymentMethod: "cash"},
		{Direction: "sideways", AmountMinor: 1, Currency: "USD", PaymentMethod: "cash"},
		{Direction: "in", AmountMinor: 1, Currency: "US1", PaymentMethod: "cash"},
		{Direction: "in", AmountMinor: 1, Currency: "USD", PaymentMethod: "unknown"},
		{Direction: "in", AmountMinor: 1, Currency: "USD", PaymentMethod: "cash", OccurredOn: "2023-02-29"},
		{Direction: "in", AmountMinor: 1, Currency: "USD", PaymentMethod: "cash", Note: strings.Repeat("x", 1001)},
	}
	for i := range invalid {
		invalid[i].Description = "Valid"
	}
	for _, description := range []string{"", " \t ", strings.Repeat("é", 201), string([]byte{0xff})} {
		input := valid
		input.Description = description
		invalid = append(invalid, input)
	}
	clientService := clients.NewService(db, config.SQLite, auth, 10)
	client, err := clientService.Create(ctx, otherActor, other, clients.Input{DisplayName: "Other client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	wrongClient := valid
	wrongClient.ClientID = &client.ID
	linked := valid
	linked.ClientID = &client.ID
	linkedEntry, err := s.Create(ctx, otherActor, other, linked)
	if err != nil || linkedEntry.ClientID == nil || *linkedEntry.ClientID != client.ID {
		t.Fatalf("linked client: %+v %v", linkedEntry, err)
	}
	for _, tc := range []struct {
		name   string
		filter ListFilter
		want   EntryID
	}{
		{"client", ListFilter{Page: 1, ClientID: client.ID}, linkedEntry.ID},
		{"method", ListFilter{Page: 1, PaymentMethod: "card"}, backdated.ID},
		{"direction", ListFilter{Page: 1, Direction: "out"}, backdated.ID},
		{"date inclusive", ListFilter{Page: 1, From: "2020-02-29", To: "2020-02-29"}, backdated.ID},
		{"entry", ListFilter{Page: 1, EntryID: backdated.ID}, backdated.ID},
		{"entry with filters", ListFilter{Page: 1, EntryID: backdated.ID, Direction: "out", PaymentMethod: "card"}, backdated.ID},
		{"combined", ListFilter{Page: 1, Direction: "out", PaymentMethod: "card", From: "2020-02-29", To: "2020-02-29"}, backdated.ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace, principal := owner.WorkspaceID, actor
			if tc.name == "client" {
				workspace, principal = other, otherActor
			}
			got, err := s.List(ctx, principal, workspace, tc.filter)
			if err != nil || len(got) != 1 || got[0].ID != tc.want {
				t.Fatalf("filtered entries: %+v, %v", got, err)
			}
		})
	}
	filtered, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, Match: "any", Conditions: []ListCondition{
		{Field: "direction", Op: "equals", Value: "out"},
		{Field: "payment_method", Op: "equals", Value: "cash"},
	}, Sort: "date", Order: "asc"})
	if err != nil || len(filtered) != 2 || filtered[0].ID != backdated.ID || filtered[1].ID != entry.ID {
		t.Fatalf("combined filters and date order: %+v, %v", filtered, err)
	}
	filtered, err = s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, Search: "sale", Conditions: []ListCondition{
		{Field: "occurred_on", Op: "gte", Value: "2020-01-01"},
	}})
	if err != nil || len(filtered) != 1 || filtered[0].ID != entry.ID {
		t.Fatalf("search with filter: %+v, %v", filtered, err)
	}
	filtered, err = s.List(ctx, otherActor, other, ListFilter{Page: 1, Search: "Other client"})
	if err != nil || len(filtered) != 1 || filtered[0].ID != linkedEntry.ID {
		t.Fatalf("client search: %+v, %v", filtered, err)
	}
	firstPage, count, err := s.ListPage(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, Limit: 1, Sort: "date", Order: "asc"})
	if err != nil || count != 2 || len(firstPage) != 1 || firstPage[0].ID != backdated.ID {
		t.Fatalf("first page and count: %+v, %d, %v", firstPage, count, err)
	}
	secondPage, count, err := s.ListPage(ctx, actor, owner.WorkspaceID, ListFilter{Page: 2, Limit: 1, Sort: "date", Order: "asc"})
	if err != nil || count != 2 || len(secondPage) != 1 || secondPage[0].ID != entry.ID {
		t.Fatalf("second page and count: %+v, %d, %v", secondPage, count, err)
	}
	emptyPage, count, err := s.ListPage(ctx, actor, owner.WorkspaceID, ListFilter{Page: 3, Limit: 1, Conditions: []ListCondition{{Field: "direction", Op: "equals", Value: "out"}}})
	if err != nil || count != 1 || len(emptyPage) != 0 {
		t.Fatalf("filtered count beyond last page: %+v, %d, %v", emptyPage, count, err)
	}
	_, count, err = s.ListPage(ctx, otherActor, other, ListFilter{Page: 1, Limit: 1})
	if err != nil || count != 1 {
		t.Fatalf("workspace-scoped count: %d, %v", count, err)
	}
	for _, condition := range []ListCondition{
		{Field: "direction", Op: "equals", Value: "in' OR 1=1 --"},
		{Field: "occurred_on", Op: "between", Value: "2026-12-01,2026-01-01"},
		{Field: "client_id", Op: "equals", Value: "not-an-id"},
	} {
		if _, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, Conditions: []ListCondition{condition}}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid condition %+v: %v", condition, err)
		}
	}
	if got, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, ClientID: client.ID}); err != nil || len(got) != 0 {
		t.Fatalf("foreign client filter: %+v, %v", got, err)
	}
	if got, err := s.List(ctx, otherActor, other, ListFilter{Page: 1, EntryID: backdated.ID}); err != nil || len(got) != 0 {
		t.Fatalf("foreign entry filter: %+v, %v", got, err)
	}
	if got, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, EntryID: backdated.ID, Direction: "in"}); err != nil || len(got) != 0 {
		t.Fatalf("entry and direction filter: %+v, %v", got, err)
	}
	if got, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 2, EntryID: backdated.ID}); err != nil || len(got) != 0 {
		t.Fatalf("entry filter page 2: %+v, %v", got, err)
	}
	batch, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, []EntryID{entry.ID, backdated.ID, linkedEntry.ID, entry.ID})
	if err != nil || len(batch) != 2 || batch[entry.ID].AmountMinor != entry.AmountMinor || batch[entry.ID].Description != "Café" || batch[backdated.ID].ID != backdated.ID {
		t.Fatalf("batch entries: %+v, %v", batch, err)
	}
	if got, err := s.EntriesByID(ctx, otherActor, other, []EntryID{backdated.ID, linkedEntry.ID}); err != nil || len(got) != 1 || got[linkedEntry.ID].ID != linkedEntry.ID {
		t.Fatalf("batch workspace isolation: %+v, %v", got, err)
	}
	if got, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, nil); err != nil || len(got) != 0 {
		t.Fatalf("empty batch: %+v, %v", got, err)
	}
	hundred := make([]EntryID, 100)
	for i := range hundred {
		hundred[i] = entry.ID
	}
	if got, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, hundred); err != nil || len(got) != 1 {
		t.Fatalf("maximum batch: %+v, %v", got, err)
	}
	for _, ids := range [][]EntryID{{0}, {-1}, make([]EntryID, 101)} {
		if _, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, ids); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid batch: %v", err)
		}
	}
	for _, filter := range []ListFilter{
		{Page: 1, EntryID: -1},
		{Page: 1, Direction: "sideways"},
		{Page: 1, From: "2023-02-29"},
		{Page: 1, From: "2024-02-01", To: "2024-01-01"},
	} {
		if _, err := s.List(ctx, actor, owner.WorkspaceID, filter); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid filter %+v: %v", filter, err)
		}
	}
	invalid = append(invalid, wrongClient)
	for _, input := range invalid {
		if _, err := s.Create(ctx, actor, owner.WorkspaceID, input); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("invalid %+v: %v", input, err)
		}
	}
	if err := clientService.Delete(ctx, otherActor, other, client.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("delete linked client: %v", err)
	}
	retained, err := s.List(ctx, otherActor, other, ListFilter{Page: 1})
	if err != nil || len(retained) != 1 || retained[0].ID != linkedEntry.ID || retained[0].ClientID != nil || retained[0].AmountMinor != linkedEntry.AmountMinor {
		t.Fatalf("entry after client deletion: %+v %v", retained, err)
	}
	var bulkIDs []clients.ClientID
	for _, name := range []string{"Bulk one", "Bulk two"} {
		c, err := clientService.Create(ctx, otherActor, other, clients.Input{DisplayName: name}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		bulkIDs = append(bulkIDs, c.ID)
		linked := valid
		linked.ClientID = &c.ID
		if _, err := s.Create(ctx, otherActor, other, linked); err != nil {
			t.Fatal(err)
		}
	}
	if err := clientService.BulkDelete(ctx, otherActor, other, bulkIDs, identity.AuditContext{}); err != nil {
		t.Fatalf("bulk delete linked clients: %v", err)
	}
	retained, err = s.List(ctx, otherActor, other, ListFilter{Page: 1})
	if err != nil || len(retained) != 3 {
		t.Fatalf("entries after bulk deletion: %+v %v", retained, err)
	}
	for _, e := range retained {
		if e.ClientID != nil {
			t.Fatalf("client link retained after deletion: %+v", e)
		}
	}
	if _, err := db.ExecContext(ctx, `UPDATE transactions_payment_methods SET enabled = FALSE WHERE workspace_id = $1 AND name = 'cash'`, owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, valid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("disabled method: %v", err)
	}
	methods, err := s.Methods(ctx, actor, owner.WorkspaceID)
	if err != nil || !reflect.DeepEqual(methods, []string{"card", "check", "transfer"}) {
		t.Fatalf("disabled method list: %v %v", methods, err)
	}
	entries, err = s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
	if err != nil || len(entries) != 2 || entries[1].PaymentMethod != "cash" {
		t.Fatalf("historical method: %+v %v", entries, err)
	}
	for range 100 {
		if _, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Page entry", Direction: "in", AmountMinor: 1, Currency: "USD", PaymentMethod: "card"}); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
	if err != nil || len(first) != 100 {
		t.Fatalf("first page: %d %v", len(first), err)
	}
	second, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 2})
	if err != nil || len(second) != 2 || second[1].ID != entry.ID {
		t.Fatalf("second page: %+v %v", second, err)
	}
	if got, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, []EntryID{entry.ID, backdated.ID}); err != nil || len(got) != 2 {
		t.Fatalf("off-page batch: %+v, %v", got, err)
	}
	if _, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid page: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_members SET role = 'viewer' WHERE workspace_id = $1 AND user_id = $2`, owner.WorkspaceID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer list: %v", err)
	}
	if _, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, []EntryID{entry.ID}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer batch: %v", err)
	}
	if _, err := s.Methods(ctx, actor, owner.WorkspaceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer methods: %v", err)
	}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, valid); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer create: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`, owner.WorkspaceID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1}); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("outsider list: %v", err)
	}
}

func TestExpenseCategoryLifecycle(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "categories@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	categories, err := s.Categories(ctx, actor, owner.WorkspaceID)
	if err != nil || len(categories) != 0 {
		t.Fatalf("initial categories: %+v %v", categories, err)
	}
	category, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Supplies", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, " Supplies ", false); !errors.Is(err, ErrDuplicateCategory) {
		t.Fatalf("duplicate category create: %v", err)
	}
	in := Input{Description: "Category entry", Direction: "in", AmountMinor: 10, Currency: "USD", PaymentMethod: "cash", CategoryID: &category.ID}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("categorized money in: %v", err)
	}
	out := in
	out.Direction = "out"
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, out); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, category.ID, "Equipment", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, category.ID, "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, out); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("archived category: %v", err)
	}
	entries, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
	if err != nil || len(entries) != 1 || entries[0].CategoryName != "Equipment" {
		t.Fatalf("history: %+v %v", entries, err)
	}
	filtered, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, CategoryID: category.ID})
	if err != nil || len(filtered) != 1 || filtered[0].ID != entries[0].ID {
		t.Fatalf("archived category filter: %+v %v", filtered, err)
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCategory(ctx, otherActor, other, category.ID, "Wrong", false); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-workspace rename: %v", err)
	}
	if _, err := s.Create(ctx, otherActor, other, out); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-workspace category: %v", err)
	}
	var audits int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = $1 AND event_type LIKE 'transactions.category.%'`, owner.WorkspaceID).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("audit: %d %v", audits, err)
	}
	auditor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.WorkspaceExport)
	if err != nil {
		t.Fatal(err)
	}
	history, err := auth.ListAuditHistory(ctx, auditor, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	foundCreate, foundRename, foundArchive := false, false, false
	for _, event := range history {
		switch event.EventType {
		case "transactions.category.create":
			foundCreate = event.CategoryName == "Supplies"
		case "transactions.category.rename":
			foundRename = event.CategoryPreviousName == "Supplies" && event.CategoryNewName == "Equipment"
		case "transactions.category.archive":
			foundArchive = event.CategoryName == "Equipment"
		}
	}
	if !foundCreate || !foundRename || !foundArchive {
		t.Fatalf("category audit history: %+v", history)
	}
	reused, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Equipment", false)
	if err != nil {
		t.Fatalf("reuse archived category name: %v", err)
	}
	another, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Travel", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, another.ID, reused.Name, false); !errors.Is(err, ErrDuplicateCategory) {
		t.Fatalf("duplicate rename: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_members SET role = 'member' WHERE workspace_id = $1 AND user_id = $2`, owner.WorkspaceID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Forbidden", false); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member category create: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_members SET role = 'viewer' WHERE workspace_id = $1 AND user_id = $2`, owner.WorkspaceID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Categories(ctx, actor, owner.WorkspaceID); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("viewer list: %v", err)
	}
}

func TestCorrectionsAndConcurrentRefundCap(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "corrections@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	category, err := s.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Refunds", false)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Receipt", Direction: "in", AmountMinor: 100, Currency: "USD", PaymentMethod: "cash"})
	if err != nil {
		t.Fatal(err)
	}
	refund := Input{Description: "Refund", Direction: "out", AmountMinor: 60, Currency: "USD", PaymentMethod: "cash", RefundOfEntryID: receipt.ID, CategoryID: &category.ID}
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.Create(ctx, actor, owner.WorkspaceID, refund); results <- err }()
	}
	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		} else if !errors.Is(err, ErrInvalidInput) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent refunds succeeded: %d", successes)
	}
	entries, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
	if err != nil || len(entries) != 2 || entries[0].CategoryID == nil || *entries[0].CategoryID != category.ID || entries[0].CategoryName != category.Name {
		t.Fatalf("entries: %+v %v", entries, err)
	}
	voided, err := s.Void(ctx, actor, owner.WorkspaceID, entries[0].ID, "mistake")
	if err != nil || voided.Description != "Refund" || voided.VoidReason != "mistake" || voided.VoidedBy != owner.UserID || voided.VoidedAt == "" || voided.CategoryID == nil || *voided.CategoryID != category.ID || voided.CategoryName != category.Name {
		t.Fatalf("void: %+v %v", voided, err)
	}
	replacement := refund
	replacement.ReplacesEntryID = voided.ID
	replacement.AmountMinor = 100
	replaced, err := s.Create(ctx, actor, owner.WorkspaceID, replacement)
	if err != nil || replaced.CategoryID == nil || *replaced.CategoryID != category.ID || replaced.CategoryName != category.Name {
		t.Fatalf("replacement: %+v %v", replaced, err)
	}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, refund); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("refund cap: %v", err)
	}
	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	refund.CategoryID = nil
	if _, err := s.Create(ctx, otherActor, other, refund); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-workspace refund: %v", err)
	}
	if _, err := s.Void(ctx, otherActor, other, receipt.ID, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("cross-workspace void: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_members SET role = 'member' WHERE workspace_id = $1 AND user_id = $2`, owner.WorkspaceID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Void(ctx, actor, owner.WorkspaceID, receipt.ID, ""); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member void: %v", err)
	}
}
