package web

import (
	"context"
	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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
	store, err := appfiles.NewStore(ctx, cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}
	fileService := appfiles.NewService(db, cfg.Database.Driver, auth, store,
		maxObjectBytes, maxWorkspaceBytes, cfg.Storage.SignedURLTTL, cfg.Storage.S3Prefix)
	handler, err := NewHandlerWithServicesAndFiles(auth, nil, nil, cfg.HTTP, fileService, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler.EnableFilesPreferences(listprefs.NewService(db, cfg.Database.Driver, auth))
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
	page := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `data-file-upload`) {
		t.Fatalf("local files page does not retain ordinary upload fallback: %d, %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `data-upload-status`) || !strings.Contains(page.Body.String(), `role="status"`) || !strings.Contains(page.Body.String(), `aria-live="polite"`) {
		t.Fatalf("files page missing upload status container: %s", page.Body.String())
	}
}

func TestFilesPageRendersOrdinaryMultipartUploadForm(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	page := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, env.cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `action="`+env.filesPath()+`"`) ||
		!strings.Contains(body, `method="post"`) || !strings.Contains(body, `enctype="multipart/form-data"`) ||
		!strings.Contains(body, `name="csrf"`) || !strings.Contains(body, `type="file"`) || !strings.Contains(body, `name="file"`) {
		t.Fatalf("files page is not an ordinary multipart upload form: %d, %s", page.Code, body)
	}
	if strings.Contains(body, "data-presigned") || strings.Contains(body, "data-initiate") {
		t.Fatalf("files page still renders presigned upload attributes: %s", body)
	}
}

func TestFilesPageHidesUploadForReadOnlyViewer(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	empty := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, env.viewerCookies, nil)
	if empty.Code != http.StatusOK || strings.Contains(empty.Body.String(), `href="#file-upload-heading"`) {
		t.Fatalf("empty viewer Files page exposes upload link: %d %s", empty.Code, empty.Body.String())
	}
	upload := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.csrf}}, "read-only.txt", "hello", env.cookies, nil)
	if upload.Code != http.StatusSeeOther {
		t.Fatalf("owner upload = %d: %s", upload.Code, upload.Body.String())
	}
	page := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, env.viewerCookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "read-only.txt") || !strings.Contains(body, "/content") {
		t.Fatalf("viewer Files list = %d: %s", page.Code, body)
	}
	if strings.Contains(body, "data-file-upload") || strings.Contains(body, `type="file"`) {
		t.Fatalf("read-only viewer sees file upload controls: %s", body)
	}
}

func TestFilesPageRendersTemplShellAndAccessibleEmptyState(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	page := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, env.cookies, nil)
	body := page.Body.String()
	for _, want := range []string{
		`<html lang="en">`,
		`href="/assets/components.css"`,
		`href="/workspaces/` + env.workspaceID.String() + `">Home</a>`,
		`aria-current="page" href="` + env.filesPath() + `">Files</a>`,
		`Upload your first file`,
		`id="file-empty-state"`,
		`id="file-empty-state"`,
		`Upload your first file`,
		`class="list-toolbar"`,
		`src="/assets/alpine-components.js"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Files page missing %q: %s", want, body)
		}
	}
	if page.Header().Get("Cache-Control") != "no-store" || page.Header().Get("Content-Language") != "en" {
		t.Errorf("Files response headers = cache %q, language %q", page.Header().Get("Cache-Control"), page.Header().Get("Content-Language"))
	}
	if strings.Contains(body, "Back to home") || hxPostOutsideColumns(body) {
		t.Errorf("Files page has obsolete back-home navigation or an unapproved HTMX enhancement: %s", body)
	}
	for _, secretMarker := range []string{"X-Amz-Signature", "X-Amz-Credential", "Signature="} {
		if strings.Contains(body, secretMarker) {
			t.Errorf("Files page leaked signed URL marker %q", secretMarker)
		}
	}
}

func TestFilesUploadFeedbackUsesSelectedLocale(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	cookies := append(append([]*http.Cookie(nil), env.cookies...), &http.Cookie{Name: "locale", Value: "es"})
	page := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, cookies, nil)
	for _, want := range []string{`data-status-uploading="Subiendo archivo…"`, `data-status-success="Carga completada."`, `data-status-choose-file="Elige un archivo para cargar."`, `data-upload-progress`, `aria-label="Progreso de carga"`, `Sube tu primer archivo`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("Spanish files page missing %q: %s", want, page.Body.String())
		}
	}

	upload := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.csrf}}, "notes.exe", "hello", cookies, nil)
	if upload.Code != http.StatusUnprocessableEntity || !strings.Contains(upload.Body.String(), "El archivo seleccionado no es válido.") {
		t.Fatalf("Spanish invalid-file error = %d: %s", upload.Code, upload.Body.String())
	}
}

func TestFilesListUsesSharedPresentationAndSavedColumns(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	path := env.filesPath()
	for _, name := range []string{"alpha.txt", "bravo.txt"} {
		if response := multipartRequest(t, env.routes, path, url.Values{"csrf": {env.csrf}}, name, "hello", env.cookies, nil); response.Code != http.StatusSeeOther {
			t.Fatalf("upload %s = %d: %s", name, response.Code, response.Body.String())
		}
	}
	body := request(env.routes, http.MethodGet, path+"?all=1&limit=1", nil, env.cookies, nil).Body.String()
	for _, required := range []string{`class="list-view-table"`, `class="list-view-cards"`, `data-column="original_name"`, `preferences/columns/size`, `class="client-field-choices"`, `page=2`, `/content`} {
		if !strings.Contains(body, required) {
			t.Errorf("Files list missing %q: %s", required, body)
		}
	}
	cursorAt := strings.Index(body, "page=2")
	if cursorAt < 0 {
		t.Fatalf("Files page has no link to page 2: %s", body)
	}
	linkStart := strings.LastIndex(body[:cursorAt], `href="`)
	linkEnd := strings.Index(body[cursorAt:], `"`)
	if linkStart < 0 || linkEnd < 0 {
		t.Fatalf("Files page has no usable next-page link: %s", body)
	}
	nextPage := request(env.routes, http.MethodGet, html.UnescapeString(body[linkStart+6:cursorAt+linkEnd]), nil, env.cookies, nil)
	nextBody := nextPage.Body.String()
	wantNext, wantFirst := "alpha.txt", "bravo.txt"
	if strings.Contains(body, wantNext) {
		wantNext, wantFirst = wantFirst, wantNext
	}
	if nextPage.Code != http.StatusOK || !strings.Contains(nextBody, `class="list-view-table"`) || !strings.Contains(body, wantFirst) || !strings.Contains(nextBody, wantNext) || strings.Contains(nextBody, wantFirst) {
		t.Fatalf("next Files page = %d: %s", nextPage.Code, nextPage.Body.String())
	}
	preference := request(env.routes, http.MethodPost, path+"/preferences/columns/size?search=alpha&sort=size&limit=1", url.Values{"csrf": {env.csrf}, "visible": {"false"}}, env.cookies, nil)
	if preference.Code != http.StatusSeeOther || !strings.Contains(preference.Header().Get("Location"), "search=alpha") || !strings.Contains(preference.Header().Get("Location"), "sort=size") {
		t.Fatalf("hide file size = %d: %s", preference.Code, preference.Body.String())
	}
	hidden := request(env.routes, http.MethodGet, path+"?all=1&search=alpha&sort=size&limit=1", nil, env.cookies, nil)
	if hidden.Code != http.StatusOK || strings.Contains(hidden.Body.String(), `data-column="size"`) || !strings.Contains(hidden.Body.String(), `<mark>alpha</mark>.txt`) || strings.Contains(hidden.Body.String(), "bravo.txt") {
		t.Fatalf("filtered Files list with saved column = %d: %s", hidden.Code, hidden.Body.String())
	}
	invalid := request(env.routes, http.MethodGet, path+"?min_size=large", nil, env.cookies, nil)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid typed file filter = %d, want 400", invalid.Code)
	}
}

