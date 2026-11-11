package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func validCallbackParams() OIDCCallbackParams {
	return OIDCCallbackParams{
		Flow:           "login",
		BrowserSession: "test-browser-session-id",
		StateCookie:    "test-state-token-123",
		StateQuery:     "test-state-token-123",
		Verifier:       "test-pkce-verifier-abc",
		Nonce:          "test-nonce-token-456",
		Code:           "test-auth-code-789",
	}
}

func TestValidateOIDCCallbackParams_Valid(t *testing.T) {
	t.Run("valid login flow", func(t *testing.T) {
		p := validCallbackParams()
		p.Flow = "login"
		if err := validateOIDCCallbackParams(p); err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("valid link flow", func(t *testing.T) {
		p := validCallbackParams()
		p.Flow = "link"
		if err := validateOIDCCallbackParams(p); err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("valid with State field alias", func(t *testing.T) {
		p := validCallbackParams()
		p.StateQuery = ""
		p.State = p.StateCookie
		if err := validateOIDCCallbackParams(p); err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("valid with PKCEVerifier field alias", func(t *testing.T) {
		p := validCallbackParams()
		p.Verifier = ""
		p.PKCEVerifier = "test-pkce-verifier-abc"
		if err := validateOIDCCallbackParams(p); err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})
}

func TestValidateOIDCCallbackParams_InvalidFlow(t *testing.T) {
	tests := []struct {
		name string
		flow string
	}{
		{name: "empty flow", flow: ""},
		{name: "uppercase LOGIN", flow: "LOGIN"},
		{name: "register flow", flow: "register"},
		{name: "oauth flow", flow: "oauth"},
		{name: "arbitrary string", flow: "unknown-flow"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := validCallbackParams()
			p.Flow = tc.flow
			err := validateOIDCCallbackParams(p)
			if !errors.Is(err, ErrInvalidFlow) {
				t.Fatalf("expected ErrInvalidFlow, got: %v", err)
			}
		})
	}
}

func TestValidateOIDCCallbackParams_MissingBrowserSession(t *testing.T) {
	for _, flow := range []string{"login", "link"} {
		t.Run("flow "+flow, func(t *testing.T) {
			p := validCallbackParams()
			p.Flow = flow
			p.BrowserSession = ""
			err := validateOIDCCallbackParams(p)
			if !errors.Is(err, ErrMissingBrowserSession) {
				t.Fatalf("expected ErrMissingBrowserSession, got: %v", err)
			}
		})
	}
}

func TestValidateOIDCCallbackParams_StateMismatch(t *testing.T) {
	tests := []struct {
		name        string
		stateCookie string
		stateQuery  string
	}{
		{name: "empty cookie and empty query", stateCookie: "", stateQuery: ""},
		{name: "empty cookie with non-empty query", stateCookie: "", stateQuery: "query-token"},
		{name: "non-empty cookie with empty query", stateCookie: "cookie-token", stateQuery: ""},
		{name: "mismatched values", stateCookie: "cookie-token-1", stateQuery: "cookie-token-2"},
		{name: "trailing whitespace in query", stateCookie: "token", stateQuery: "token "},
		{name: "case mismatch", stateCookie: "token-ABC", stateQuery: "token-abc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := validCallbackParams()
			p.StateCookie = tc.stateCookie
			p.StateQuery = tc.stateQuery
			p.State = ""
			err := validateOIDCCallbackParams(p)
			if !errors.Is(err, ErrStateMismatch) {
				t.Fatalf("expected ErrStateMismatch, got: %v", err)
			}
		})
	}
}

func TestValidateOIDCCallbackParams_MissingCryptoParams(t *testing.T) {
	tests := []struct {
		name     string
		verifier string
		nonce    string
	}{
		{name: "empty verifier", verifier: "", nonce: "nonce-123"},
		{name: "empty nonce", verifier: "verifier-123", nonce: ""},
		{name: "both empty", verifier: "", nonce: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := validCallbackParams()
			p.Verifier = tc.verifier
			p.PKCEVerifier = ""
			p.Nonce = tc.nonce
			err := validateOIDCCallbackParams(p)
			if !errors.Is(err, ErrMissingCryptoParams) {
				t.Fatalf("expected ErrMissingCryptoParams, got: %v", err)
			}
		})
	}
}

func TestValidateOIDCCallbackParams_MissingAuthCode(t *testing.T) {
	p := validCallbackParams()
	p.Code = ""
	err := validateOIDCCallbackParams(p)
	if !errors.Is(err, ErrMissingAuthCode) {
		t.Fatalf("expected ErrMissingAuthCode, got: %v", err)
	}
}

func TestValidateOIDCCallbackParams_Precedence(t *testing.T) {
	t.Run("invalid flow takes precedence over missing browser session", func(t *testing.T) {
		p := validCallbackParams()
		p.Flow = "invalid"
		p.BrowserSession = ""
		err := validateOIDCCallbackParams(p)
		if !errors.Is(err, ErrInvalidFlow) {
			t.Fatalf("expected ErrInvalidFlow, got: %v", err)
		}
	})

	t.Run("missing browser session takes precedence over state mismatch", func(t *testing.T) {
		p := validCallbackParams()
		p.BrowserSession = ""
		p.StateCookie = "mismatch-1"
		p.StateQuery = "mismatch-2"
		err := validateOIDCCallbackParams(p)
		if !errors.Is(err, ErrMissingBrowserSession) {
			t.Fatalf("expected ErrMissingBrowserSession, got: %v", err)
		}
	})

	t.Run("state mismatch takes precedence over missing crypto params", func(t *testing.T) {
		p := validCallbackParams()
		p.StateCookie = "mismatch-1"
		p.StateQuery = "mismatch-2"
		p.Verifier = ""
		p.Nonce = ""
		err := validateOIDCCallbackParams(p)
		if !errors.Is(err, ErrStateMismatch) {
			t.Fatalf("expected ErrStateMismatch, got: %v", err)
		}
	})

	t.Run("missing crypto params takes precedence over missing auth code", func(t *testing.T) {
		p := validCallbackParams()
		p.Verifier = ""
		p.Code = ""
		err := validateOIDCCallbackParams(p)
		if !errors.Is(err, ErrMissingCryptoParams) {
			t.Fatalf("expected ErrMissingCryptoParams, got: %v", err)
		}
	})
}

func TestValidateOIDCCallbackParams_TableDrivenMatrix(t *testing.T) {
	matrix := []struct {
		name    string
		params  OIDCCallbackParams
		wantErr error
	}{
		{
			name:    "all valid login",
			params:  validCallbackParams(),
			wantErr: nil,
		},
		{
			name: "all valid link",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.Flow = "link"
				return p
			}(),
			wantErr: nil,
		},
		{
			name: "invalid flow bad-flow",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.Flow = "bad-flow"
				return p
			}(),
			wantErr: ErrInvalidFlow,
		},
		{
			name: "missing browser session",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.BrowserSession = ""
				return p
			}(),
			wantErr: ErrMissingBrowserSession,
		},
		{
			name: "missing state cookie",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.StateCookie = ""
				return p
			}(),
			wantErr: ErrStateMismatch,
		},
		{
			name: "missing state query",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.StateQuery = ""
				p.State = ""
				return p
			}(),
			wantErr: ErrStateMismatch,
		},
		{
			name: "state mismatch",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.StateQuery = "different-query"
				return p
			}(),
			wantErr: ErrStateMismatch,
		},
		{
			name: "missing verifier",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.Verifier = ""
				p.PKCEVerifier = ""
				return p
			}(),
			wantErr: ErrMissingCryptoParams,
		},
		{
			name: "missing nonce",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.Nonce = ""
				return p
			}(),
			wantErr: ErrMissingCryptoParams,
		},
		{
			name: "missing code",
			params: func() OIDCCallbackParams {
				p := validCallbackParams()
				p.Code = ""
				return p
			}(),
			wantErr: ErrMissingAuthCode,
		},
	}

	for _, tc := range matrix {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOIDCCallbackParams(tc.params)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
			} else {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

type testLogCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (c *testLogCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *testLogCapture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r)
	return nil
}
func (c *testLogCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *testLogCapture) WithGroup(string) slog.Handler      { return c }

