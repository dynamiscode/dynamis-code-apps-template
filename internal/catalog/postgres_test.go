package catalog

import (
	"context"
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
	service := NewService(db, auth)
	minutes := 45
	entry, err := service.Create(ctx, actor, owner.WorkspaceID, "Consultation", &minutes)
	if err != nil {
		t.Fatal(err)
	}
	amount := "0.123456789012345678"
	if _, err := service.AddPrice(ctx, actor, owner.WorkspaceID, entry.ID, PriceOption{Mode: "fixed", Amount: &amount, Currency: "BTC", Variant: "Visit"}); err != nil {
		t.Fatal(err)
	}
	entries, err := service.List(ctx, actor, owner.WorkspaceID)
	if err != nil || len(entries) != 1 || entries[0].Name != "Consultation" || entries[0].DurationMinutes == nil || *entries[0].DurationMinutes != minutes || len(entries[0].Prices) != 1 || entries[0].Prices[0].Amount == nil || *entries[0].Prices[0].Amount != amount {
		t.Fatalf("entries = %+v, %v", entries, err)
	}
}
