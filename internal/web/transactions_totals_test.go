package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/transactions"
)

func TestTransactionsTotalsWeb(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableTransactions(transactions.NewService(db, auth))
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO workspace_currencies (workspace_id, code, decimals, is_default) VALUES ($1, 'USD', 2, TRUE), ($1, 'EUR', 2, FALSE), ($1, 'GBP', 2, FALSE)`, workspace); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	for _, input := range []transactions.Input{
		{Description: "Receipt", Direction: "in", AmountMinor: 125, Currency: "USD", PaymentMethod: "cash", OccurredOn: "2026-01-02"},
		{Description: "Expense", Direction: "out", AmountMinor: 25, Currency: "USD", PaymentMethod: "cash", OccurredOn: "2026-01-03"},
		{Description: "Euro receipt", Direction: "in", AmountMinor: 40, Currency: "EUR", PaymentMethod: "card", OccurredOn: "2026-01-03"},
	} {
		if _, err := h.transactions.Create(ctx, actor, workspace, input); err != nil {
			t.Fatal(err)
		}
	}
	oldCategory, err := h.transactions.ChangeCategory(ctx, actor, workspace, 0, "Travel", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.transactions.Create(ctx, actor, workspace, transactions.Input{Description: "Old expense", Direction: "out", AmountMinor: 9223372036854775807, Currency: "GBP", PaymentMethod: "cash", OccurredOn: "2026-01-03", CategoryID: &oldCategory.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.transactions.ChangeCategory(ctx, actor, workspace, oldCategory.ID, "", true); err != nil {
		t.Fatal(err)
	}
	newCategory, err := h.transactions.ChangeCategory(ctx, actor, workspace, 0, "Travel", false)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := h.transactions.Create(ctx, actor, workspace, transactions.Input{Description: "New expense", Direction: "out", AmountMinor: 9223372036854775807, Currency: "GBP", PaymentMethod: "cash", OccurredOn: "2026-01-03", CategoryID: &newCategory.ID}); err != nil {
			t.Fatal(err)
		}
	}
	path := "/workspaces/" + workspace.String() + "/transactions/totals"
	routes := h.Routes()
	redirect := request(routes, http.MethodGet, path, nil, cookies, nil)
	if redirect.Code != http.StatusSeeOther || !strings.Contains(redirect.Header().Get("Location"), "period=this_month") {
		t.Fatalf("default period: %d %s", redirect.Code, redirect.Header().Get("Location"))
	}
	for _, preset := range []string{"this_month", "last_month"} {
		response := request(routes, http.MethodGet, path+"?period="+preset+"&start=1999-01-01&end=1999-01-02", nil, cookies, nil)
		location := response.Header().Get("Location")
		if response.Code != http.StatusSeeOther || location != path+"?period="+preset {
			t.Fatalf("preset redirect: %d %s", response.Code, location)
		}
		page := request(routes, http.MethodGet, location, nil, cookies, nil)
		if page.Code != http.StatusOK || strings.Contains(page.Body.String(), `value="1999-01-01"`) || strings.Contains(page.Body.String(), `value="1999-01-02"`) {
			t.Fatalf("preset dates: %d %s", page.Code, page.Body)
		}
	}
	custom := path + "?period=custom&start=2026-01-02&end=2026-01-03"
	page := request(routes, http.MethodGet, custom, nil, cookies, nil)
	for _, value := range []string{"USD", "EUR", "USD 1.25", "USD 0.25", "USD 1.00", "EUR 0.40", "Uncategorized", "2026-01-02", "2026-01-03", `value="custom"`, `for="transactions-totals-start"`, "workspace time zone"} {
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), value) {
			t.Fatalf("totals missing %q: %d %s", value, page.Code, page.Body)
		}
	}
	for _, value := range []string{"GBP 276,701,161,105,643,274.21", "GBP 184,467,440,737,095,516.14", "Archived", "Active"} {
		if !strings.Contains(page.Body.String(), value) {
			t.Fatalf("totals missing %q: %s", value, page.Body)
		}
	}
	if strings.Contains(page.Body.String(), "Category ID") || strings.Index(page.Body.String(), `aria-label="GBP"`) > strings.Index(page.Body.String(), `id="transactions-totals-period-heading"`) {
		t.Fatalf("report hierarchy or category labels: %s", page.Body)
	}
	invalid := request(routes, http.MethodGet, path+"?period=custom&start=2026-02-02&end=2026-01-01", nil, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Choose a valid period") || !strings.Contains(invalid.Body.String(), `value="2026-02-02"`) {
		t.Fatalf("invalid period: %d %s", invalid.Code, invalid.Body)
	}
	missingDates := request(routes, http.MethodGet, path+"?period=custom", nil, cookies, nil)
	for _, value := range []string{"Choose a valid period", "Both dates are required for a custom date range.", `aria-describedby="transactions-totals-custom-required"`} {
		if missingDates.Code != http.StatusUnprocessableEntity || !strings.Contains(missingDates.Body.String(), value) {
			t.Fatalf("missing custom dates: %d %s", missingDates.Code, missingDates.Body)
		}
	}
	spanish := request(routes, http.MethodGet, custom, nil, append(append([]*http.Cookie{}, cookies...), &http.Cookie{Name: "locale", Value: "es"}), nil)
	if spanish.Code != http.StatusOK || !strings.Contains(spanish.Body.String(), "Sin categoría") || !strings.Contains(spanish.Body.String(), "Activa") || !strings.Contains(spanish.Body.String(), "Archivada") {
		t.Fatalf("Spanish totals: %d %s", spanish.Code, spanish.Body)
	}
	for _, role := range []identity.Role{identity.Admin, identity.Member, identity.Viewer} {
		inv, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		user, _, err := auth.CreateInvitedLocalUser(ctx, inv.Secret, "long-enough-password", identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		s, err := auth.CreateSession(ctx, user, "local", "", time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		response := request(routes, http.MethodGet, custom, nil, []*http.Cookie{{Name: "session", Value: s.Secret}, {Name: "csrf", Value: s.CSRFSecret}}, nil)
		want := http.StatusForbidden
		if role == identity.Admin {
			want = http.StatusOK
		}
		if response.Code != want {
			t.Fatalf("%s: got %d want %d", role, response.Code, want)
		}
	}
}
