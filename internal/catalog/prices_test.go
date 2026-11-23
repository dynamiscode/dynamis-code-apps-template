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
	s := NewService(db, auth)
	entry, err := s.Create(ctx, actor, owner.WorkspaceID, "Service", nil)
	if err != nil {
		t.Fatal(err)
	}
	amount := "0.123456789012345678"
	for _, option := range []PriceOption{{Mode: "fixed", Amount: &amount, Currency: "BTC", Variant: "Visit"}, {Mode: "manual", Currency: "USD", Variant: "Week"}} {
		if _, err := s.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID, option); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := s.List(ctx, actor, owner.WorkspaceID)
	if err != nil || len(entries) != 1 || len(entries[0].Prices) != 2 || *entries[0].Prices[0].Amount != amount || entries[0].Prices[1].Amount != nil {
		t.Fatalf("prices = %+v, %v", entries, err)
	}
	negative := "-1"
	for _, option := range []PriceOption{{Mode: "fixed", Currency: "USD", Variant: "Visit"}, {Mode: "fixed", Amount: &negative, Currency: "USD", Variant: "Visit"}, {Mode: "manual", Variant: "Visit"}, {Mode: "manual", Currency: "USD"}} {
		if _, err := s.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID, option); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid option %+v: %v", option, err)
		}
	}
}
