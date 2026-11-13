package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/files/filestest"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

func TestFileRESTLifecycle(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello files")); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/files", &body)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload = %d, %s", response.Code, response.Body.String())
	}
	var uploaded fileResponse
	if err := json.Unmarshal(response.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}
	if uploaded.OriginalName != "notes.txt" || uploaded.Status != appfiles.Ready || uploaded.Size != int64(len("hello files")) {
		t.Fatalf("uploaded = %+v", uploaded)
	}
	content := serveAuthorized(handler, http.MethodGet, uploaded.DownloadURL, "", token, nil)
	if content.Code != http.StatusOK || content.Body.String() != "hello files" {
		t.Fatalf("download = %d, %q", content.Code, content.Body.String())
	}
	if content.Header().Get("Content-Disposition") == "" {
		t.Fatal("download filename missing")
	}
	wrongWorkspace := serveAuthorized(handler, http.MethodGet,
		"/api/v1/workspaces/00000000000000000000000000000000/files/"+uploaded.ID.String(), "", token, nil)
	assertProblem(t, wrongWorkspace, http.StatusNotFound, "not-found")
	list := serveAuthorized(handler, http.MethodGet, "/api/v1/workspaces/"+workspaceID+"/files", "", token, nil)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(uploaded.ID.String())) {
		t.Fatalf("list = %d, %s", list.Code, list.Body.String())
	}
}

func TestLocalInitiatedUploadOmitsCompletionURL(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	response := serveAuthorized(handler, http.MethodPost,
		"/api/v1/workspaces/"+workspaceID+"/files/uploads",
		`{"originalName":"notes.txt","size":5,"contentType":"text/plain"}`,
		token, map[string]string{"Content-Type": "application/json"})
	if response.Code != http.StatusCreated {
		t.Fatalf("initiate = %d, %s", response.Code, response.Body.String())
	}
	var initiated fileResponse
	if err := json.Unmarshal(response.Body.Bytes(), &initiated); err != nil {
		t.Fatal(err)
	}
	if initiated.CompleteURL != "" {
		t.Fatalf("local completion URL = %q, want empty", initiated.CompleteURL)
	}
	uploaded := serveAuthorized(handler, http.MethodPut, initiated.UploadURL, "hello", token,
		map[string]string{"Content-Type": "application/octet-stream"})
	if uploaded.Code != http.StatusOK {
		t.Fatalf("local upload = %d, %s", uploaded.Code, uploaded.Body.String())
	}
}

