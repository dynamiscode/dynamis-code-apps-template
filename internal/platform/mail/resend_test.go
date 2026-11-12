package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendSendPostsToHTTPSAPIWithBearerKey(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/emails" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"email_1"}`))
	}))
	defer server.Close()

	sender, err := NewResend(server.URL, "re_secret", "no-reply@example.com", "Dynamis Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(context.Background(), "invitee@example.com", "Subject", "Body"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotAuth != "Bearer re_secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["from"] != `"Dynamis Team" <no-reply@example.com>` {
		t.Fatalf("from = %v", gotBody["from"])
	}
	to, _ := gotBody["to"].([]any)
	if len(to) != 1 || to[0] != "invitee@example.com" || gotBody["subject"] != "Subject" || gotBody["text"] != "Body" {
		t.Fatalf("payload = %v", gotBody)
	}
}

func TestResendRejectedKeyIsReportedWithoutEchoingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"API key is invalid re_secret"}`))
	}))
	defer server.Close()

	sender, err := NewResend(server.URL, "re_secret", "no-reply@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	err = sender.Send(context.Background(), "invitee@example.com", "s", "b")
	if err == nil || err.Error() != "Resend rejected the message" {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestResendDomainsListsSenderDomainStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/domains" || r.Header.Get("Authorization") != "Bearer re_secret" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"name":"example.com","status":"verified"},{"name":"other.test","status":"pending"}]}`))
	}))
	defer server.Close()

	domains, err := ResendDomains(context.Background(), server.URL, "re_secret")
	if err != nil {
		t.Fatalf("ResendDomains() error = %v", err)
	}
	if len(domains) != 2 || domains[0].Name != "example.com" || domains[0].Status != "verified" || domains[1].Status != "pending" {
		t.Fatalf("domains = %+v", domains)
	}
}

func TestResendDomainsBadKeyFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	if _, err := ResendDomains(context.Background(), server.URL, "re_bad"); err == nil {
		t.Fatal("ResendDomains() with rejected key returned nil error")
	}
}
