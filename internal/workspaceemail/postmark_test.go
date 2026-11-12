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

// fakePostmark stands in for api.postmarkapp.com. It accepts one server token and
// one account token, and records every call so tests can prove nothing was sent.
type fakePostmark struct {
	mu           sync.Mutex
	serverToken  string
	accountToken string
	sent         int
	calls        []string
}

func (f *fakePostmark) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/server":
			if r.Header.Get("X-Postmark-Server-Token") != f.serverToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"ID":1,"Name":"Dynamis"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/domains":
			if r.Header.Get("X-Postmark-Account-Token") != f.accountToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `{"TotalCount":1,"Domains":[{"Name":"example.com","DKIMVerified":true,"ReturnPathDomainVerified":true}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/email":
			if r.Header.Get("X-Postmark-Server-Token") != f.serverToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			f.sent++
			_, _ = io.WriteString(w, `{"ErrorCode":0,"Message":"OK"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func postmarkInput(serverToken, accountToken string) SaveInput {
	return SaveInput{
		Provider: ProviderPostmark, SenderAddress: "mailer@example.com", SenderName: "Dynamis",
		Password: serverToken, AccountToken: accountToken,
	}
}

func TestPostmarkSaveStoresOnlyAfterPassingChecks(t *testing.T) {
	ctx := context.Background()
	fake := &fakePostmark{serverToken: "pm_server_good", accountToken: "pm_account_good"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	service, db, owner, _ := testService(t, nil)
	service.postmarkBase = server.URL
	if err := service.Save(ctx, owner, owner.WorkspaceID, postmarkInput("pm_server_good", "pm_account_good"), identity.AuditContext{}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var ciphertext, accountCiphertext, provider, domainStatus string
	if err := db.QueryRow(`SELECT encrypted_password, encrypted_account_token, provider, sender_domain_status
		FROM workspace_email_settings`).Scan(&ciphertext, &accountCiphertext, &provider, &domainStatus); err != nil {
		t.Fatal(err)
	}
	if provider != ProviderPostmark || domainStatus != "verified" || accountCiphertext == "" {
		t.Fatalf("stored provider=%q status=%q account=%q", provider, domainStatus, accountCiphertext)
	}
	if strings.Contains(ciphertext, "pm_server_good") || strings.Contains(accountCiphertext, "pm_account_good") {
		t.Fatal("token stored in plaintext")
	}
	settings, err := service.Get(ctx, owner, owner.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Provider != ProviderPostmark || settings.DomainStatus != "verified" || !settings.PasswordSet || !settings.AccountTokenSet {
		t.Fatalf("Get() = %+v", settings)
	}
	if fake.sent != 0 {
		t.Fatalf("save sent %d messages, want none", fake.sent)
	}
}

func TestPostmarkBadTokensBlockSave(t *testing.T) {
	ctx := context.Background()
	fake := &fakePostmark{serverToken: "pm_server_good", accountToken: "pm_account_good"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	for name, input := range map[string]SaveInput{
		"bad server token":  postmarkInput("pm_server_bad", "pm_account_good"),
		"bad account token": postmarkInput("pm_server_good", "pm_account_bad"),
	} {
		t.Run(name, func(t *testing.T) {
			service, _, owner, _ := testService(t, nil)
			service.postmarkBase = server.URL
			err := service.Save(ctx, owner, owner.WorkspaceID, input, identity.AuditContext{})
			var checkErr *LoginError
			if !errors.As(err, &checkErr) || strings.Contains(err.Error(), "pm_") {
				t.Fatalf("Save() error = %v, want LoginError without secrets", err)
			}
			settings, err := service.Get(ctx, owner, owner.WorkspaceID)
			if err != nil || settings.Configured {
				t.Fatalf("failed check stored settings: %+v, err = %v", settings, err)
			}
		})
	}
}

func TestPostmarkBlankTokensKeepStoredValuesForSameProvider(t *testing.T) {
	ctx := context.Background()
	fake := &fakePostmark{serverToken: "pm_server_good", accountToken: "pm_account_good"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	service, _, owner, _ := testService(t, nil)
	service.postmarkBase = server.URL
	if err := service.Save(ctx, owner, owner.WorkspaceID, postmarkInput("pm_server_good", "pm_account_good"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.Save(ctx, owner, owner.WorkspaceID, postmarkInput("", ""), identity.AuditContext{}); err != nil {
		t.Fatalf("blank tokens save error = %v, want stored tokens reused", err)
	}
	settings, err := service.Get(ctx, owner, owner.WorkspaceID)
	if err != nil || !settings.AccountTokenSet || settings.DomainStatus != "verified" {
		t.Fatalf("Get() after blank save = %+v, err = %v", settings, err)
	}
}

func TestPostmarkFirstSaveRequiresAccountToken(t *testing.T) {
	ctx := context.Background()
	service, _, owner, _ := testService(t, nil)
	err := service.Save(ctx, owner, owner.WorkspaceID, postmarkInput("pm_server_good", ""), identity.AuditContext{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Save() error = %v, want ErrInvalidInput", err)
	}
}

func TestPostmarkInvitationIsDeliveredThroughPostmark(t *testing.T) {
	ctx := context.Background()
	fake := &fakePostmark{serverToken: "pm_server_good", accountToken: "pm_account_good"}
	server := httptest.NewServer(fake.handler())
	defer server.Close()

	instance := &recordingSender{}
	service, _, owner, _ := testService(t, instance)
	service.postmarkBase = server.URL
	if err := service.Save(ctx, owner, owner.WorkspaceID, postmarkInput("pm_server_good", "pm_account_good"), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if err := service.SendFor(ctx, owner.WorkspaceID.String(), "invitee@example.com", "s", "b"); err != nil {
		t.Fatalf("SendFor() error = %v", err)
	}
	if fake.sent != 1 || len(instance.sent) != 0 {
		t.Fatalf("postmark sent = %d, instance sent = %v; want Postmark only", fake.sent, instance.sent)
	}
}