// On S3 storage the initiate response points at the application's content
// route with no upload headers. The bytes stream through the application, the
// file stays pending until complete verifies it, and downloads still redirect
// to a presigned URL.
func TestS3InitiatedUploadStreamsThroughApplication(t *testing.T) {
	handler, db, workspaceID, token := testHandlerWithStorage(t, func(storage *config.Storage) {
		storage.Driver = config.StorageS3
		storage.S3Endpoint = filestest.S3Endpoint(t)
		storage.S3Region = "us-east-1"
		storage.S3Bucket = "files"
		storage.S3ForcePathStyle = true
	})
	base := "/api/v1/workspaces/" + workspaceID + "/files"
	initiateBody := `{"originalName":"notes.txt","size":5,"contentType":"text/plain"}`
	jsonHeaders := map[string]string{"Content-Type": "application/json"}
	octetHeaders := map[string]string{"Content-Type": "application/octet-stream"}

	initiate := serveAuthorized(handler, http.MethodPost, base+"/uploads", initiateBody, token, jsonHeaders)
	if initiate.Code != http.StatusCreated {
		t.Fatalf("initiate = %d, %s", initiate.Code, initiate.Body.String())
	}
	var initiated fileResponse
	if err := json.Unmarshal(initiate.Body.Bytes(), &initiated); err != nil {
		t.Fatal(err)
	}
	fileBase := base + "/" + initiated.ID.String()
	if initiated.UploadURL != fileBase+"/content" || initiated.CompleteURL != fileBase+"/complete" {
		t.Fatalf("upload URLs = %q, %q; want application routes", initiated.UploadURL, initiated.CompleteURL)
	}
	if len(initiated.UploadHeaders) != 0 {
		t.Fatalf("upload headers = %v, want none", initiated.UploadHeaders)
	}

	anonymous := httptest.NewRecorder()
	handler.ServeHTTP(anonymous, httptest.NewRequest(http.MethodPut, initiated.UploadURL, strings.NewReader("hello")))
	if anonymous.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous content write = %d, want 401", anonymous.Code)
	}

	written := serveAuthorized(handler, http.MethodPut, initiated.UploadURL, "hello", token, octetHeaders)
	if written.Code != http.StatusOK {
		t.Fatalf("content write = %d, %s", written.Code, written.Body.String())
	}
	var stored fileResponse
	if err := json.Unmarshal(written.Body.Bytes(), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status != appfiles.Pending {
		t.Fatalf("status after content write = %s, want pending", stored.Status)
	}

	completed := serveAuthorized(handler, http.MethodPost, initiated.CompleteURL, "", token, nil)
	if completed.Code != http.StatusOK {
		t.Fatalf("complete = %d, %s", completed.Code, completed.Body.String())
	}
	var ready fileResponse
	if err := json.Unmarshal(completed.Body.Bytes(), &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Status != appfiles.Ready || ready.Size != 5 {
		t.Fatalf("completed file = %+v", ready)
	}

	download := serveAuthorized(handler, http.MethodGet, fileBase+"/content", "", token, nil)
	if download.Code != http.StatusFound || !strings.Contains(download.Header().Get("Location"), "X-Amz-Signature") {
		t.Fatalf("download = %d, location %q; want presigned redirect", download.Code, download.Header().Get("Location"))
	}

	second := serveAuthorized(handler, http.MethodPost, base+"/uploads", initiateBody, token, jsonHeaders)
	if second.Code != http.StatusCreated {
		t.Fatalf("second initiate = %d, %s", second.Code, second.Body.String())
	}
	var pending fileResponse
	if err := json.Unmarshal(second.Body.Bytes(), &pending); err != nil {
		t.Fatal(err)
	}
	denied := serveAuthorized(handler, http.MethodPut, pending.UploadURL, "hello", viewerAPIToken(t, db, workspaceID), octetHeaders)
	assertProblem(t, denied, http.StatusForbidden, "insufficient-scope")
}

// viewerAPIToken returns a read-only bearer token for a new viewer member of the
// workspace. A viewer cannot grant write scope, so the token carries only read.
func viewerAPIToken(t *testing.T, db *sql.DB, workspaceID string) string {
	t.Helper()
	ctx := context.Background()
	userID, err := id.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec("INSERT INTO users (id, email, password_hash, created_at) VALUES (?, ?, NULL, ?)", userID, "viewer@example.com", now); err != nil {
		t.Fatal(err)
	}
	workspaceKey, err := identity.ParseWorkspaceID(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)", workspaceKey, userID, "viewer", now); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(ctx, userID, workspaceKey, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.CreateAPIToken(ctx, principal, "viewer", []identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return token.Secret
}

func TestFileProblemMapsBodyLimit(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/files", nil)
	request = request.WithContext(withRequestID(request.Context(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	response := httptest.NewRecorder()
	(&handler{}).fileProblem(response, request, &http.MaxBytesError{Limit: 1})
	assertProblem(t, response, http.StatusRequestEntityTooLarge, "body-too-large")
}

// File paths and bodies carry only the file short ID. Integer keys, malformed
// and unknown IDs, and files reached through another workspace must return the
// same not-found response.
func TestFileRoutesAreShortIDOnlyAndUniform(t *testing.T) {
	handler, db, workspaceID, token := testHandler(t)
	response := serveAuthorized(handler, http.MethodPost,
		"/api/v1/workspaces/"+workspaceID+"/files/uploads",
		`{"originalName":"notes.txt","size":5,"contentType":"text/plain"}`,
		token, map[string]string{"Content-Type": "application/json"})
	if response.Code != http.StatusCreated {
		t.Fatalf("initiate = %d, %s", response.Code, response.Body.String())
	}
	var initiated fileResponse
	if err := json.Unmarshal(response.Body.Bytes(), &initiated); err != nil {
		t.Fatal(err)
	}
	if initiated.ID == 0 || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"`+initiated.ID.String()+`"`)) {
		t.Fatalf("initiate body = %s, want the file short ID", response.Body.String())
	}
	fileKey := int64(initiated.ID)

	var otherKey, foreignFileKey int64
	if err := db.QueryRow(
		"INSERT INTO workspaces (name, created_at) VALUES (?, ?) RETURNING id",
		"Foreign", "2026-11-11T00:00:00Z",
	).Scan(&otherKey); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`
		INSERT INTO files (workspace_id, object_key, original_name, size, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		otherKey, "foreign/object", "foreign.txt", 5, "ready", "2026-11-11T00:00:00Z", "2026-11-11T00:00:00Z",
	).Scan(&foreignFileKey); err != nil {
		t.Fatal(err)
	}

	base := "/api/v1/workspaces/" + workspaceID + "/files/"
	probes := map[string]string{
		"integer key":               "/api/v1/workspaces/" + workspaceID + "/files/" + strconv.FormatInt(fileKey, 10),
		"malformed":                 base + "not-a-short-id!",
		"hex legacy form":           base + "00000000000000000000000000000000",
		"unknown short ID":          base + appfiles.FileID(fileKey+1000).String(),
		"file in another workspace": base + appfiles.FileID(foreignFileKey).String(),
	}
	var reference map[string]any
	for name, path := range probes {
		probe := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
		assertProblem(t, probe, http.StatusNotFound, "not-found")
		var body map[string]any
		if err := json.Unmarshal(probe.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		delete(body, "instance")
		delete(body, "requestId")
		if reference == nil {
			reference = body
			continue
		}
		if !reflect.DeepEqual(reference, body) {
			t.Fatalf("%s response = %v, want the same body as %v", name, body, reference)
		}
	}
}
