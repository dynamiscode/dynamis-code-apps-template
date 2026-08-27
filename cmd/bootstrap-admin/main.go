package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func main() {
	email := flag.String("email", "", "first owner email")
	workspace := flag.String("workspace", "", "first workspace name")
	workspaceLocale := flag.String("workspace-locale", "en", "first workspace locale (en or es)")
	flag.Parse()

	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if password == "" {
		fail("BOOTSTRAP_ADMIN_PASSWORD is required")
	}
	cfg, err := config.Load()
	if err != nil {
		fail(err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		fail(err.Error())
	}
	defer db.Close()
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		fail(err.Error())
	}
	service, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		fail(err.Error())
	}
	result, err := service.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: *email, Password: password, WorkspaceName: *workspace, WorkspaceLocale: *workspaceLocale,
	}, identity.AuditContext{})
	if err != nil {
		fail(err.Error())
	}
	fmt.Printf(
		"bootstrap complete: user=%s workspace=%s\n",
		result.UserID,
		result.WorkspaceID,
	)
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
