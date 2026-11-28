package appctl

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransactionsTotalsUsesREST(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workspaces/23456789/transactions/totals" ||
			r.URL.Query().Get("period") != "custom" || r.URL.Query().Get("start") != "2026-01-01" || r.URL.Query().Get("end") != "2026-01-31" ||
			r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"period":"custom","start":"2026-01-01","end":"2026-01-31","currencies":[{"currency":"USD","moneyIn":"125","moneyOut":"25","net":"100","methods":[],"categories":[]}]}`))
	}))
	defer server.Close()
	env := func(key string) string {
		switch key {
		case "APPCTL_BASE_URL":
			return server.URL
		case "APPCTL_TOKEN":
			return "secret"
		}
		return ""
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"transactions", "totals", "--workspace", "23456789", "--period", "custom", "--start", "2026-01-01", "--end", "2026-01-31"}, env, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"net":"100"`) {
		t.Fatalf("code %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestTransactionsCommandsUseREST(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch calls {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspaces/23456789/transactions/entries" {
				t.Errorf("create request = %s %s", r.Method, r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["description"] != "Sale" || body["direction"] != "in" || body["amountMinor"] != float64(125) || body["currency"] != "USD" || body["paymentMethod"] != "cash" || body["clientId"] != "23456789" || body["categoryId"] != "23456789" || body["note"] != "sale" {
				t.Errorf("create body = %v", body)
			}
			_, _ = w.Write([]byte(`{"id":"23456789","description":"Sale"}`))
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workspaces/23456789/transactions/entries" || r.URL.Query().Get("page") != "2" {
				t.Errorf("list request = %s %s", r.Method, r.URL.String())
			}
			for key, want := range map[string]string{"clientId": "23456789", "categoryId": "23456789", "paymentMethod": "cash", "direction": "out", "from": "2025-01-01", "to": "2025-01-31"} {
				if got := r.URL.Query().Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			_, _ = w.Write([]byte(`[{"id":"23456789"}]`))
		case 3:
			if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workspaces/23456789/transactions/methods" {
				t.Errorf("methods request = %s %s", r.Method, r.URL.Path)
			}
			_, _ = w.Write([]byte(`[{"id":"cash","name":"Cash","enabled":true}]`))
		case 4, 5, 6, 7:
			methods := []string{"GET", "POST", "PATCH", "DELETE"}
			paths := []string{"/api/v1/workspaces/23456789/transactions/categories", "/api/v1/workspaces/23456789/transactions/categories", "/api/v1/workspaces/23456789/transactions/categories/23456789", "/api/v1/workspaces/23456789/transactions/categories/23456789"}
			if r.Method != methods[calls-4] || r.URL.Path != paths[calls-4] {
				t.Errorf("category request = %s %s", r.Method, r.URL.Path)
			}
			if calls == 5 || calls == 6 {
				var body map[string]any
				want := "Rent"
				if calls == 6 {
					want = "Office"
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["name"] != want {
					t.Errorf("category body = %v, %v; want name %q", body, err, want)
				}
			}
			_, _ = w.Write([]byte(`{}`))
		case 8, 9:
			if r.URL.Path != "/api/v1/workspaces/23456789/transactions/methods" || (calls == 8 && r.Method != http.MethodPost) || (calls == 9 && r.Method != http.MethodPut) {
				t.Errorf("method request = %s %s", r.Method, r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if calls == 8 && (body["name"] != "Wire" || len(body) != 1) {
				t.Errorf("create method body = %v", body)
			}
			if calls == 9 && (body["id"] != "Wire" || body["name"] != "Bank wire" || body["enabled"] != false) {
				t.Errorf("update method body = %v", body)
			}
			_, _ = w.Write([]byte(`{"id":"Wire","name":"Bank wire","enabled":false}`))
		case 10:
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspaces/23456789/transactions/entries/23456789/void" {
				t.Errorf("void request = %s %s", r.Method, r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["reason"] != "correction" {
				t.Errorf("void body = %v, %v", body, err)
			}
			_, _ = w.Write([]byte(`{"id":"23456789"}`))
		case 11, 12:
			if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspaces/23456789/transactions/entries" {
				t.Errorf("correction request = %s %s", r.Method, r.URL.Path)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			key := "replacesEntryId"
			if calls == 12 {
				key = "refundOfEntryId"
			}
			if body[key] != "23456789" {
				t.Errorf("correction body = %v", body)
			}
			if calls == 12 && body["categoryId"] != "23456789" {
				t.Errorf("refund category = %v", body)
			}
			_, _ = w.Write([]byte(`{"id":"23456789"}`))
		}
	}))
	defer server.Close()
	env := func(key string) string {
		switch key {
		case "APPCTL_BASE_URL":
			return server.URL
		case "APPCTL_TOKEN":
			return "secret"
		}
		return ""
	}
	for i, args := range [][]string{
		{"transactions", "create", "--workspace", "23456789", "--description", "Sale", "--direction", "in", "--amount-minor", "125", "--currency", "USD", "--payment-method", "cash", "--client", "23456789", "--category", "23456789", "--note", "sale"},
		{"transactions", "list", "--workspace", "23456789", "--page", "2", "--client", "23456789", "--category", "23456789", "--payment-method", "cash", "--direction", "out", "--from", "2025-01-01", "--to", "2025-01-31"},
		{"transactions", "methods", "--workspace", "23456789"},
		{"transactions", "categories", "--workspace", "23456789"},
		{"transactions", "category-create", "--workspace", "23456789", "--name", "Rent"},
		{"transactions", "category-rename", "--workspace", "23456789", "--category", "23456789", "--name", "Office"},
		{"transactions", "category-archive", "--workspace", "23456789", "--category", "23456789"},
		{"transactions", "method-create", "--workspace", "23456789", "--name", "Wire"},
		{"transactions", "method-update", "--workspace", "23456789", "--id", "Wire", "--name", "Bank wire", "--enabled=false"},
		{"transactions", "void", "--workspace", "23456789", "--entry", "23456789", "--reason", "correction"},
		{"transactions", "create", "--workspace", "23456789", "--description", "Replacement", "--direction", "in", "--amount-minor", "125", "--currency", "USD", "--payment-method", "cash", "--replaces-entry", "23456789"},
		{"transactions", "create", "--workspace", "23456789", "--description", "Refund", "--direction", "out", "--amount-minor", "125", "--currency", "USD", "--payment-method", "cash", "--refund-of-entry", "23456789", "--category", "23456789"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, env, &stdout, &stderr); code != 0 || !json.Valid(stdout.Bytes()) {
			t.Fatalf("Run(%v) = %d, stdout %q, stderr %q", args, code, stdout.String(), stderr.String())
		}
		if i == 0 && !strings.Contains(stdout.String(), `"description":"Sale"`) {
			t.Fatalf("create output = %q", stdout.String())
		}
	}
	if calls != 12 {
		t.Fatalf("requests = %d", calls)
	}
}

func TestTransactionsRejectsInvalidFlags(t *testing.T) {
	env := func(key string) string {
		if key == "APPCTL_TOKEN" {
			return "secret"
		}
		return ""
	}
	for _, args := range [][]string{
		{"transactions", "list", "--workspace", "23456789", "--note", "x"},
		{"transactions", "list", "--workspace", "23456789", "--description", "x"},
		{"transactions", "create", "--workspace", "23456789", "--client", "bad!"},
		{"transactions", "methods", "--workspace", "invalid!"},
		{"transactions", "method-create", "--workspace", "23456789", "--name", "Wire", "--enabled=false"},
		{"transactions", "method-update", "--workspace", "23456789", "--id", "Wire", "--name", "Wire"},
		{"transactions", "methods", "--workspace", "23456789", "--name", "Wire"},
		{"transactions", "void", "--workspace", "23456789", "--entry", "bad!", "--reason", "x"},
		{"transactions", "void", "--workspace", "23456789", "--reason", "x"},
		{"transactions", "create", "--workspace", "23456789", "--refund-of-entry", "bad!"},
		{"transactions", "list", "--workspace", "23456789", "--replaces-entry", "23456789"},
		{"transactions", "list", "--workspace", "23456789", "--client", "bad!"},
		{"transactions", "methods", "--workspace", "23456789", "--from", "2025-01-01"},
		{"transactions", "list", "--workspace", "23456789", "--period", "this_month"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, env, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "usage") {
			t.Fatalf("Run(%v) = %d, stderr %q", args, code, stderr.String())
		}
	}
}
