package transactions

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/testdb"
)

func TestTotalsExactSQLite(t *testing.T) {
	db, err := database.Open(context.Background(), config.Database{Driver: config.SQLite, SQLitePath: ":memory:", MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db, config.SQLite); err != nil {
		t.Fatal(err)
	}
	testTotalsExact(t, db, config.SQLite)
}

func TestTotalsExactPostgres(t *testing.T) {
	testTotalsExact(t, testdb.Postgres(t), config.Postgres)
}

func testTotalsExact(t *testing.T, db *sql.DB, driver config.DatabaseDriver) {
	t.Helper()
	ctx := context.Background()
	auth, err := identity.NewService(db, driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "totals@example.com", Password: "long-enough-password", WorkspaceName: "Totals"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE workspaces SET time_zone = 'Pacific/Kiritimati' WHERE id = $1`, owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	localDate, err := today(ctx, db, owner.WorkspaceID, time.Date(2024, 1, 31, 10, 30, 0, 0, time.UTC))
	if err != nil || localDate != "2024-02-01" {
		t.Fatalf("workspace boundary: %q %v", localDate, err)
	}
	service := NewService(db, auth)
	old, err := service.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Rent", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Old rent", Direction: "out", AmountMinor: math.MaxInt64, Currency: "USD", PaymentMethod: "cash", OccurredOn: "2024-01-01", CategoryID: &old.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangeCategory(ctx, actor, owner.WorkspaceID, old.ID, "", true); err != nil {
		t.Fatal(err)
	}
	current, err := service.ChangeCategory(ctx, actor, owner.WorkspaceID, 0, "Rent", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Create(ctx, actor, owner.WorkspaceID, Input{Description: "Rent", Direction: "out", AmountMinor: math.MaxInt64, Currency: "USD", PaymentMethod: "cash", OccurredOn: "2024-01-31", CategoryID: &current.ID})
	if err != nil {
		t.Fatal(err)
	}
	totals, err := service.Totals(ctx, actor, owner.WorkspaceID, "custom", "2024-01-01", "2024-01-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(totals.Currencies) != 1 {
		t.Fatalf("currencies: %+v", totals.Currencies)
	}
	got := totals.Currencies[0]
	if got.MoneyOut != "18446744073709551614" || got.Net != "-18446744073709551614" || len(got.Methods) != 1 || got.Methods[0].MoneyOut != got.MoneyOut {
		t.Fatalf("exact totals: %+v", got)
	}
	if len(got.Categories) != 2 || got.Categories[0].ID == nil || got.Categories[1].ID == nil || *got.Categories[0].ID != old.ID || !got.Categories[0].Archived || *got.Categories[1].ID != current.ID || got.Categories[1].Archived {
		t.Fatalf("category identities: %+v", got.Categories)
	}
}
