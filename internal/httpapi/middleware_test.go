package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBodyLimitFileOverrideIsRequestLocal(t *testing.T) {
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, err := io.Copy(io.Discard, request.Body)
		if err != nil {
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := bodyLimitMiddleware(next, 4, 8)

	fileRequest := httptest.NewRequest(http.MethodPut, "/api/v1/workspaces/"+strings.Repeat("a", 32)+"/files/content", strings.NewReader("12345678"))
	fileResponse := httptest.NewRecorder()
	handler.ServeHTTP(fileResponse, fileRequest)
	if fileResponse.Code != http.StatusNoContent {
		t.Fatalf("file response = %d, want %d", fileResponse.Code, http.StatusNoContent)
	}
	overLimitRequest := httptest.NewRequest(http.MethodPut, "/api/v1/workspaces/"+strings.Repeat("a", 32)+"/files/content", strings.NewReader("123456789"))
	overLimitResponse := httptest.NewRecorder()
	handler.ServeHTTP(overLimitResponse, overLimitRequest)
	if overLimitResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized file response = %d, want %d", overLimitResponse.Code, http.StatusRequestEntityTooLarge)
	}

	ordinaryRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("12345"))
	ordinaryResponse := httptest.NewRecorder()
	handler.ServeHTTP(ordinaryResponse, ordinaryRequest)
	if ordinaryResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("ordinary response = %d, want %d", ordinaryResponse.Code, http.StatusRequestEntityTooLarge)
	}

	jsonRequest := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+strings.Repeat("a", 32)+"/files", strings.NewReader("123456789"))
	jsonRequest.Header.Set("Content-Type", "application/json")
	jsonResponse := httptest.NewRecorder()
	handler.ServeHTTP(jsonResponse, jsonRequest)
	if jsonResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("file initiation response = %d, want %d", jsonResponse.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestBodyLimitAllowsMultipartEnvelopeAroundFileLimit(t *testing.T) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "small.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(strings.Repeat("x", 8))); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, err := io.Copy(io.Discard, request.Body); err != nil {
			t.Errorf("read request body: %v", err)
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := bodyLimitMiddleware(next, 8, 8)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+strings.Repeat("a", 32)+"/files", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("multipart response = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestFileStreamingPathGetsRequestDeadline(t *testing.T) {
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := request.Context().Deadline(); !ok {
			t.Error("file stream request has no context deadline")
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	handler := timeoutMiddleware(next, time.Minute, slog.Default())
	request := httptest.NewRequest(http.MethodPut, "/api/v1/workspaces/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/files/file/content", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("response = %d, want %d", response.Code, http.StatusNoContent)
	}
}
