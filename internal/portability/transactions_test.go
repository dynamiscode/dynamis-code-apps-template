package portability_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/portability"
	"example.com/dynamis-code/apps-template/internal/transactions"
)

func TestTransactionUpdateAuditExportOmitsSensitiveValues(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "export-transactions@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	var clientKey int64
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := db.QueryRowContext(ctx, `INSERT INTO clients (workspace_id, display_name, version, created_at, updated_at) VALUES ($1, $2, 1, $3, $3) RETURNING id`, owner.WorkspaceID, "Private client", now).Scan(&clientKey); err != nil {
		t.Fatal(err)
	}
	clientID := id.EncodeKey(id.TypeClient, clientKey)
	service := transactions.NewService(db, auth)
	entry, err := service.Create(ctx, actor, owner.WorkspaceID, transactions.Input{Description: "Original private description", Direction: "in", AmountMinor: 100, Currency: "USD", PaymentMethod: "cash", Note: "Original private note"})
	if err != nil {
		t.Fatal(err)
	}
	updates := []struct{ field, value string }{
		{"description", "Updated private description"},
		{"note", "Updated private note"},
		{"client_id", clientID},
	}
	for _, update := range updates {
		entry, err = service.Update(ctx, actor, owner.WorkspaceID, entry.ID, transactions.UpdateInput{ExpectedRevision: entry.Revision, Field: update.field, Value: update.value})
		if err != nil {
			t.Fatalf("update %s: %v", update.field, err)
		}
	}
	exporter := portability.NewService(db, config.SQLite, auth, 100, 1024*1024, nil)
	encoded, err := exporter.Export(ctx, actor, owner.WorkspaceID, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var result portability.Export
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool)
	for _, event := range result.AuditEvents {
		if event.EventType != "transactions.entry.updated" {
			continue
		}
		var metadata map[string]any
		if err := json.Unmarshal(event.Metadata, &metadata); err != nil {
			t.Fatal(err)
		}
		field, _ := metadata["field"].(string)
		if found[field] || len(metadata) != 3 || metadata["changed"] != true {
			t.Fatalf("unexpected update metadata: %s", event.Metadata)
		}
		for i, update := range updates {
			if field == update.field {
				if metadata["revision"] != float64(i+2) {
					t.Fatalf("revision for %s: %s", field, event.Metadata)
				}
				found[field] = true
				break
			}
		}
	}
	for _, update := range updates {
		if !found[update.field] {
			t.Errorf("missing audit for %s", update.field)
		}
	}
}
