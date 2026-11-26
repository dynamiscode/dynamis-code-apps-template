package catalog

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/currency"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/testdb"
)

func TestPostgresCatalogCreateAndList(t *testing.T) {
	ctx := context.Background()
	db := testdb.Postgres(t)
	auth, err := identity.NewService(db, config.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "catalog-owner@example.com", Password: "long-enough-password", WorkspaceName: "Catalog"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(db, auth, nil)
	if err := currency.NewService(db, auth).SetCurrencies(ctx, actor, owner.WorkspaceID, []currency.Currency{{Code: "USD", Decimals: 2, Default: true}, {Code: "BHD", Decimals: 3}}); err != nil {
		t.Fatal(err)
	}
	minutes := 45
	entry, err := service.Create(ctx, actor, owner.WorkspaceID, "Consultation", &minutes, testPrices())
	if err != nil {
		t.Fatal(err)
	}
	amount := "0.123"
	price, err := service.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID.String(), PriceOption{Mode: "fixed", Amount: &amount, Currency: "BHD", Variant: "Visit"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Update(ctx, actor, owner.WorkspaceID, entry.ID, "Consultation", &minutes, entry.Revision); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateField(ctx, actor, owner.WorkspaceID, entry.ID, "name", "Stale", entry.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale inline update: %v", err)
	}
	{
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			results <- service.Update(ctx, actor, owner.WorkspaceID, entry.ID, "Consultation", &minutes, 2)
		}()
		go func() {
			<-start
			results <- service.UpdateField(ctx, actor, owner.WorkspaceID, entry.ID, "name", "Consultation", 2)
		}()
		close(start)
		successes, conflicts := 0, 0
		for range 2 {
			err := <-results
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrConflict):
				conflicts++
			default:
				t.Fatalf("concurrent entry edit: %v", err)
			}
		}
		if successes != 1 || conflicts != 1 {
			t.Fatalf("concurrent entry edits: %d successes, %d conflicts", successes, conflicts)
		}
	}
	if err := service.UpdatePrice(ctx, actor, owner.WorkspaceID, entry.ID, price.ID, PriceOption{Mode: "manual", Currency: "USD", Variant: "Visit"}); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdatePrice(ctx, actor, owner.WorkspaceID, entry.ID, price.ID, PriceOption{Mode: "fixed", Amount: &amount, Currency: "BHD", Variant: "Visit"}); err != nil {
		t.Fatal(err)
	}
	entries, err := service.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 1 || entries[0].Name != "Consultation" || entries[0].DurationMinutes == nil || *entries[0].DurationMinutes != minutes || len(entries[0].Prices) != 2 || entries[0].Prices[1].Amount == nil || *entries[0].Prices[1].Amount != amount {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, option := range entries[0].Prices {
		go func(priceID PriceID) {
			<-start
			results <- service.DeletePrice(ctx, actor, owner.WorkspaceID, entry.ID, priceID)
		}(option.ID)
	}
	close(start)
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrLastPrice)) || (second == nil && errors.Is(first, ErrLastPrice))) {
		t.Fatalf("concurrent deletes: %v, %v", first, second)
	}
	entries, err = service.List(ctx, actor, owner.WorkspaceID, false)
	if err != nil || len(entries) != 1 || len(entries[0].Prices) != 1 {
		t.Fatalf("remaining prices: %+v, %v", entries, err)
	}
}
