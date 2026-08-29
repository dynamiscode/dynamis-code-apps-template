package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/portability"
)

func TestFileDownloadMissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	cfg.Storage.LocalPath = t.TempDir()
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Files",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	store, err := appfiles.NewStore(ctx, cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}
	fileService := appfiles.NewService(db, cfg.Database.Driver, auth, store,
		cfg.Storage.MaxObjectBytes, cfg.Storage.MaxWorkspaceBytes, cfg.Storage.SignedURLTTL, cfg.Storage.S3Prefix)
	handler, err := NewHandlerWithServicesAndFiles(auth, itemService,
		portability.NewService(db, cfg.Database.Driver, auth, cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes, cfg.Data.ImportMaxRecords, cfg.Data.ImportMaxBytes, itemService),
		nil, cfg.HTTP, fileService, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	response := request(handler.Routes(), http.MethodGet,
		"/workspaces/"+owner.WorkspaceID+"/files/"+strings.Repeat("0", 32)+"/content", nil,
		[]*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing file response = %d, want %d", response.Code, http.StatusNotFound)
	}
	page := request(handler.Routes(), http.MethodGet, "/workspaces/"+owner.WorkspaceID+"/files", nil,
		[]*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `data-file-upload`) || strings.Contains(page.Body.String(), `data-presigned="true"`) {
		t.Fatalf("local files page does not retain ordinary upload fallback: %d, %s", page.Code, page.Body.String())
	}
}