func TestFilesMultipartUploadStoresFileAndRedirects(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	upload := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.csrf}}, "notes.txt", "hello", env.cookies, nil)
	if upload.Code != http.StatusSeeOther || upload.Header().Get("Location") != env.filesPath() {
		t.Fatalf("multipart upload = %d location %q, want %d to files list", upload.Code, upload.Header().Get("Location"), http.StatusSeeOther)
	}
	page := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, env.cookies, nil)
	if !strings.Contains(page.Body.String(), "notes.txt") {
		t.Fatalf("uploaded file missing from list: %s", page.Body.String())
	}
}

func TestFilesDownloadRequiresWorkspaceSessionAndServesMemberFile(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	if response := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.csrf}}, "notes.txt", "private contents", env.cookies, nil); response.Code != http.StatusSeeOther {
		t.Fatalf("upload = %d: %s", response.Code, response.Body.String())
	}
	page := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, env.cookies, nil)
	body := page.Body.String()
	contentAt := strings.Index(body, "/content")
	if page.Code != http.StatusOK || contentAt < 0 {
		t.Fatalf("files page has no download link: %d, %s", page.Code, body)
	}
	linkStart := strings.LastIndex(body[:contentAt], `href="`)
	if linkStart < 0 {
		t.Fatalf("files page has no download link: %d, %s", page.Code, body)
	}
	downloadPath := html.UnescapeString(body[linkStart+6 : contentAt+len("/content")])
	unauthenticated := request(env.routes, http.MethodGet, downloadPath, nil, nil, nil)
	if unauthenticated.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated download = %d, want sign-in redirect", unauthenticated.Code)
	}
	downloaded := request(env.routes, http.MethodGet, downloadPath, nil, env.viewerCookies, nil)
	if downloaded.Code != http.StatusOK || downloaded.Body.String() != "private contents" || !strings.Contains(downloaded.Header().Get("Content-Disposition"), "notes.txt") {
		t.Fatalf("workspace member download = %d, disposition %q: %s", downloaded.Code, downloaded.Header().Get("Content-Disposition"), downloaded.Body.String())
	}
}

func TestFilesMultipartUploadDeniesViewer(t *testing.T) {
	env := newFilesTestEnv(t, 16<<20, 1<<30)
	upload := multipartRequest(t, env.routes, env.filesPath(), url.Values{"csrf": {env.viewerCSRF}}, "notes.txt", "hello", env.viewerCookies, nil)
	if upload.Code != http.StatusForbidden {
		t.Fatalf("viewer upload = %d, want %d: %s", upload.Code, http.StatusForbidden, upload.Body.String())
	}
	page := request(env.routes, http.MethodGet, env.filesPath()+"?all=1", nil, env.cookies, nil)
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
