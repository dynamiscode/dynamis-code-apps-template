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

func TestTransactionCreateFieldDraftSurvivesCurrencySearch(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableTransactions(transactions.NewService(db, auth))
	h.clients = clients.NewService(db, config.SQLite, auth, 10)
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspace.String() + "/transactions"
	form := url.Values{"csrf": {session.CSRFSecret}, "description": {"Draft description"}, "direction": {"out"}, "amount": {"12.50"}, "currency": {"USD"}, "currency_search": {"usd"}, "payment_method": {"cash"}, "occurred_on": {"2026-01-02"}, "note": {"Draft note"}, "picker_action": {"currency"}}
	response := request(h.Routes(), http.MethodPost, path, form, cookies, nil)
	body := response.Body.String()
	if response.Code != http.StatusOK {
		t.Fatalf("search: %d %s", response.Code, body)
	}
	for _, want := range []string{
		`id="transactions-description" name="description" type="text" value="Draft description"`,
		`id="transactions-amount" name="amount" type="text" inputmode="decimal" required value="12.50"`,
		`id="transactions-date" name="occurred_on" type="date" required value="2026-01-02"`,
		`value="out" selected`,
		`value="USD" selected`,
		`name="currency_search" value="usd"`,
		`>Draft note</textarea>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("search response lost draft control %q", want)
		}
	}
}
