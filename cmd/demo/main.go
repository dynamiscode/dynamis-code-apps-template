package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type result struct {
	WorkspaceID string   `json:"workspaceId"`
	ItemIDs     []string `json:"itemIds"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	email := strings.TrimSpace(valueOrDefault("DEMO_OWNER_EMAIL", "demo@example.com"))
	password, ok := os.LookupEnv("DEMO_OWNER_PASSWORD")
	if !ok || password == "" {
		return errors.New("DEMO_OWNER_PASSWORD must be set")
	}
	workspaceName := strings.TrimSpace(valueOrDefault("DEMO_WORKSPACE", "Demo Workspace"))
	if workspaceName == "" {
		return errors.New("DEMO_WORKSPACE must not be empty")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		return err
	}
	auth, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		return err
	}
	workspaceID, userID, err := demoOwner(ctx, auth, email, password, workspaceName)
	if err != nil {
		return err
	}
	actor, err := auth.Authorize(ctx, userID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		return err
	}
	actor.AuthMethod = "demo"
	service := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	itemIDs := make([]string, 0, 3)
	for number, title := range []string{"Welcome to your demo workspace", "Import a CSV or JSON export", "Replace this sample item"} {
		created, err := service.Create(ctx, actor, workspaceID, title,
			fmt.Sprintf("demo-item-%d", number+1), identity.AuditContext{})
		if err != nil {
			return err
		}
		itemIDs = append(itemIDs, created.Item.ID)
	}
	return json.NewEncoder(os.Stdout).Encode(result{WorkspaceID: workspaceID, ItemIDs: itemIDs})
}

func demoOwner(
	ctx context.Context,
	auth *identity.Service,
	email, password, workspaceName string,
) (string, string, error) {
	bootstrapped, err := auth.IsBootstrapped(ctx)
	if err != nil {
		return "", "", err
	}
	if !bootstrapped {
		created, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
			Email: email, Password: password, WorkspaceName: workspaceName,
		}, identity.AuditContext{})
		if err != nil {
			return "", "", err
		}
		return created.WorkspaceID, created.UserID, nil
	}
	userID, err := auth.AuthenticateLocal(ctx, email, password)
	if err != nil {
		return "", "", err
	}
	workspaces, err := auth.ListWorkspaces(ctx, userID)
	if err != nil {
		return "", "", err
	}
	for _, workspace := range workspaces {
		if workspace.Name == workspaceName {
			return workspace.ID, userID, nil
		}
	}
	return "", "", fmt.Errorf("demo workspace %q was not found", workspaceName)
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
