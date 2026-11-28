package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/transactions"
)

func TestTransactionsFiltersAndClientHistory(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	clientService := clients.NewService(db, config.SQLite, auth, 10)
	h.EnableClients(clientService, nil)
	service := transactions.NewService(db, auth)
	h.EnableTransactions(service)
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	first, err := clientService.Create(ctx, actor, workspace, clients.Input{DisplayName: "First client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := clientService.Create(ctx, actor, workspace, clients.Input{DisplayName: "Second client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	category, err := service.ChangeCategory(ctx, actor, workspace, 0, "Travel", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []transactions.Input{
		{Description: "First matching expense", Direction: "out", AmountMinor: 731, Currency: "USD", PaymentMethod: "cash", OccurredOn: "2026-01-02", ClientID: &first.ID, CategoryID: &category.ID, Note: "First matching expense"},
		{Description: "Second receipt", Direction: "in", AmountMinor: 852, Currency: "USD", PaymentMethod: "card", OccurredOn: "2026-01-03", ClientID: &second.ID, Note: "Second receipt"},
	} {
		if _, err := service.Create(ctx, actor, workspace, input); err != nil {
			t.Fatal(err)
		}
	}
	otherWorkspace, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, otherWorkspace, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	otherClient, err := clientService.Create(ctx, otherActor, otherWorkspace, clients.Input{DisplayName: "Other workspace client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, otherActor, otherWorkspace, transactions.Input{Description: "Other workspace payment", Direction: "in", AmountMinor: 963, Currency: "USD", PaymentMethod: "cash", OccurredOn: "2026-01-02", ClientID: &otherClient.ID, Note: "Other workspace payment"}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	routes := h.Routes()
	transactionsPath := "/workspaces/" + workspace.String() + "/transactions"
	clientPath := "/workspaces/" + workspace.String() + "/clients/" + first.ID.String()
	filters := url.Values{"client_id": {first.ID.String()}, "category_id": {category.ID.String()}, "payment_method": {"cash"}, "direction": {"out"}, "from": {"2026-01-02"}, "to": {"2026-01-02"}}
	filtered := request(routes, http.MethodGet, transactionsPath+"?"+filters.Encode(), nil, cookies, nil)
	if filtered.Code != http.StatusOK || !strings.Contains(filtered.Body.String(), "First matching expense") || strings.Contains(filtered.Body.String(), "Second receipt") {
		t.Fatalf("filtered transactions: %d %s", filtered.Code, filtered.Body)
	}
	searched := request(routes, http.MethodGet, transactionsPath+"?q=matching&limit=1&sort=date&order=asc", nil, cookies, nil)
	if searched.Code != http.StatusOK || !strings.Contains(searched.Body.String(), "First matching expense") || strings.Contains(searched.Body.String(), "Second receipt") || !strings.Contains(searched.Body.String(), `list-page-size`) {
		t.Fatalf("searched transactions page: %d %s", searched.Code, searched.Body)
	}
	filters.Set("page", "2")
	page := request(routes, http.MethodGet, transactionsPath+"?"+filters.Encode(), nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), url.QueryEscape("client_id:equals:"+first.ID.String())) || !strings.Contains(page.Body.String(), url.QueryEscape("category_id:equals:"+category.ID.String())) || !strings.Contains(page.Body.String(), "page=1") {
		t.Fatalf("filtered pagination: %d %s", page.Code, page.Body)
	}
	invalidDates := request(routes, http.MethodGet, transactionsPath+"?from=2026-11-30&to=2026-11-01", nil, cookies, nil)
	for _, expected := range []string{"The start date must be on or before the end date.", "2026-11-30", "2026-11-01", `id="transactions-filter-date-error"`} {
		if invalidDates.Code != http.StatusBadRequest || !strings.Contains(invalidDates.Body.String(), expected) {
			t.Fatalf("invalid date recovery missing %q: %d %s", expected, invalidDates.Code, invalidDates.Body)
		}
	}
	for _, query := range []url.Values{
		{"client_id": {first.ID.String()}},
		{"category_id": {category.ID.String()}},
		{"payment_method": {"cash"}},
		{"direction": {"out"}},
		{"from": {"2026-01-02"}, "to": {"2026-01-02"}},
	} {
		got := request(routes, http.MethodGet, transactionsPath+"?"+query.Encode(), nil, cookies, nil)
		if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "First matching expense") || strings.Contains(got.Body.String(), "Second receipt") || strings.Contains(got.Body.String(), "Other workspace payment") {
			t.Fatalf("filter %v: %d %s", query, got.Code, got.Body)
		}
	}
	foreign := request(routes, http.MethodGet, transactionsPath+"?client_id="+otherClient.ID.String(), nil, cookies, nil)
	if foreign.Code != http.StatusOK || strings.Contains(foreign.Body.String(), "Other workspace payment") || strings.Contains(foreign.Body.String(), "Other workspace client") {
		t.Fatalf("foreign client filter: %d %s", foreign.Code, foreign.Body)
	}
	foreign = request(routes, http.MethodGet, "/workspaces/"+otherWorkspace.String()+"/clients/"+first.ID.String(), nil, cookies, nil)
	if foreign.Code == http.StatusOK || strings.Contains(foreign.Body.String(), "First matching expense") {
		t.Fatalf("foreign client page: %d %s", foreign.Code, foreign.Body)
	}
	for _, query := range []string{"direction=invalid", "from=2026-02-30", "client_id=bad", "category_id=bad"} {
		if got := request(routes, http.MethodGet, transactionsPath+"?"+query, nil, cookies, nil); got.Code != http.StatusBadRequest {
			t.Fatalf("%s: %d", query, got.Code)
		}
	}
	history := request(routes, http.MethodGet, clientPath, nil, cookies, nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), "First matching expense") || strings.Contains(history.Body.String(), "Second receipt") || !strings.Contains(history.Body.String(), "USD 7.31") || !strings.Contains(history.Body.String(), transactionsPath+"?client_id="+first.ID.String()+"#transactions-entries-heading") {
		t.Fatalf("client history: %d %s", history.Code, history.Body)
	}
	history = request(routes, http.MethodGet, clientPath+"?transactions_page=2&timeline_page=3&return=page%3D4", nil, cookies, nil)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), clientPath+"?return=page%3D4&amp;timeline_page=3&amp;transactions_page=1#client-transactions-heading") || !strings.Contains(history.Body.String(), `aria-label="Client transaction history"`) {
		t.Fatalf("history pagination: %d %s", history.Code, history.Body)
	}
	search := request(routes, http.MethodGet, transactionsPath+"?filter_field=client_id&filter_search=Second", nil, cookies, nil)
	if search.Code != http.StatusOK || !strings.Contains(search.Body.String(), `value="`+second.ID.String()+`">Second client</option>`) {
		t.Fatalf("client filter search: %d %s", search.Code, search.Body)
	}
	spanishCookies := append(append([]*http.Cookie(nil), cookies...), &http.Cookie{Name: "locale", Value: "es"})
	spanish := request(routes, http.MethodGet, clientPath, nil, spanishCookies, nil)
	if spanish.Code != http.StatusOK || !strings.Contains(spanish.Body.String(), "Historial de movimientos del cliente") {
		t.Fatalf("Spanish client history: %d %s", spanish.Code, spanish.Body)
	}
	spanish = request(routes, http.MethodGet, transactionsPath, nil, spanishCookies, nil)
	if spanish.Code != http.StatusOK || !strings.Contains(spanish.Body.String(), "Filtrar") || !strings.Contains(spanish.Body.String(), "Buscar movimientos") {
		t.Fatalf("Spanish filters: %d %s", spanish.Code, spanish.Body)
	}
	inviter, err := auth.Authorize(ctx, owner.UserID, workspace, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []identity.Role{identity.Member, identity.Viewer} {
		invite, err := auth.CreateInvitation(ctx, inviter, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		user, _, err := auth.CreateInvitedLocalUser(ctx, invite.Secret, "long-enough-password", identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		roleSession, err := auth.CreateSession(ctx, user, "local", "", time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		roleCookies := []*http.Cookie{{Name: "session", Value: roleSession.Secret}, {Name: "csrf", Value: roleSession.CSRFSecret}}
		got := request(routes, http.MethodGet, clientPath, nil, roleCookies, nil)
		for _, marker := range []string{"First matching expense", "USD 7.31", "client-transactions-heading"} {
			if got.Code != http.StatusOK || strings.Contains(got.Body.String(), marker) != (role == identity.Member) {
				t.Fatalf("%s history marker %q: %d %s", role, marker, got.Code, got.Body)
			}
		}
		if role == identity.Viewer {
			if denied := request(routes, http.MethodGet, transactionsPath, nil, roleCookies, nil); denied.Code != http.StatusForbidden {
				t.Fatalf("viewer transactions: %d", denied.Code)
			}
		}
	}
}
