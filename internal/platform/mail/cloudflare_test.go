package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

const testCloudflareToken = "cf-test-token-value"

func testCloudflareConfig() config.CloudflareEmail {
	return config.CloudflareEmail{
		AccountID: "account-123", APIToken: testCloudflareToken, From: "no-reply@example.com",
	}
}

func TestCloudflareSendPostsMessageWithBearerToken(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"success":true,"errors":[],"messages":[],"result":{"delivered":["ann@example.com"],"permanent_bounces":[],"queued":[]}}`)
	}))
	defer server.Close()

	sender, err := NewCloudflare(testCloudflareConfig())
	if err != nil {
		t.Fatalf("NewCloudflare() error = %v", err)
	}
	sender.endpoint = server.URL

	if err := sender.Send(context.Background(), "ann@example.com", "Verify", "Open this link"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/accounts/account-123/email/sending/send" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer "+testCloudflareToken {
		t.Fatalf("Authorization header not sent as bearer token")
	}
	want := map[string]string{
		"to": "ann@example.com", "from": "no-reply@example.com",
		"subject": "Verify", "text": "Open this link",
	}
	for key, value := range want {
		if gotBody[key] != value {
			t.Fatalf("body[%q] = %v, want %q", key, gotBody[key], value)
		}
	}
}

func TestCloudflareSendReportsProviderRejectionWithoutSecrets(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":10000,"message":"echo `+testCloudflareToken+`"}],"result":null}`)
	}))
	defer server.Close()

	sender, err := NewCloudflare(testCloudflareConfig())
	if err != nil {
		t.Fatalf("NewCloudflare() error = %v", err)
	}
	sender.endpoint = server.URL

	err = sender.Send(context.Background(), "ann@example.com", "Verify", "body")
	if err == nil {
		t.Fatal("Send() accepted provider rejection")
	}
	if strings.Contains(err.Error(), testCloudflareToken) || strings.Contains(err.Error(), "echo") {
		t.Fatalf("Send() error leaked provider detail or token: %v", err)
	}
}

func TestCloudflareSendRejectsSuccessFalseEnvelope(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":10001,"message":"invalid"}],"result":null}`)
	}))
	defer server.Close()

	sender, err := NewCloudflare(testCloudflareConfig())
	if err != nil {
		t.Fatalf("NewCloudflare() error = %v", err)
	}
	sender.endpoint = server.URL

	if err := sender.Send(context.Background(), "ann@example.com", "Verify", "body"); err == nil {
		t.Fatal("Send() treated HTTP 200 with success=false as delivered")
	}
}

func TestNewSenderSelectsConfiguredProvider(t *testing.T) {
	t.Parallel()

	sender, err := NewSender(config.Mail{}, testCloudflareConfig())
	if err != nil {
		t.Fatalf("NewSender() error = %v", err)
	}
	if _, ok := sender.(*Cloudflare); !ok {
		t.Fatalf("NewSender() = %T, want *Cloudflare", sender)
	}
}

func TestCloudflareSendRejectsHeaderInjection(t *testing.T) {
	t.Parallel()

	sender, err := NewCloudflare(testCloudflareConfig())
	if err != nil {
		t.Fatalf("NewCloudflare() error = %v", err)
	}
	if err := sender.Send(context.Background(), "ann@example.com\r\nBcc: x@example.com", "Verify", "body"); err == nil {
		t.Fatal("Send() accepted CRLF in recipient")
	}
}

func TestNewCloudflareValidatesConfig(t *testing.T) {
	t.Parallel()

	sender, err := NewCloudflare(config.CloudflareEmail{})
	if err != nil || sender != nil {
		t.Fatalf("unconfigured NewCloudflare() = %v, %v; want nil, nil", sender, err)
	}
	invalid := testCloudflareConfig()
	invalid.From = "not-an-address"
	if _, err := NewCloudflare(invalid); err == nil {
		t.Fatal("NewCloudflare() accepted invalid sender address")
	} else if strings.Contains(err.Error(), testCloudflareToken) {
		t.Fatalf("NewCloudflare() error leaked token: %v", err)
	}
}
