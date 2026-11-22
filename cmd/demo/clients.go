package main

import (
	"context"
	"database/sql"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func seedClients(ctx context.Context, db *sql.DB, cfg config.Config, auth *identity.Service, workspaceID identity.WorkspaceID, userID identity.UserID) error {
	actor, err := auth.Authorize(ctx, userID, workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		return err
	}
	return seedLeadSource(ctx, clients.NewService(db, cfg.Database.Driver, auth, cfg.Data.ClientFieldsMaxPerWorkspace), actor, workspaceID)
}

func seedLeadSource(ctx context.Context, service *clients.Service, actor identity.Principal, workspaceID identity.WorkspaceID) error {
	fields, err := service.ListFields(ctx, actor, workspaceID, clients.ListFieldsInput{IncludeArchived: true})
	if err != nil {
		return err
	}
	for _, field := range fields {
		if field.Label == "Lead source" {
			return nil
		}
	}
	field, err := service.DefineField(ctx, actor, workspaceID, clients.FieldInput{Label: "Lead source", Type: clients.FieldTypeSingleChoice}, identity.AuditContext{})
	if err != nil {
		return err
	}
	for _, label := range []string{"Social media", "Referral", "Walk-in", "Phone", "Website"} {
		if _, err := service.AddOption(ctx, actor, workspaceID, field.ID, label, identity.AuditContext{}); err != nil {
			return err
		}
	}
	return nil
}
