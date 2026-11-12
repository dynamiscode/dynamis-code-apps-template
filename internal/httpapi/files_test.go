package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"

	appfiles "example.com/dynamis-code/apps-template/internal/files"
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
