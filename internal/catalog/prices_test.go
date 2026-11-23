package catalog

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestPriceOptions(t *testing.T) {
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "price@example.com", Password: "long-enough-password", WorkspaceName: "Prices"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth, nil)
	for _, prices := range [][]PriceOption{nil, {}, {{Mode: "manual", Currency: "USD", Variant: "Valid"}, {Mode: "fixed", Currency: "USD", Variant: "Invalid"}}} {
		if _, err := s.Create(ctx, actor, owner.WorkspaceID, "Invalid", nil, prices); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid create: %v", err)
		}
	}
	entries, err := s.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 0 {
		t.Fatalf("invalid create persisted entries: %+v, %v", entries, err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER reject_price BEFORE INSERT ON catalog_prices WHEN NEW.variant = 'Reject' BEGIN SELECT RAISE(ABORT, 'rejected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, actor, owner.WorkspaceID, "Rollback", nil, []PriceOption{{Mode: "manual", Currency: "USD", Variant: "Valid"}, {Mode: "manual", Currency: "USD", Variant: "Reject"}}); err == nil {
		t.Fatal("create unexpectedly succeeded")
	}
	entries, err = s.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed create persisted entries: %+v, %v", entries, err)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER reject_price`); err != nil {
		t.Fatal(err)
	}
	amount := "0.123456789012345678"
	entry, err := s.Create(ctx, actor, owner.WorkspaceID, "Service", nil, []PriceOption{{Mode: "fixed", Amount: &amount, Currency: "BTC", Variant: "Visit"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range []PriceOption{{Mode: "manual", Currency: "USD", Variant: "Week"}} {
		if _, err := s.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID.String(), option); err != nil {
			t.Fatal(err)
		}
	}
	entries, err = s.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 1 || len(entries[0].Prices) != 2 || *entries[0].Prices[0].Amount != amount || entries[0].Prices[1].Amount != nil {
		t.Fatalf("prices = %+v, %v", entries, err)
	}
	priceID := entries[0].Prices[0].ID
	manual := PriceOption{Mode: "manual", Currency: "EUR", Variant: "Week"}
	if err := s.UpdatePrice(ctx, actor, owner.WorkspaceID, entry.ID, priceID, manual); err != nil {
		t.Fatal(err)
	}
	fixed := PriceOption{Mode: "fixed", Amount: &amount, Currency: "BTC", Variant: "Visit"}
	if err := s.UpdatePrice(ctx, actor, owner.WorkspaceID, entry.ID, priceID, fixed); err != nil {
		t.Fatal(err)
	}
	negative := "-1"
	for _, invalid := range []PriceOption{{Mode: "fixed", Currency: "USD", Variant: "Visit"}, {Mode: "fixed", Amount: &negative, Currency: "USD", Variant: "Visit"}, {Mode: "manual", Variant: "Visit"}, {Mode: "manual", Currency: "USD"}} {
		if err := s.UpdatePrice(ctx, actor, owner.WorkspaceID, entry.ID, priceID, invalid); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid edit: %v", err)
		}
	}
	minutes := 45
	if err := s.Update(ctx, actor, owner.WorkspaceID, entry.ID, "Updated", &minutes); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, actor, owner.WorkspaceID, entry.ID, " ", nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank name: %v", err)
	}
	if err := s.UpdatePrice(ctx, actor, owner.WorkspaceID, entry.ID+1, priceID, manual); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong entry: %v", err)
	}
	if err := s.UpdatePrice(ctx, actor, owner.WorkspaceID+1, entry.ID, priceID, manual); !errors.Is(err, identity.ErrForbidden) {
		t.Fatalf("wrong workspace: %v", err)
	}
	if err := s.DeletePrice(ctx, actor, owner.WorkspaceID, entry.ID, entries[0].Prices[1].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePrice(ctx, actor, owner.WorkspaceID, entry.ID, priceID); !errors.Is(err, ErrLastPrice) {
		t.Fatalf("delete last price: %v", err)
	}
	if _, err := s.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID.String(), manual); err != nil {
		t.Fatal(err)
	}
	if err := s.SetArchived(ctx, actor, owner.WorkspaceID, entry.ID, true); err != nil {
		t.Fatal(err)
	}
	active, err := s.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(active) != 0 {
		t.Fatalf("active entries = %+v, %v", active, err)
	}
	archived, err := s.List(ctx, actor, owner.WorkspaceID, true)
	if err != nil || len(archived) != 1 || !archived[0].Archived || len(archived[0].Prices) != 2 || archived[0].Prices[0].Amount == nil || *archived[0].Prices[0].Amount != amount {
		t.Fatalf("archived prices = %+v, %v", archived, err)
	}
	if _, err := s.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID.String(), PriceOption{Mode: "manual", Currency: "USD", Variant: "Visit"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("add price to archived entry: %v", err)
	}
	if err := s.SetArchived(ctx, actor, owner.WorkspaceID, entry.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, option := range []PriceOption{{Mode: "fixed", Currency: "USD", Variant: "Visit"}, {Mode: "fixed", Amount: &negative, Currency: "USD", Variant: "Visit"}, {Mode: "manual", Variant: "Visit"}, {Mode: "manual", Currency: "USD"}} {
		if _, err := s.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID.String(), option); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid option %+v: %v", option, err)
		}
	}
	entries, err = s.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 1 || len(entries[0].Prices) != 2 {
		t.Fatalf("before concurrent deletes: %+v, %v", entries, err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, price := range entries[0].Prices {
		go func(priceID PriceID) {
			<-start
			results <- s.DeletePrice(ctx, actor, owner.WorkspaceID, entry.ID, priceID)
		}(price.ID)
	}
	close(start)
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrLastPrice)) || (second == nil && errors.Is(first, ErrLastPrice))) {
		t.Fatalf("concurrent deletes: %v, %v", first, second)
	}
	entries, err = s.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 1 || len(entries[0].Prices) != 1 {
		t.Fatalf("after concurrent deletes: %+v, %v", entries, err)
	}
	if err := s.Delete(ctx, actor, owner.WorkspaceID, entry.ID); err != nil {
		t.Fatal(err)
	}
	entries, err = s.List(ctx, actor, owner.WorkspaceID, true)
	if err != nil || len(entries) != 0 {
		t.Fatalf("deleted entries = %+v, %v", entries, err)
	}
}
