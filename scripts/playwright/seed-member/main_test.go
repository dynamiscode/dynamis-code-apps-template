package main

import (
	"context"
	"path/filepath"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestSeedMemberCreatesInvitedLocalMember(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "playwright.db")
	db, err := database.Open(ctx, config.Database{
		Driver:       config.SQLite,
		SQLitePath:   databasePath,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db, config.SQLite); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	ownerEmail := "playwright@example.com"
	if _, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "playwright@example.com", Password: "playwright-password-123", WorkspaceName: "UI Baselines",
	}, identity.AuditContext{}); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	memberEmail := "playwright-member@example.com"
	memberPassword := "playwright-member-password-123"
	t.Setenv("PLAYWRIGHT_EMAIL", ownerEmail)
	t.Setenv("PLAYWRIGHT_MEMBER_EMAIL", memberEmail)
	t.Setenv("PLAYWRIGHT_MEMBER_PASSWORD", memberPassword)
	t.Setenv("PLAYWRIGHT_SQLITE_PATH", databasePath)
	if err := seedMember(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(ctx, config.Database{
		Driver:       config.SQLite,
		SQLitePath:   databasePath,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	auth, err = identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	memberID, err := auth.AuthenticateLocal(ctx, memberEmail, memberPassword)
	if err != nil {
		t.Fatal("seeded member cannot sign in")
	}
	var role identity.Role
	if err := db.QueryRowContext(ctx,
		"SELECT role FROM workspace_members WHERE user_id = ?",
		memberID,
	).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != identity.Member {
		t.Fatalf("seeded member role = %q, want %q", role, identity.Member)
	}
}
