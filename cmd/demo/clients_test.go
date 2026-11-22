package main

import (
	"context"
	"reflect"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

func TestSeedLeadSource(t *testing.T) {
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
	workspaceID, userID, err := demoOwner(ctx, auth, "owner@example.com", "long-enough-password", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	svc := clients.NewService(db, config.SQLite, auth, 50)
	actor, err := auth.Authorize(ctx, userID, workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	field, err := svc.DefineField(ctx, actor, workspaceID, clients.FieldInput{Label: "Lead source", Type: clients.FieldTypeSingleChoice}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddOption(ctx, actor, workspaceID, field.ID, "Social media", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Database: config.Database{Driver: config.SQLite}}
	cfg.Data.ClientFieldsMaxPerWorkspace = 50
	for i := 0; i < 2; i++ {
		if err := seedClients(ctx, db, cfg, auth, workspaceID, userID); err != nil {
			t.Fatal(err)
		}
	}
	fields, err := svc.ListFields(ctx, actor, workspaceID, clients.ListFieldsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 1 || fields[0].Label != "Lead source" || fields[0].Type != clients.FieldTypeSingleChoice {
		t.Fatalf("fields = %+v", fields)
	}
	options, err := svc.ListOptions(ctx, actor, workspaceID, fields[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, option := range options {
		labels = append(labels, option.Label)
	}
	if want := []string{"Social media", "Referral", "Walk-in", "Phone", "Website"}; !reflect.DeepEqual(labels, want) {
		t.Fatalf("options = %v, want %v", labels, want)
	}
}
