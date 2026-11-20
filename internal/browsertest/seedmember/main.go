package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func main() {
	if err := seedMember(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func seedMember() error {
	ownerEmail := os.Getenv("PLAYWRIGHT_EMAIL")
	memberEmail := os.Getenv("PLAYWRIGHT_MEMBER_EMAIL")
	memberPassword := os.Getenv("PLAYWRIGHT_MEMBER_PASSWORD")
	sqlitePath := os.Getenv("PLAYWRIGHT_SQLITE_PATH")
	if ownerEmail == "" || memberEmail == "" || memberPassword == "" || sqlitePath == "" {
		return errors.New("Playwright owner, member, and SQLite fixture settings are required")
	}

	ctx := context.Background()
	db, err := database.Open(ctx, config.Database{
		Driver:       config.SQLite,
		SQLitePath:   sqlitePath,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		return fmt.Errorf("open Playwright fixture database: %w", err)
	}
	defer db.Close()

	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		return fmt.Errorf("initialize Playwright identity fixture: %w", err)
	}
	var ownerID identity.UserID
	var workspaceID identity.WorkspaceID
	if err := db.QueryRowContext(ctx, `
		SELECT u.id, wm.workspace_id
		FROM users u
		JOIN workspace_members wm ON wm.user_id = u.id
		WHERE u.email = ? AND wm.role = ?
	`, ownerEmail, identity.Owner).Scan(&ownerID, &workspaceID); err != nil {
		return fmt.Errorf("find Playwright workspace owner: %w", err)
	}
	principal, err := auth.Authorize(ctx, ownerID, workspaceID, identity.InvitationsManage)
	if err != nil {
		return fmt.Errorf("authorize Playwright workspace invitation: %w", err)
	}
	invitation, err := auth.CreateInvitation(ctx, principal, memberEmail, identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		return fmt.Errorf("create Playwright member invitation: %w", err)
	}
	memberID, joinedWorkspaceID, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, memberPassword, identity.AuditContext{})
	if err != nil {
		return fmt.Errorf("accept Playwright member invitation: %w", err)
	}
	if joinedWorkspaceID != workspaceID {
		return errors.New("Playwright member joined the wrong workspace")
	}
	var role identity.Role
	if err := db.QueryRowContext(ctx,
		"SELECT role FROM workspace_members WHERE workspace_id = ? AND user_id = ?",
		workspaceID, memberID,
	).Scan(&role); err != nil {
		return fmt.Errorf("verify Playwright member role: %w", err)
	}
	if role != identity.Member {
		return errors.New("Playwright member fixture has the wrong role")
	}
	return nil
}
