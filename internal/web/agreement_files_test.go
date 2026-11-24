package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
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
	page, available, err := h.agreementFileChoices(ctx, actor, env.workspaceID, 1)
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
			page, available, err := tc.h.agreementFileChoices(ctx, tc.actor, tc.workspace, 1)
			if err != nil || available || len(page.Files) != 0 || page.Total != 0 {
				t.Fatalf("choices: %+v available=%v err=%v", page, available, err)
			}
		})
	}
}

func TestAgreementPickerPaginationAndSubmission(t *testing.T) {
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
	files := appfiles.NewService(env.db, config.SQLite, env.auth, store, 1024, 10000, cfg.Storage.SignedURLTTL, "")
	actor, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	actor.AuthMethod = "session"
	var last appfiles.File
	for i := 0; i < 101; i++ {
		last, err = files.Upload(ctx, actor, env.workspaceID, fmt.Sprintf("file-%03d.txt", i), strings.NewReader("x"), identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
	}
	h, err := NewHandlerWithServicesAndFiles(env.auth, nil, nil, cfg.HTTP, files, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	h.EnableClients(clients.NewService(env.db, config.SQLite, env.auth, 50), listprefs.NewService(env.db, config.SQLite, env.auth))
	h.EnableEngagement(engagement.NewService(env.db, config.SQLite, env.auth))
	h.EnableFilesPreferences(listprefs.NewService(env.db, config.SQLite, env.auth))
	routes := h.Routes()
	client, err := clients.NewService(env.db, config.SQLite, env.auth, 50).Create(ctx, actor, env.workspaceID, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	svc := engagement.NewService(env.db, config.SQLite, env.auth)
	stage, err := svc.CreateStage(ctx, actor, env.workspaceID, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	path := env.listPath() + "/" + client.ID.String()
	created := request(routes, http.MethodPost, path+"/engagements", url.Values{"csrf": {env.csrf}, "name": {"Work"}, "stage": {stage.ID.String()}}, env.cookies, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create: %d", created.Code)
	}
	items, err := svc.ListForClient(ctx, actor, env.workspaceID, client.ID)
	if err != nil || len(items) != 1 {
		t.Fatalf("items: %v %v", items, err)
	}
	detail := path + "/engagements/" + items[0].ID.String()
	first := request(routes, http.MethodGet, detail, nil, env.cookies, nil)
	for _, want := range []string{`for="engagement-file-choice"`, "file-000.txt", "page=2", `name="file_id"`} {
		if !strings.Contains(first.Body.String(), want) {
			t.Fatalf("first page missing %q", want)
		}
	}
	if strings.Contains(first.Body.String(), "file-100.txt") {
		t.Fatal("last file unexpectedly on first page")
	}
	second := request(routes, http.MethodGet, detail+"?page=2", nil, env.cookies, nil)
	if !strings.Contains(second.Body.String(), "file-100.txt") || !strings.Contains(second.Body.String(), "page=1") {
		t.Fatal("second page or previous link missing")
	}
	viewer := request(routes, http.MethodGet, detail, nil, env.viewerCookies, nil)
	if strings.Contains(viewer.Body.String(), "file-000.txt") {
		t.Fatal("viewer saw picker choices")
	}
	result := request(routes, http.MethodPost, detail+"/agreement", url.Values{"csrf": {env.csrf}, "return_to": {"detail"}, "selected_file_id": {last.ID.String()}, "file_id": {"invalid"}}, env.cookies, nil)
	if result.Code != http.StatusSeeOther {
		t.Fatalf("select: %d", result.Code)
	}
	item, err := svc.Get(ctx, actor, env.workspaceID, items[0].ID)
	if err != nil || item.Agreement.FileID != last.ID.String() {
		t.Fatalf("saved: %+v %v", item.Agreement, err)
	}
	bad := request(routes, http.MethodPost, detail+"/agreement", url.Values{"csrf": {env.csrf}, "return_to": {"detail"}, "selected_file_id": {"bad-id"}}, env.cookies, nil)
	if bad.Code != http.StatusUnprocessableEntity || !strings.Contains(bad.Body.String(), `value="bad-id"`) {
		t.Fatal("invalid selected ID not retained")
	}
	manual := request(routes, http.MethodPost, detail+"/agreement", url.Values{"csrf": {env.csrf}, "return_to": {"detail"}, "file_id": {""}}, env.cookies, nil)
	if manual.Code != http.StatusSeeOther {
		t.Fatalf("clear: %d", manual.Code)
	}
	item, err = svc.Get(ctx, actor, env.workspaceID, items[0].ID)
	if err != nil || item.Agreement.FileID != "" {
		t.Fatalf("clear: %+v %v", item.Agreement, err)
	}
}
