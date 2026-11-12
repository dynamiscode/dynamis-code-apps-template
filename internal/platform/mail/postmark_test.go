package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostmarkSendPostsMessageWithServerToken(t *testing.T) {
	var gotToken, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken, gotPath = r.Header.Get("X-Postmark-Server-Token"), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"ErrorCode":0,"Message":"OK"}`)
	}))
	defer server.Close()

	sender, err := NewPostmark(server.URL, "pm_server_fake", "mailer@example.com", "Dynamis")
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(context.Background(), "invitee@example.com", "Hi", "Body"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotToken != "pm_server_fake" || gotPath != "/email" {
		t.Fatalf("token=%q path=%q", gotToken, gotPath)
	}
	if gotBody["From"] != `"Dynamis" <mailer@example.com>` || gotBody["To"] != "invitee@example.com" ||
		gotBody["Subject"] != "Hi" || gotBody["TextBody"] != "Body" {
		t.Fatalf("body = %v", gotBody)
	}
}

func TestPostmarkSendReportsRejectionWithoutSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"ErrorCode":300,"Message":"pm_server_fake echoed"}`)
	}))
	defer server.Close()

	sender, _ := NewPostmark(server.URL, "pm_server_fake", "mailer@example.com", "")
	err := sender.Send(context.Background(), "invitee@example.com", "Hi", "Body")
	if err == nil || strings.Contains(err.Error(), "pm_server_fake") {
		t.Fatalf("Send() error = %v, want rejection without secret", err)
	}
}

func TestPostmarkSendRejectsHeaderInjection(t *testing.T) {
	sender, _ := NewPostmark("http://unused.invalid", "pm_server_fake", "mailer@example.com", "")
	if err := sender.Send(context.Background(), "a@example.com\r\nBcc: x@example.com", "Hi", "Body"); err == nil {
		t.Fatal("Send() accepted a header injection")
	}
}
