package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseProfilesTransactionsRequiresIdentityAndClients(t *testing.T) {
	for _, value := range []string{"Core,Identity,Agent,Transactions", "Core,Agent,Clients,Transactions"} {
		_, err := parseProfiles(value)
		if err == nil || !strings.Contains(err.Error(), "Transactions profile requires Identity and Clients") {
			t.Fatalf("parseProfiles(%q) error = %v", value, err)
		}
	}
	got, err := parseProfiles("Transactions,Clients,Agent,Identity,Core")
	if err != nil || strings.Join(got, ",") != "Core,Identity,Agent,Clients,Transactions" {
		t.Fatalf("parseProfiles() = %v, %v", got, err)
	}
}
func TestTransactionsPathsIncludeBrowserAndMigrations(t *testing.T) {
	for _, path := range []string{
		"internal/browsertest/transactions_test.go",
		"internal/platform/database/transactions_migrate_test.go",
		"internal/platform/database/migrations/000057_transactions.sql",
		"internal/platform/database/migrations/000058_transactions_client_erasure.sql",
		"internal/platform/database/migrations/000059_transactions_corrections.sql",
		"internal/platform/database/migrations/000060_transactions_method_names.sql",
		"internal/platform/database/migrations/000061_transactions_categories.sql",
		"internal/platform/database/migrations/000062_transaction_description.sql",
	} {
		if !transactionsPath(path) {
			t.Errorf("%s must be omitted when Transactions is disabled", path)
		}
	}
}

func TestTransactionsPruningRejectsHistoricalViews(t *testing.T) {
	source := "const (\n\tListClients = \"clients\"\n\tListTransactions = \"transactions\"\n)\n" +
		"func canReadList(principal identity.Principal, listKey string) bool {\n\treturn listKey != ListTransactions || identity.CanReadTransactions(principal)\n}\n" +
		"case ListClients, ListItems, ListFiles, ListMembers, ListTransactions:\n"
	pruned := string(disableTransactionsSource("internal/listviews/service.go", []byte(source)))
	if strings.Contains(pruned, "ListTransactions") || strings.Contains(pruned, "CanReadTransactions") ||
		!strings.Contains(pruned, `return listKey != "transactions"`) {
		t.Fatalf("unexpected list view pruning: %s", pruned)
	}
}

func TestTransactionsProfileGeneration(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	for _, enabled := range []bool{false, true} {
		name := "without-transactions"
		profiles := "Core,Identity,Agent,Clients"
		if enabled {
			name = "with-transactions"
			profiles += ",Transactions"
		}
		t.Run(name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "app")
			err := run([]string{
				"-template-dir", root, "-output", output, "-name", "Transactions Application",
				"-module", "example.com/acme/transactions-app",
				"-repository", "https://github.com/acme/transactions-app",
				"-security-url", "https://github.com/acme/transactions-app/security/advisories/new",
				"-maintainer", "@acme/platform", "-profiles", profiles,
				"-source", "https://example.com/acme/template", "-commit", strings.Repeat("a", 40),
			}, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			lock, err := os.ReadFile(filepath.Join(output, "template.lock"))
			if err != nil || strings.Contains(string(lock), `"Transactions"`) != enabled {
				t.Fatalf("template.lock transactions selection: %s, %v", lock, err)
			}
			for _, path := range []string{"internal/transactions", "internal/httpapi/transactions.go", "internal/web/transactions.go", "internal/mcpserver/transactions.go", "internal/appctl/transactions.go", "internal/browsertest/transactions_test.go", "internal/platform/database/transactions_migrate_test.go", "internal/platform/database/migrations/000057_transactions.sql", "internal/platform/database/migrations/000058_transactions_client_erasure.sql", "internal/platform/database/migrations/000059_transactions_corrections.sql", "internal/platform/database/migrations/000060_transactions_method_names.sql", "internal/platform/database/migrations/000061_transactions_categories.sql", "internal/platform/database/migrations/000062_transaction_description.sql"} {
				_, err := os.Stat(filepath.Join(output, path))
				if enabled && err != nil || !enabled && !os.IsNotExist(err) {
					t.Errorf("%s: enabled=%v, error=%v", path, enabled, err)
				}
			}
			if !enabled {
				page, err := os.ReadFile(filepath.Join(output, "internal/web/clients.templ"))
				if err != nil || !strings.Contains(string(page), "@clientDetail(data)") || strings.Contains(string(page), "client-transactions-heading") {
					t.Fatalf("client page without Transactions: %v", err)
				}
			}
			packages := []string{"./internal/bootstrap", "./internal/httpapi", "./internal/web", "./internal/mcpserver", "./internal/appctl"}
			if enabled {
				packages = append(packages, "./internal/transactions")
			}
			if err := runCommand(output, "go", append([]string{"test"}, packages...)...); err != nil {
				t.Fatalf("generated application tests: %v", err)
			}
		})
	}
}