func (c *testLogCapture) hasError(target error) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.records {
		var matched bool
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "error" {
				if err, ok := a.Value.Any().(error); ok && errors.Is(err, target) {
					matched = true
					return false
				}
				if a.Value.String() == target.Error() {
					matched = true
					return false
				}
			}
			return true
		})
		if matched {
			return true
		}
	}
	return false
}

func TestOIDCCallbackHandler_ValidationFailures(t *testing.T) {
	ctx := context.Background()
	oidcRegistry, err := identity.NewOIDCRegistry(ctx, config.OIDC{})
	if err != nil {
		t.Fatal(err)
	}

	_, auth, itemService, _, _ := testWeb(t, 0)
	handler, err := NewHandlerWithServices(auth, itemService, nil, nil, oidcRegistry, config.HTTP{}, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name          string
		queryState    string
		queryCode     string
		cookies       []*http.Cookie
		expectedError error
	}{
		{
			name:          "no cookies and no query parameters",
			queryState:    "",
			queryCode:     "",
			cookies:       nil,
			expectedError: ErrInvalidFlow,
		},
		{
			name:       "invalid flow cookie",
			queryState: "st-1",
			queryCode:  "code-1",
			cookies: []*http.Cookie{
				{Name: "oidc_flow", Value: "unknown"},
				{Name: "oidc_browser", Value: "sess-1"},
				{Name: "oidc_state", Value: "st-1"},
				{Name: "oidc_verifier", Value: "ver-1"},
				{Name: "oidc_nonce", Value: "non-1"},
			},
			expectedError: ErrInvalidFlow,
		},
		{
			name:       "missing browser session cookie",
			queryState: "st-1",
			queryCode:  "code-1",
			cookies: []*http.Cookie{
				{Name: "oidc_flow", Value: "login"},
				{Name: "oidc_state", Value: "st-1"},
				{Name: "oidc_verifier", Value: "ver-1"},
				{Name: "oidc_nonce", Value: "non-1"},
			},
			expectedError: ErrMissingBrowserSession,
		},
		{
			name:       "state mismatch query differs from cookie",
			queryState: "state-query-val",
			queryCode:  "code-1",
			cookies: []*http.Cookie{
				{Name: "oidc_flow", Value: "login"},
				{Name: "oidc_browser", Value: "sess-1"},
				{Name: "oidc_state", Value: "state-cookie-val"},
				{Name: "oidc_verifier", Value: "ver-1"},
				{Name: "oidc_nonce", Value: "non-1"},
			},
			expectedError: ErrStateMismatch,
		},
		{
			name:       "missing verifier cookie",
			queryState: "st-1",
			queryCode:  "code-1",
			cookies: []*http.Cookie{
				{Name: "oidc_flow", Value: "login"},
				{Name: "oidc_browser", Value: "sess-1"},
				{Name: "oidc_state", Value: "st-1"},
				{Name: "oidc_nonce", Value: "non-1"},
			},
			expectedError: ErrMissingCryptoParams,
		},
		{
			name:       "missing nonce cookie",
			queryState: "st-1",
			queryCode:  "code-1",
			cookies: []*http.Cookie{
				{Name: "oidc_flow", Value: "login"},
				{Name: "oidc_browser", Value: "sess-1"},
				{Name: "oidc_state", Value: "st-1"},
				{Name: "oidc_verifier", Value: "ver-1"},
			},
			expectedError: ErrMissingCryptoParams,
		},
		{
			name:       "missing auth code in query",
			queryState: "st-1",
			queryCode:  "",
			cookies: []*http.Cookie{
				{Name: "oidc_flow", Value: "login"},
				{Name: "oidc_browser", Value: "sess-1"},
				{Name: "oidc_state", Value: "st-1"},
				{Name: "oidc_verifier", Value: "ver-1"},
				{Name: "oidc_nonce", Value: "non-1"},
			},
			expectedError: ErrMissingAuthCode,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			capture := &testLogCapture{}
			origLogger := slog.Default()
			slog.SetDefault(slog.New(capture))
			defer slog.SetDefault(origLogger)

			target := "/auth/oidc/test-provider/callback"
			params := url.Values{}
			if tc.queryState != "" {
				params.Set("state", tc.queryState)
			}
			if tc.queryCode != "" {
				params.Set("code", tc.queryCode)
			}
			if len(params) > 0 {
				target += "?" + params.Encode()
			}

			resp := request(handler.Routes(), http.MethodGet, target, nil, tc.cookies, nil)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected status 401 Unauthorized, got: %d", resp.Code)
			}
			if !strings.Contains(resp.Body.String(), "The identity provider sign-in could not be completed.") {
				t.Fatalf("expected flash error message in response body, got: %s", resp.Body.String())
			}
			if !capture.hasError(tc.expectedError) {
				t.Fatalf("expected slog to capture error %v, records were: %+v", tc.expectedError, capture.records)
			}
		})
	}
}
