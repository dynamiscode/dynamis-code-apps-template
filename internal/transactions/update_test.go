package transactions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/testdb"
)

func TestUpdateSQLite(t *testing.T) {
	db, err := database.Open(context.Background(), config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	testUpdate(t, db, config.SQLite)
}

func TestUpdatePostgres(t *testing.T) { testUpdate(t, testdb.Postgres(t), config.Postgres) }

func testUpdate(t *testing.T, db *sql.DB, driver config.DatabaseDriver) {
	t.Helper()
	ctx := context.Background()
	auth, err := identity.NewService(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "update@example.com", Password: "long-enough-password", WorkspaceName: "Transactions"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := currency.NewService(db, auth).SetCurrencies(ctx, actor, owner.WorkspaceID, []currency.Currency{{Code: "USD", Decimals: 2, Default: true}, {Code: "JPY", Decimals: 0}, {Code: "KWD", Decimals: 3}}); err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	entry, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Original", Direction: "in", AmountMinor: 1200, Currency: "USD", PaymentMethod: "cash"})
	if err != nil {
		t.Fatal(err)
	}
	update := func(id EntryID, revision int64, field, value string) (Entry, error) {
		return s.Update(ctx, actor, owner.WorkspaceID, id, UpdateInput{ExpectedRevision: revision, Field: field, Value: value})
	}
	updated, err := update(entry.ID, entry.Revision, "description", "  Corrected  ")
	if err != nil || updated.ID != entry.ID || updated.Description != "Corrected" || updated.Revision != entry.Revision+1 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := update(entry.ID, entry.Revision, "note", "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if _, err := update(entry.ID, updated.Revision, "description", "  "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty description: %v", err)
	}
	unchanged, err := update(entry.ID, updated.Revision, "description", "Corrected")
	if err != nil || unchanged.Revision != updated.Revision {
		t.Fatalf("no-op: %+v %v", unchanged, err)
	}
	var audits int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE workspace_id = $1 AND event_type = 'transactions.entry.updated' AND target_key = $2`, owner.WorkspaceID, int64(entry.ID)).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit count: %d %v", audits, err)
	}
	var metadata string
	if err := db.QueryRowContext(ctx, `SELECT metadata FROM audit_events WHERE event_type = 'transactions.entry.updated' AND target_key = $1`, int64(entry.ID)).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(metadata), &detail); err != nil || detail["field"] != "description" || detail["revision"] != float64(updated.Revision) || detail["changed"] != true || strings.Contains(metadata, "Original") || strings.Contains(metadata, "Corrected") {
		t.Fatalf("private description audit: %s %v", metadata, err)
	}
	list, err := s.List(ctx, actor, owner.WorkspaceID, ListFilter{Page: 1, EntryID: entry.ID})
	if err != nil || len(list) != 1 || list[0].Description != "Corrected" || list[0].Revision != updated.Revision {
		t.Fatalf("persisted: %+v %v", list, err)
	}

	updated, err = update(entry.ID, updated.Revision, "currency", "JPY")
	if err != nil || updated.AmountMinor != 12 || updated.Currency != "JPY" {
		t.Fatalf("USD to JPY: %+v %v", updated, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT metadata FROM audit_events WHERE event_type = 'transactions.entry.updated' AND target_key = $1 ORDER BY id DESC LIMIT 1`, int64(entry.ID)).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	detail = nil
	if err := json.Unmarshal([]byte(metadata), &detail); err != nil {
		t.Fatal(err)
	}
	before, beforeOK := detail["before"].(map[string]any)
	after, afterOK := detail["after"].(map[string]any)
	if !beforeOK || !afterOK || before["currency"] != "USD" || before["amountMinor"] != float64(1200) || after["currency"] != "JPY" || after["amountMinor"] != float64(12) || detail["revision"] != float64(updated.Revision) {
		t.Fatalf("currency audit: %s", metadata)
	}
	updated, err = update(entry.ID, updated.Revision, "currency", "KWD")
	if err != nil || updated.AmountMinor != 12000 {
		t.Fatalf("JPY to KWD: %+v %v", updated, err)
	}
	updated, err = update(entry.ID, updated.Revision, "amount_minor", "12001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := update(entry.ID, updated.Revision, "currency", "USD"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("lossy rescale: %v", err)
	}
	updated, err = update(entry.ID, updated.Revision, "amount_minor", "9223372036854775807")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := update(entry.ID, updated.Revision, "currency", "JPY"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("lossy maximum: %v", err)
	}
	large, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Large", Direction: "in", AmountMinor: 9223372036854775807, Currency: "JPY", PaymentMethod: "cash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := update(large.ID, large.Revision, "currency", "KWD"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overflow rescale: %v", err)
	}

	receipt, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Receipt", Direction: "in", AmountMinor: 1000, Currency: "USD", PaymentMethod: "cash"})
	if err != nil {
		t.Fatal(err)
	}
	refund, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Refund", Direction: "out", AmountMinor: 700, Currency: "USD", PaymentMethod: "cash", RefundOfEntryID: receipt.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id           EntryID
		revision     int64
		field, value string
	}{
		{receipt.ID, receipt.Revision, "amount_minor", "699"},
		{receipt.ID, receipt.Revision, "direction", "out"},
		{receipt.ID, receipt.Revision, "currency", "JPY"},
		{refund.ID, refund.Revision, "amount_minor", "1001"},
		{refund.ID, refund.Revision, "direction", "in"},
		{refund.ID, refund.Revision, "currency", "JPY"},
	} {
		if _, err := update(tc.id, tc.revision, tc.field, tc.value); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("refund invariant %s=%s: %v", tc.field, tc.value, err)
		}
	}
	refund, err = update(refund.ID, refund.Revision, "amount_minor", "600")
	if err != nil || refund.AmountMinor != 600 {
		t.Fatalf("refund reduction: %+v %v", refund, err)
	}
	if _, err := s.Void(ctx, actor, owner.WorkspaceID, refund.ID, "mistake"); err != nil {
		t.Fatal(err)
	}
	if _, err := update(refund.ID, refund.Revision, "note", "voided"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("voided update: %v", err)
	}
	clientService := clients.NewService(db, driver, auth, 10)
	client, err := clientService.Create(ctx, actor, owner.WorkspaceID, clients.Input{DisplayName: "Private client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Linked entry", Direction: "in", AmountMinor: 100, Currency: "USD", PaymentMethod: "cash", ClientID: &client.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := clientService.Delete(ctx, actor, owner.WorkspaceID, client.ID, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if _, err := update(linked.ID, linked.Revision, "note", "stale client"); !errors.Is(err, ErrConflict) {
		t.Fatalf("client unlink must invalidate revision: %v", err)
	}
	unlinked, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, []EntryID{linked.ID})
	if err != nil || unlinked[linked.ID].ClientID != nil || unlinked[linked.ID].Revision != linked.Revision+1 {
		t.Fatalf("client unlink revision: %+v %v", unlinked, err)
	}
	linked = unlinked[linked.ID]
	linked, err = update(linked.ID, linked.Revision, "note", "private note")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT metadata FROM audit_events WHERE event_type = 'transactions.entry.updated' AND target_key = $1 ORDER BY id DESC LIMIT 1`, int64(linked.ID)).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, "private note") || strings.Contains(metadata, client.ID.String()) || strings.Contains(metadata, "before") || strings.Contains(metadata, "after") {
		t.Fatalf("private note audit: %s", metadata)
	}
	secondClient, err := clientService.Create(ctx, actor, owner.WorkspaceID, clients.Input{DisplayName: "Second private client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err = update(linked.ID, linked.Revision, "client_id", secondClient.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT metadata FROM audit_events WHERE event_type = 'transactions.entry.updated' AND target_key = $1 ORDER BY id DESC LIMIT 1`, int64(linked.ID)).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadata, secondClient.ID.String()) || strings.Contains(metadata, "before") || strings.Contains(metadata, "after") {
		t.Fatalf("private client audit: %s", metadata)
	}
	linked, err = update(linked.ID, linked.Revision, "client_id", "")
	if err != nil {
		t.Fatal(err)
	}
	unlinked, err = s.EntriesByID(ctx, actor, owner.WorkspaceID, []EntryID{linked.ID})
	if err != nil || unlinked[linked.ID].Revision != linked.Revision {
		t.Fatalf("explicit unlink double increment: %+v %v", unlinked, err)
	}
	if driver == config.Postgres {
		raceClient, err := clientService.Create(ctx, actor, owner.WorkspaceID, clients.Input{DisplayName: "Concurrent client"}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		raceEntry, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Concurrent client entry", Direction: "in", AmountMinor: 10, Currency: "USD", PaymentMethod: "cash", ClientID: &raceClient.ID})
		if err != nil {
			t.Fatal(err)
		}
		clientStart := make(chan struct{})
		updateResult := make(chan error, 1)
		deleteResult := make(chan error, 1)
		go func() {
			<-clientStart
			_, err := update(raceEntry.ID, raceEntry.Revision, "note", "concurrent edit")
			updateResult <- err
		}()
		go func() {
			<-clientStart
			deleteResult <- clientService.Delete(ctx, actor, owner.WorkspaceID, raceClient.ID, identity.AuditContext{})
		}()
		close(clientStart)
		updateErr, deleteErr := <-updateResult, <-deleteResult
		if updateErr != nil && !errors.Is(updateErr, ErrConflict) {
			t.Fatalf("concurrent client update: %v", updateErr)
		}
		if deleteErr != nil {
			var state interface{ SQLState() string }
			if !errors.As(deleteErr, &state) || state.SQLState() != "40001" {
				t.Fatalf("concurrent client delete: %v", deleteErr)
			}
			if err := clientService.Delete(ctx, actor, owner.WorkspaceID, raceClient.ID, identity.AuditContext{}); err != nil {
				t.Fatal(err)
			}
		}
		raceEntries, err := s.EntriesByID(ctx, actor, owner.WorkspaceID, []EntryID{raceEntry.ID})
		if err != nil || raceEntries[raceEntry.ID].ClientID != nil || raceEntries[raceEntry.ID].Revision <= raceEntry.Revision || (updateErr == nil && raceEntries[raceEntry.ID].Note != "concurrent edit") {
			t.Fatalf("concurrent client erasure: %+v %v", raceEntries, err)
		}
		raceReceipt, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Concurrent receipt", Direction: "in", AmountMinor: 1000, Currency: "USD", PaymentMethod: "cash"})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, err := update(raceReceipt.ID, raceReceipt.Revision, "amount_minor", "500")
			results <- err
		}()
		go func() {
			<-start
			_, err := s.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Concurrent refund", Direction: "out", AmountMinor: 700, Currency: "USD", PaymentMethod: "cash", RefundOfEntryID: raceReceipt.ID})
			results <- err
		}()
		close(start)
		successes := 0
		for range 2 {
			err := <-results
			if err == nil {
				successes++
			} else if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("concurrent update/refund: %v", err)
			}
		}
		if successes != 1 {
			t.Fatalf("concurrent update/refund successes: %d", successes)
		}
		var amount, refunded int64
		if err := db.QueryRowContext(ctx, `SELECT amount_minor, COALESCE((SELECT SUM(r.amount_minor) FROM transactions_entries r WHERE r.workspace_id = $1 AND r.refund_of_entry_id = $2 AND r.voided_at IS NULL), 0) FROM transactions_entries WHERE workspace_id = $1 AND id = $2`, owner.WorkspaceID, raceReceipt.ID).Scan(&amount, &refunded); err != nil || refunded > amount {
			t.Fatalf("concurrent refund invariant: amount=%d refunds=%d err=%v", amount, refunded, err)
		}
	}

	other, err := auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := auth.Authorize(ctx, owner.UserID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, otherActor, other, receipt.ID, UpdateInput{ExpectedRevision: receipt.Revision, Field: "note", Value: "foreign"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("workspace isolation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspace_members SET role = 'member' WHERE workspace_id = $1 AND user_id = $2`, owner.WorkspaceID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := update(receipt.ID, receipt.Revision, "note", "member"); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("member edit: %v", err)
	}
}
