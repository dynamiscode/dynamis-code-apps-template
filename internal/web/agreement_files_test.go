package web

import (
	"context"
	"strings"
	"testing"

	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestAgreementFileChoicesAuthorizationAndWorkspace(t *testing.T) {
	ctx := context.Background()
	env := newClientsTestEnv(t)
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Storage.LocalPath = t.TempDir()
	store, err := appfiles.NewStore(ctx, cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}
	service := appfiles.NewService(env.db, config.SQLite, env.auth, store, 1024, 4096, cfg.Storage.SignedURLTTL, "")
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	actor.AuthMethod = "session"
	ready, err := service.Upload(ctx, actor, env.workspaceID, "ready.txt", strings.NewReader("hello"), identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Initiate(ctx, actor, env.workspaceID, appfiles.InitiateInput{OriginalName: "pending.txt", Size: 5, ContentType: "text/plain"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	other, err := env.auth.CreateWorkspace(ctx, actor, identity.WorkspaceCreateInput{Name: "Other"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := env.auth.Authorize(ctx, env.ownerID, other, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Upload(ctx, otherActor, other, "other.txt", strings.NewReader("hello"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	h := &Handler{files: service, filesEnabled: true}
	page, available, err := h.agreementFileChoices(ctx, actor, env.workspaceID)
	if err != nil || !available || page.Total != 1 || len(page.Files) != 1 || page.Files[0].ID != ready.ID {
		t.Fatalf("choices: %+v available=%v err=%v", page, available, err)
	}
	noRead := actor
	noRead.Permissions = map[identity.Permission]bool{identity.ResourcesWrite: true}
	for _, tc := range []struct {
		name      string
		h         *Handler
		actor     identity.Principal
		workspace identity.WorkspaceID
	}{
		{"missing read", h, noRead, env.workspaceID},
		{"wrong workspace", h, actor, other},
		{"profile disabled", &Handler{files: service}, actor, env.workspaceID},
		{"service absent", &Handler{filesEnabled: true}, actor, env.workspaceID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, available, err := tc.h.agreementFileChoices(ctx, tc.actor, tc.workspace)
			if err != nil || available || len(page.Files) != 0 || page.Total != 0 {
				t.Fatalf("choices: %+v available=%v err=%v", page, available, err)
			}
		})
	}
}
