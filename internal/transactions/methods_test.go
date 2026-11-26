package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/testdb"
)

func TestMethodsSQLite(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.SQLite, SQLitePath: filepath.Join(t.TempDir(), "transactions.db"), MaxOpenConns: 4, MaxIdleConns: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	checkMethods(t, db, config.SQLite)
}

func TestMethodsPostgres(t *testing.T) {
	checkMethods(t, testdb.Postgres(t), config.Postgres)
}

func checkMethods(t *testing.T, db *sql.DB, driver config.DatabaseDriver) {
	t.Helper()
	ctx := context.Background()
	auth, err := identity.NewService(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "methods@example.com", Password: "long-enough-password", WorkspaceName: "Methods"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	entry, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Entry", Direction: "in", AmountMinor: 5, Currency: "USD", PaymentMethod: "cash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveMethod(ctx, actor, owner.WorkspaceID, "cash", "Notes", false); err != nil {
		t.Fatal(err)
	}
	readOnly := actor
	readOnly.Permissions = map[identity.Permission]bool{identity.ResourcesRead: true}
	visible, err := s.ListMethods(ctx, readOnly, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, method := range visible {
		if method.ID == "cash" && method.Name == "Notes" && !method.Enabled {
			found = true
		}
	}
	if !found {
		t.Fatalf("read-only owner cannot see disabled method: %+v", visible)
	}
	if _, err := s.SaveMethod(ctx, readOnly, owner.WorkspaceID, "cash", "Notes", true); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("read-only owner managed method: %v", err)
	}
	var metadata string
	if err := db.QueryRowContext(ctx, `SELECT metadata FROM audit_events WHERE workspace_id = $1 AND event_type = 'transactions.method.updated' ORDER BY id DESC LIMIT 1`, owner.WorkspaceID).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var event struct {
		MethodID string `json:"methodId"`
		Before   Method `json:"before"`
		After    Method `json:"after"`
	}
	if err := json.Unmarshal([]byte(metadata), &event); err != nil {
		t.Fatal(err)
	}
	if event.MethodID != "cash" || event.Before.Name != "cash" || !event.Before.Enabled || event.After.Name != "Notes" || event.After.Enabled {
		t.Fatalf("method audit: %+v", event)
	}
	history, err := auth.ListAuditHistory(ctx, actor, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, item := range history {
		if item.EventType == "transactions.method.updated" && item.TargetLabel == "cash" {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit history did not identify the method: %+v", history)
	}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Entry", Direction: "in", AmountMinor: 5, Currency: "USD", PaymentMethod: "cash"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("disabled method accepted: %v", err)
	}
	entries, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1})
	if err != nil || len(entries) != 1 || entries[0].ID != entry.ID || entries[0].PaymentMethodName != "Notes" {
		t.Fatalf("historical entry: %+v, %v", entries, err)
	}
	if _, err := s.SaveMethod(ctx, actor, owner.WorkspaceID, "cash", "Notes", true); err != nil {
		t.Fatal(err)
	}
	created, err := s.SaveMethod(ctx, actor, owner.WorkspaceID, "", "cash", true)
	if err != nil || created.ID == "cash" {
		t.Fatalf("add renamed default name: %+v, %v", created, err)
	}
	for _, method := range []struct{ id, name string }{{"cash", "Notes"}, {"check", "check"}, {"transfer", "transfer"}} {
		if _, err := s.SaveMethod(ctx, actor, owner.WorkspaceID, method.id, method.name, false); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, method := range []Method{{ID: "card", Name: "card"}, created} {
		wg.Add(1)
		go func(m Method) {
			defer wg.Done()
			<-start
			_, err := s.SaveMethod(ctx, actor, owner.WorkspaceID, m.ID, m.Name, false)
			errs <- err
		}(method)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent disables: %d succeeded, want 1", successes)
	}
	methods, err := s.Methods(ctx, actor, owner.WorkspaceID)
	if err != nil || len(methods) != 1 {
		t.Fatalf("enabled methods after concurrency: %v, %v", methods, err)
	}
	remaining := methods[0]
	all, err := s.ListMethods(ctx, actor, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range all {
		if method.ID == remaining {
			if _, err := s.SaveMethod(ctx, actor, owner.WorkspaceID, method.ID, method.Name, false); !errors.Is(err, ErrLastMethod) {
				t.Fatalf("last method: %v", err)
			}
		}
	}
	for i := range 103 {
		if _, err := db.ExecContext(ctx, `INSERT INTO transactions_payment_methods (workspace_id, name, display_name, enabled) VALUES ($1, $2, $3, $4)`, owner.WorkspaceID, fmt.Sprintf("historical-%03d", i), fmt.Sprintf("Historical %03d", i), i != 0); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ListMethodPage(ctx, actor, owner.WorkspaceID, "", false)
	if err != nil || len(first) != 101 {
		t.Fatalf("first method page = %d, %v", len(first), err)
	}
	second, err := s.ListMethodPage(ctx, actor, owner.WorkspaceID, first[99].ID, false)
	if err != nil || len(second) != len(all)+103-100 || second[0].ID != first[100].ID {
		t.Fatalf("second method page = %+v, %v", second, err)
	}
	enabled, err := s.ListMethodPage(ctx, actor, owner.WorkspaceID, "", true)
	if err != nil || len(enabled) != 101 {
		t.Fatalf("enabled page = %d, %v", len(enabled), err)
	}
	for _, method := range enabled {
		if !method.Enabled {
			t.Fatalf("enabled page includes disabled method: %+v", method)
		}
	}
	unauthorized := actor
	unauthorized.Permissions = map[identity.Permission]bool{}
	if _, err := s.ListMethodPage(ctx, unauthorized, owner.WorkspaceID, "", false); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("unauthorized method page: %v", err)
	}
}
