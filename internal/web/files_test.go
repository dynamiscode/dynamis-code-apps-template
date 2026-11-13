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

type filesTestEnv struct {
	routes      http.Handler
	auth        *identity.Service
	workspaceID identity.WorkspaceID
	cookies     []*http.Cookie
}

// newFilesTestEnv wires the web handler with a local file store and the given
// per-object and per-workspace limits, and signs in the workspace owner.
func newFilesTestEnv(t *testing.T, maxObjectBytes, maxWorkspaceBytes int64) filesTestEnv {
	t.Helper()
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
		maxObjectBytes, maxWorkspaceBytes, cfg.Storage.SignedURLTTL, cfg.Storage.S3Prefix)
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
	return filesTestEnv{
		routes:      handler.Routes(),
		auth:        auth,
		workspaceID: owner.WorkspaceID,
		cookies:     []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}},
	}
}

func TestFileDownloadMissingReturnsNotFound(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	response := request(env.routes, http.MethodGet,
		"/workspaces/"+env.workspaceID.String()+"/files/"+strings.Repeat("0", 32)+"/content", nil, env.cookies, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing file response = %d, want %d", response.Code, http.StatusNotFound)
	}
	page := request(env.routes, http.MethodGet, "/workspaces/"+env.workspaceID.String()+"/files", nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `data-file-upload`) {
		t.Fatalf("local files page does not retain ordinary upload fallback: %d, %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `data-upload-status`) || !strings.Contains(page.Body.String(), `role="status"`) || !strings.Contains(page.Body.String(), `aria-live="polite"`) {
		t.Fatalf("files page missing upload status container: %s", page.Body.String())
	}
}

func TestFilesPageRendersOrdinaryMultipartUploadForm(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	page := request(env.routes, http.MethodGet, "/workspaces/"+env.workspaceID.String()+"/files", nil, env.cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `action="/workspaces/`+env.workspaceID.String()+`/files"`) ||
		!strings.Contains(body, `method="post"`) || !strings.Contains(body, `enctype="multipart/form-data"`) ||
		!strings.Contains(body, `name="csrf"`) || !strings.Contains(body, `type="file" name="file"`) {
		t.Fatalf("files page is not an ordinary multipart upload form: %d, %s", page.Code, body)
	}
	if strings.Contains(body, "data-presigned") || strings.Contains(body, "data-initiate") {
		t.Fatalf("files page still renders presigned upload attributes: %s", body)
	}
}
