package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	routes        http.Handler
	workspaceID   identity.WorkspaceID
	cookies       []*http.Cookie
	csrf          string
	viewerCookies []*http.Cookie
	viewerCSRF    string
}

// newFilesTestEnv wires the web handler with a local file store and the given
// per-object and per-workspace limits. It signs in the workspace owner and a
// viewer invited to the same workspace.
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
	invitations, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, invitations, "viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "viewer-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerSession, err := auth.CreateSession(ctx, viewerID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return filesTestEnv{
		routes:        handler.Routes(),
		workspaceID:   owner.WorkspaceID,
		cookies:       []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}},
		csrf:          session.CSRFSecret,
		viewerCookies: []*http.Cookie{{Name: "session", Value: viewerSession.Secret}, {Name: "csrf", Value: viewerSession.CSRFSecret}},
		viewerCSRF:    viewerSession.CSRFSecret,
	}
}

func (env filesTestEnv) filesPath() string {
	return "/workspaces/" + env.workspaceID.String() + "/files"
}

func TestFileDownloadMissingReturnsNotFound(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	response := request(env.routes, http.MethodGet, env.filesPath()+"/"+strings.Repeat("0", 32)+"/content", nil, env.cookies, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing file response = %d, want %d", response.Code, http.StatusNotFound)
	}
	page := request(env.routes, http.MethodGet, env.filesPath(), nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `data-file-upload`) {
		t.Fatalf("local files page does not retain ordinary upload fallback: %d, %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `data-upload-status`) || !strings.Contains(page.Body.String(), `role="status"`) || !strings.Contains(page.Body.String(), `aria-live="polite"`) {
		t.Fatalf("files page missing upload status container: %s", page.Body.String())
	}
}

func TestFilesPageRendersOrdinaryMultipartUploadForm(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	page := request(env.routes, http.MethodGet, env.filesPath(), nil, env.cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `action="`+env.filesPath()+`"`) ||
		!strings.Contains(body, `method="post"`) || !strings.Contains(body, `enctype="multipart/form-data"`) ||
		!strings.Contains(body, `name="csrf"`) || !strings.Contains(body, `type="file" name="file"`) {
		t.Fatalf("files page is not an ordinary multipart upload form: %d, %s", page.Code, body)
	}
	if strings.Contains(body, "data-presigned") || strings.Contains(body, "data-initiate") {
		t.Fatalf("files page still renders presigned upload attributes: %s", body)
	}
}

func TestFilesMultipartUploadStoresFileAndRedirects(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	upload := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.csrf}}, "notes.txt", "hello", env.cookies, nil)
	if upload.Code != http.StatusSeeOther || upload.Header().Get("Location") != env.filesPath() {
		t.Fatalf("multipart upload = %d location %q, want %d to files list", upload.Code, upload.Header().Get("Location"), http.StatusSeeOther)
	}
	page := request(env.routes, http.MethodGet, env.filesPath(), nil, env.cookies, nil)
	if !strings.Contains(page.Body.String(), "notes.txt") {
		t.Fatalf("uploaded file missing from list: %s", page.Body.String())
	}
}

func TestFilesMultipartUploadDeniesViewer(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	upload := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.viewerCSRF}}, "notes.txt", "hello", env.viewerCookies, nil)
	if upload.Code != http.StatusForbidden {
		t.Fatalf("viewer upload = %d, want %d: %s", upload.Code, http.StatusForbidden, upload.Body.String())
	}
	page := request(env.routes, http.MethodGet, env.filesPath(), nil, env.cookies, nil)
	if strings.Contains(page.Body.String(), "notes.txt") {
		t.Fatalf("denied viewer upload was stored: %s", page.Body.String())
	}
}

func TestFilesUploadStatusMessages(t *testing.T) {
	cases := []struct {
		name         string
		maxObject    int64
		maxWorkspace int64
		uploads      []struct{ filename, content string }
		want         string
	}{
		{
			name: "oversize", maxObject: 8, maxWorkspace: 1 << 30,
			uploads: []struct{ filename, content string }{{"notes.txt", "0123456789abcdef"}},
			want:    "The file exceeds the per-file size limit.",
		},
		{
			name: "storage limit", maxObject: 1 << 20, maxWorkspace: 10,
			uploads: []struct{ filename, content string }{{"a.txt", "12345678"}, {"b.txt", "12345678"}},
			want:    "The workspace file storage limit was reached.",
		},
		{
			name: "invalid file", maxObject: 1 << 20, maxWorkspace: 1 << 30,
			uploads: []struct{ filename, content string }{{"notes.exe", "hello"}},
			want:    "The selected file is invalid.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newFilesTestEnv(t, tc.maxObject, tc.maxWorkspace)
			var last *httptest.ResponseRecorder
			for _, upload := range tc.uploads {
				last = multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.csrf}}, upload.filename, upload.content, env.cookies, nil)
			}
			if last.Code != http.StatusUnprocessableEntity || !strings.Contains(last.Body.String(), `role="alert"`) || !strings.Contains(last.Body.String(), tc.want) {
				t.Fatalf("upload status = %d, want %d with %q: %s", last.Code, http.StatusUnprocessableEntity, tc.want, last.Body.String())
			}
		})
	}
}

func TestFilesInitiateAndCompleteRoutesReturnNotFound(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	form := url.Values{"csrf": {env.csrf}, "originalName": {"notes.txt"}, "size": {"5"}, "contentType": {"text/plain"}}
	initiate := request(env.routes, http.MethodPost, env.filesPath()+"/initiate", form, env.cookies, nil)
	if initiate.Code != http.StatusNotFound {
		t.Fatalf("initiate route = %d, want %d", initiate.Code, http.StatusNotFound)
	}
	complete := request(env.routes, http.MethodPost, env.filesPath()+"/"+strings.Repeat("0", 32)+"/complete", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if complete.Code != http.StatusNotFound {
		t.Fatalf("complete route = %d, want %d", complete.Code, http.StatusNotFound)
	}
}
