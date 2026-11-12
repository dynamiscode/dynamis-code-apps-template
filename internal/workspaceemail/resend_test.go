package workspaceemail

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// fakeResend stands in for api.resend.com. It accepts only goodKey and records sends.
type fakeResend struct {
	mu    sync.Mutex
	sent  int
	calls []string
}

func (f *fakeResend) handler(goodKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer "+goodKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/domains":
			_, _ = io.WriteString(w, `{"object":"list","data":[{"name":"example.com","status":"verified"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/emails":
			f.sent++
			_, _ = io.WriteString(w, `{"id":"email_1"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func resendInput(key string) SaveInput {
	return SaveInput{
		Provider: ProviderResend, SenderAddress: "mailer@example.com", SenderName: "Dynamis",
		Password: key,
	}
}

func TestResendSaveStoresOnlyAfterPassingCheck(t *testing.T) {
	ctx := context.Background()
	fake := &fakeResend{}
	server := httptest.NewServer(fake.handler("re_good"))
	defer server.Close()

	service, db, owner, _ := testService(t, nil)
	service.resendBase = server.URL
	if err := service.Save(ctx, owner, owner.WorkspaceID, resendInput("re_good"), identity.AuditContext{}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var ciphertext, provider, domainStatus string
	if err := db.QueryRow("SELECT encrypted_password, provider, sender_domain_status FROM workspace_email_settings").Scan(&ciphertext, &provider, &domainStatus); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "re_good") || provider != ProviderResend || domainStatus != "verified" {
		t.Fatalf("stored provider=%q status=%q ciphertext leaked=%v", provider, domainStatus, strings.Contains(ciphertext, "re_good"))
	}
	settings, err := service.Get(ctx, owner, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Configured || settings.Provider != ProviderResend || settings.DomainStatus != "verified" || !settings.PasswordSet {
		t.Fatalf("Get() = %+v", settings)
	}
}

func TestResendBadTokenBlocksSave(t *testing.T) {
	ctx := context.Background()
	fake := &fakeResend{}
	server := httptest.NewServer(fake.handler("re_good"))
	defer server.Close()

	service, _, owner, _ := testService(t, nil)
	service.resendBase = server.URL
	err := service.Save(ctx, owner, owner.WorkspaceID, resendInput("re_bad"), identity.AuditContext{})
	var checkErr *LoginError
	if !errors.As(err, &checkErr) {
		t.Fatalf("Save() error = %v, want check failure", err)
	}
	settings, err := service.Get(ctx, owner, owner.WorkspaceID)
	if err != nil || settings.Configured {
		t.Fatalf("bad token stored settings: %+v, err = %v", settings, err)
	}
}

func TestResendBlankKeyReusesStoredKeyForSameProvider(t *testing.T) {
	ctx := context.Background()
	fake := &fakeResend{}
	server := httptest.NewServer(fake.handler("re_good"))
	defer server.Close()

	service, _, owner, _ := testService(t, nil)
	service.resendBase = server.URL
	if err := service.Save(ctx, owner, owner.WorkspaceID, resendInput("re_good"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.Save(ctx, owner, owner.WorkspaceID, resendInput(""), identity.AuditContext{}); err != nil {
		t.Fatalf("blank key save error = %v, want stored key reused", err)
	}
}

func TestResendInvitationIsDeliveredThroughResend(t *testing.T) {
	ctx := context.Background()
	fake := &fakeResend{}
	server := httptest.NewServer(fake.handler("re_good"))
	defer server.Close()

	instance := &recordingSender{}
	service, _, owner, _ := testService(t, instance)
	service.resendBase = server.URL
	if err := service.Save(ctx, owner, owner.WorkspaceID, resendInput("re_good"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.SendFor(ctx, owner.WorkspaceID.String(), "invitee@example.com", "s", "b"); err != nil {
		t.Fatalf("SendFor() error = %v", err)
	}
	if fake.sent != 1 || len(instance.sent) != 0 {
		t.Fatalf("resend sent = %d, instance sent = %v; want Resend only", fake.sent, instance.sent)
	}
}
