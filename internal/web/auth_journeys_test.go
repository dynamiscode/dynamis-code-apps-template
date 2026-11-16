package web

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestSetupPresentsRequiredDetailsBeforeDeferrableTasks(t *testing.T) {
	handler, _ := testUnbootstrappedWeb(t, "")
	for _, locale := range []string{"en", "es"} {
		t.Run(locale, func(t *testing.T) {
			response := request(handler, http.MethodGet, "/setup", nil, nil, map[string]string{"Accept-Language": locale})
			body := response.Body.String()
			if response.Code != http.StatusOK || !strings.Contains(body, `class="brand auth-brand"`) {
				t.Fatalf("setup page = %d, %s", response.Code, body)
			}
			if !strings.Contains(body, `Dynamis Code`) || !strings.Contains(body, `id="email"`) {
				t.Fatalf("setup page misses product brand or account field: %s", body)
			}
			if strings.Index(body, `id="email"`) > strings.Index(body, `id="workspace"`) {
				t.Fatal("workspace details appear before required account details")
			}
			if !strings.Contains(body, `data-setup-optional`) {
				t.Fatal("setup page does not explain that optional tasks can wait")
			}
			for _, marker := range []string{`class="input"`, `class="btn btn-primary"`, `action="/setup" method="post"`} {
				if !strings.Contains(body, marker) {
					t.Fatalf("setup view lacks DaisyUI marker %q: %s", marker, body)
				}
			}
			csrf := responseCookie(response, "setup_csrf")
			expired := request(handler, http.MethodPost, "/setup", mapValues("email", "owner@example.com", "workspace", "Retained workspace"), nil, nil)
			if expired.Code != http.StatusForbidden || !strings.Contains(expired.Body.String(), `role="alert" tabindex="-1"`) {
				t.Fatalf("expired setup form = %d, %s", expired.Code, expired.Body.String())
			}
			invalid := request(handler, http.MethodPost, "/setup", mapValues("email", "owner@example.com", "workspace", "Retained workspace", "csrf", csrf.Value), []*http.Cookie{csrf}, nil)
			if invalid.Code != http.StatusUnprocessableEntity {
				t.Fatalf("setup validation = %d, %s", invalid.Code, invalid.Body.String())
			}
			for _, marker := range []string{`role="alert" tabindex="-1"`, `class="alert alert-error error"`, `value="owner@example.com"`, `value="Retained workspace"`} {
				if !strings.Contains(invalid.Body.String(), marker) {
					t.Fatalf("setup validation view lacks %q: %s", marker, invalid.Body.String())
				}
			}
		})
	}
}

func TestAuthenticationPagesUseBrandAndOrdinaryFormPaths(t *testing.T) {
	webHandler, _, _, _, _ := testWebRaw(t, 10)
	handler := webHandler.Routes()
	brand := webHandler.catalog.Translate(i18n.English, "common.product_name", nil)
	for _, path := range []string{"/login", "/password-reset", "/password-reset/unavailable", "/account/verify-email/unavailable"} {
		t.Run(path, func(t *testing.T) {
			response := request(handler, http.MethodGet, path, nil, nil, nil)
			body := response.Body.String()
			if response.Code != http.StatusOK && response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("GET %s = %d, %s", path, response.Code, body)
			}
			if (!strings.Contains(body, `class="brand auth-brand"`) && !strings.Contains(body, `class="brand"`)) ||
				!strings.Contains(body, brand) {
				t.Fatalf("%s lacks the generated-app brand: %s", path, body)
			}
			if strings.Contains(path, "password-reset") && !strings.Contains(body, `method="post"`) {
				t.Fatalf("%s lacks the ordinary form path: %s", path, body)
			}
			if strings.Contains(path, "password-reset") {
				for _, marker := range []string{`class="brand auth-brand"`, `class="input"`, `class="btn btn-primary"`, `id="main"`, `data-theme="system"`} {
					if !strings.Contains(body, marker) {
						t.Fatalf("%s lacks auth layout marker %q: %s", path, marker, body)
					}
				}
			}
			if path == "/login" {
				for _, marker := range []string{`class="input"`, `class="btn btn-primary"`, `action="/login" method="post"`, `name="csrf"`} {
					if !strings.Contains(body, marker) {
						t.Fatalf("login lacks %q: %s", marker, body)
					}
				}
			}
		})
	}
}

func TestConfiguredOIDCProviderChoiceIsOrdinaryLink(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	data := pageData{Locale: "en", Theme: "light", Catalog: catalog, OIDCProviders: []identity.OIDCProviderInfo{{ID: "company", Name: "Company SSO"}}}
	if err := authLogin(data).Render(context.Background(), &body); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`href="/auth/oidc/company"`, `Company SSO`, `class="btn"`, `aria-labelledby="auth-provider-heading"`} {
		if !strings.Contains(body.String(), marker) {
			t.Fatalf("provider choice lacks %q: %s", marker, body.String())
		}
	}
	if strings.Contains(body.String(), "javascript:") {
		t.Fatal("provider choice must remain an ordinary link")
	}
}

func mapValues(values ...string) url.Values {
	form := url.Values{}
	for i := 0; i+1 < len(values); i += 2 {
		form.Set(values[i], values[i+1])
	}
	return form
}

func TestMFAPageKeepsFormFallbacksAvailable(t *testing.T) {
	handler, auth, _, _, owner := testWebWithMFA(t, 10, true)
	ctx := t.Context()
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := auth.BeginTOTPEnrollment(ctx, owner.UserID, session.ID, "long-enough-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CompleteTOTPEnrollment(ctx, session.ID, enrollment.Challenge, webTOTPCode(enrollment.Secret, time.Now()), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	challenge, err := auth.BeginMFALogin(ctx, owner.UserID, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	response := request(handler, http.MethodGet, "/mfa", nil, []*http.Cookie{{Name: "mfa_challenge", Value: challenge.Token}}, nil)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `class="brand auth-brand"`) {
		t.Fatalf("MFA page = %d, %s", response.Code, body)
	}
	for _, marker := range []string{`action="/mfa/totp" method="post"`, `action="/mfa/recovery" method="post"`, `id="totp-code"`, `id="recovery-code"`} {
		if !strings.Contains(body, marker) {
			t.Fatalf("MFA page lacks %q: %s", marker, body)
		}
	}
}
