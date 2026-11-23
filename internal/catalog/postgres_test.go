package catalog

import (
	"context"
	"errors"
	"os"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestPostgresCatalogCreateAndList(t *testing.T) {
	url := os.Getenv("POSTGRES_TEST_URL")
	if url == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{Driver: config.Postgres, URL: url, MaxOpenConns: 4, MaxIdleConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, config.Postgres); err != nil {
		t.Fatal(err)
	}
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
	minutes := 45
	entry, err := service.Create(ctx, actor, owner.WorkspaceID, "Consultation", &minutes, testPrices())
	if err != nil {
		t.Fatal(err)
	}
	amount := "0.123456789012345678"
	price, err := service.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID.String(), PriceOption{Mode: "fixed", Amount: &amount, Currency: "BTC", Variant: "Visit"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Update(ctx, actor, owner.WorkspaceID, entry.ID, "Consultation", &minutes); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdatePrice(ctx, actor, owner.WorkspaceID, entry.ID, price.ID, PriceOption{Mode: "manual", Currency: "USD", Variant: "Visit"}); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdatePrice(ctx, actor, owner.WorkspaceID, entry.ID, price.ID, PriceOption{Mode: "fixed", Amount: &amount, Currency: "BTC", Variant: "Visit"}); err != nil {
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
