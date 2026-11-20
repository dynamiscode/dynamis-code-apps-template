package web

import (
	"bytes"
	"context"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSetupPresentsRequiredDetailsBeforeDeferrableTasks(t *testing.T) {
	handler, _ := testUnbootstrappedWeb(t, "")
	for _, locale := range []string{"en", "es"} {
		t.Run(locale, func(t *testing.T) {
			response := request(handler, http.MethodGet, "/setup", nil, nil, map[string]string{"Accept-Language": locale})
			body := response.Body.String()
			if response.Code != http.StatusOK || !strings.Contains(body, `class="brand auth-brand"`) {
				t.Fatalf("setup page status = %d; missing auth brand", response.Code)
			}
			if !strings.Contains(body, `Dynamis Code`) || !strings.Contains(body, `id="email"`) {
				t.Fatal("setup page missing product brand or account field")
			}
			if strings.Index(body, `id="email"`) > strings.Index(body, `id="workspace"`) {
				t.Fatal("workspace details appear before required account details")
			}
			if !strings.Contains(body, `data-setup-optional`) {
				t.Fatal("setup page does not explain that optional tasks can wait")
			}
			for _, marker := range []string{`class="input w-full"`, `class="btn btn-primary"`, `action="/setup" method="post"`} {
				if !strings.Contains(body, marker) {
					t.Fatalf("setup view missing marker %q", marker)
				}
			}
			csrf := responseCookie(response, "setup_csrf")
			expired := request(handler, http.MethodPost, "/setup", mapValues("email", "owner@example.com", "workspace", "Retained workspace"), nil, nil)
			if expired.Code != http.StatusForbidden || !strings.Contains(expired.Body.String(), `tabindex="-1"`) {
				t.Fatalf("expired setup form status = %d; missing focused alert", expired.Code)
			}
			invalid := request(handler, http.MethodPost, "/setup", mapValues("email", "owner@example.com", "workspace", "Retained workspace", "csrf", csrf.Value), []*http.Cookie{csrf}, nil)
			if invalid.Code != http.StatusUnprocessableEntity {
				t.Fatalf("setup validation status = %d", invalid.Code)
			}
			for _, marker := range []string{`tabindex="-1"`, `class="alert alert-error error"`, `value="owner@example.com"`, `value="Retained workspace"`} {
				if !strings.Contains(invalid.Body.String(), marker) {
					t.Fatalf("setup validation view missing marker %q", marker)
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
				t.Fatalf("GET %s status = %d", path, response.Code)
			}
			if (!strings.Contains(body, `class="brand auth-brand"`) && !strings.Contains(body, `class="brand"`)) ||
				!strings.Contains(body, brand) {
				t.Fatalf("%s missing generated-app brand", path)
			}
			if strings.Contains(path, "password-reset") && !strings.Contains(body, `method="post"`) {
				t.Fatalf("%s missing ordinary form marker", path)
			}
			if strings.Contains(path, "password-reset") {
				for _, marker := range []string{`class="brand auth-brand"`, `class="input w-full"`, `class="btn btn-primary"`, `id="main"`, `data-theme="system"`} {
					if !strings.Contains(body, marker) {
						t.Fatalf("%s missing auth layout marker %q", path, marker)
					}
				}
			}
			if strings.Contains(path, "verify-email") && !strings.Contains(body, `tabindex="-1"`) {
				t.Fatal("invalid verification link missing focused error")
			}
			if path == "/login" {
				for _, marker := range []string{`class="input w-full"`, `class="btn btn-primary"`, `action="/login" method="post"`, `name="csrf"`} {
					if !strings.Contains(body, marker) {
						t.Fatalf("login missing marker %q", marker)
					}
				}
				if !strings.Contains(body, `class="auth-recovery-link" href="/password-reset"`) {
					t.Fatal("login missing accessible password recovery target")
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
	data := viewmodel.PageData{Locale: "en", Theme: "light", Catalog: catalog, OIDCProviders: []identity.OIDCProviderInfo{{ID: "company", Name: "Company SSO"}}}
	if err := authLogin(data).Render(context.Background(), &body); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`href="/auth/oidc/company"`, `Company SSO`, `class="btn"`, `aria-labelledby="auth-provider-heading"`} {
		if !strings.Contains(body.String(), marker) {
			t.Fatalf("provider choice missing marker %q", marker)
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
		t.Fatalf("MFA page status = %d; missing auth brand", response.Code)
	}
	for _, marker := range []string{`action="/mfa/totp" method="post"`, `action="/mfa/recovery" method="post"`, `id="totp-code"`, `id="recovery-code"`, `class="btn btn-primary"`} {
		if !strings.Contains(body, marker) {
			t.Fatalf("MFA page missing marker %q", marker)
		}
	}
	csrf := responseCookie(response, "mfa_csrf")
	retry := request(handler, http.MethodPost, "/mfa/totp", mapValues("csrf", csrf.Value, "code", "invalid"), []*http.Cookie{
		{Name: "mfa_challenge", Value: challenge.Token}, csrf,
	}, map[string]string{"Accept-Language": "es"})
	if retry.Code != http.StatusUnauthorized || !strings.Contains(retry.Body.String(), `class="alert alert-error error"`) ||
		!strings.Contains(retry.Body.String(), "El código de verificación no es válido.") {
		t.Fatalf("MFA retry status = %d; expected localized verification error", retry.Code)
	}
}

func TestSecurityPageUsesTemplAndPreservesFactorForms(t *testing.T) {
	handler, auth, _, _, owner := testWebWithMFA(t, 10, true)
	ctx := t.Context()
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	cookies = append(cookies, &http.Cookie{Name: "locale", Value: "en"})
	page := request(handler, http.MethodGet, "/security", nil, cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK {
		t.Fatalf("security page status = %d", page.Code)
	}
	for _, marker := range []string{
		`class="settings-panel"`, `class="input w-full"`, `class="btn btn-primary"`,
		`action="/api/v1/auth/mfa/passkeys/register/options" method="post"`,
		`action="/security/totp/start" method="post"`, `id="security-linked-identity-heading"`,
		`name="csrf"`, `aria-labelledby="security-mfa-heading"`, `aria-labelledby="security-linked-identity-heading"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("security page missing %q", marker)
		}
	}
	if strings.Contains(body, `name="challenge"`) || strings.Contains(body, `name="code"`) || strings.Contains(body, "otpauth://") {
		t.Fatal("inactive security page leaked TOTP enrollment credentials")
	}

	start := request(handler, http.MethodPost, "/security/totp/start", url.Values{
		"csrf": {session.CSRFSecret}, "password": {"wrong-password"},
	}, cookies, nil)
	if start.Code != http.StatusUnauthorized || !strings.Contains(start.Body.String(), `tabindex="-1"`) ||
		!strings.Contains(start.Body.String(), "Fresh authentication failed.") {
		t.Fatalf("invalid TOTP enrollment status = %d; missing accessible failure feedback", start.Code)
	}
}

func TestSecurityPageRendersReauthenticatedOIDCLinkForm(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	data := viewmodel.PageData{
		Locale: "en", Theme: "dark", Catalog: catalog, CSRF: "csrf-value",
		MFAStatus: identity.MFAStatus{Enabled: true}, MFARecoveryCodes: []string{"recovery-one", "recovery-two"},
		OIDCProviders: []identity.OIDCProviderInfo{{ID: "company", Name: "Company SSO"}},
	}
	if err := securityPage(data).Render(t.Context(), &body); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`<form action="/security" method="post">`, `name="csrf" value="csrf-value"`,
		`<option value="company">Company SSO</option>`, `id="reauth-password"`,
		`name="password"`, `type="password"`, `autocomplete="current-password"`, `required`, `class="input w-full"`,
	} {
		if !strings.Contains(body.String(), marker) {
			t.Errorf("OIDC linking form missing %q", marker)
		}
	}
	if !strings.Contains(body.String(), "mockup-code") || !strings.Contains(body.String(), "recovery-one") || !strings.Contains(body.String(), "recovery-two") {
		t.Fatal("recovery codes lost their per-code presentation")
	}
}
