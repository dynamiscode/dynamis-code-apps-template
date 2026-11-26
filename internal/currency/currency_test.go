package currency

import (
	"context"
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestCurrencyConfiguration(t *testing.T) {
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
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(db, auth)
	initial, err := s.ListCurrencies(ctx, actor, owner.WorkspaceID)
	if err != nil || len(initial) != 1 || initial[0] != (Currency{Code: "USD", Decimals: 2, Default: true}) {
		t.Fatalf("initial=%v err=%v", initial, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_currencies WHERE workspace_id = $1`, owner.WorkspaceID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("new workspace rows=%d err=%v", count, err)
	}
	for _, invalid := range [][]Currency{
		{},
		{{Code: "ZZZ", Decimals: 2, Default: true}},
		{{Code: "EUR", Decimals: 18, Default: true}},
		{{Code: "USD", Decimals: 2}, {Code: "EUR", Decimals: 2}},
		{{Code: "USD", Decimals: 2, Default: true}, {Code: "USD", Decimals: 2}},
	} {
		if err := s.SetCurrencies(ctx, actor, owner.WorkspaceID, invalid); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid %v: %v", invalid, err)
		}
	}
	afterInvalid, err := s.ListCurrencies(ctx, actor, owner.WorkspaceID)
	if err != nil || len(afterInvalid) != 1 || afterInvalid[0] != initial[0] {
		t.Fatalf("invalid saves changed default: %v, %v", afterInvalid, err)
	}
	want := []Currency{{Code: "EUR", Decimals: 2, Default: true}, {Code: "USD", Decimals: 2}}
	if err := s.SetCurrencies(ctx, actor, owner.WorkspaceID, want); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_currencies WHERE workspace_id = $1`, owner.WorkspaceID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("saved rows=%d err=%v", count, err)
	}
	got, err := s.ListCurrencies(ctx, actor, owner.WorkspaceID)
	if err != nil || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("saved=%v err=%v", got, err)
	}
	if err := s.SetCurrencies(ctx, actor, owner.WorkspaceID, []Currency{{Code: "EUR", Decimals: 3, Default: true}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("changed precision: %v", err)
	}
	afterInvalid, err = s.ListCurrencies(ctx, actor, owner.WorkspaceID)
	if err != nil || len(afterInvalid) != 2 || afterInvalid[0] != want[0] || afterInvalid[1] != want[1] {
		t.Fatalf("invalid save changed persisted settings: %v, %v", afterInvalid, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO workspace_currencies (workspace_id, code, decimals, is_default) VALUES ($1, 'OLD', 7, FALSE)`, owner.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	legacy := append(want, Currency{Code: "OLD", Decimals: 7})
	if err := s.SetCurrencies(ctx, actor, owner.WorkspaceID, legacy); err != nil {
		t.Fatalf("preserving legacy: %v", err)
	}
	legacy[0].Default = false
	legacy[2].Default = true
	if err := s.SetCurrencies(ctx, actor, owner.WorkspaceID, legacy); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("new legacy default: %v", err)
	}
}

func TestAvailableCurrencies(t *testing.T) {
	found := make(map[string]int)
	for _, c := range Available() {
		if _, duplicate := found[c.Code]; duplicate {
			t.Fatalf("duplicate %s", c.Code)
		}
		found[c.Code] = c.Decimals
	}
	for code, decimals := range map[string]int{"USD": 2, "JPY": 0, "KWD": 3} {
		if got, ok := found[code]; !ok || got != decimals {
			t.Fatalf("%s decimals=%d present=%t", code, got, ok)
		}
	}
	if _, ok := found["ZZZ"]; ok {
		t.Fatal("arbitrary currency accepted")
	}
	if _, ok := Canonical("BTC"); ok {
		t.Fatal("non-ISO currency accepted")
	}
	if _, ok := Canonical("D A"); ok {
		t.Fatal("partial code accepted")
	}
}
