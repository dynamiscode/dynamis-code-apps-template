package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestNewBuildsSQLiteApplication(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	app, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { app.Close() })

	if err := app.DB.PingContext(context.Background()); err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}
}

func TestCookieSecurityAtHTTPBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, secure, publicURL, host string
		status                        int
		cookieSecure                  bool
	}{
		{"default without URL", "", "", "app.example.com", http.StatusOK, true},
		{"misleading local URL", "", "http://localhost:8080", "app.example.com", http.StatusOK, true},
		{"public HTTPS", "", "https://app.example.com", "app.example.com", http.StatusOK, true},
		{"local HTTP", "false", "", "localhost:8080", http.StatusOK, false},
		{"local IPv6 HTTP", "false", "", "[::1]:8080", http.StatusOK, false},
		{"local mode rejects public host", "false", "http://localhost:8080", "app.example.com", http.StatusBadRequest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"APP_PUBLIC_URL": tc.publicURL}
			if tc.secure != "" {
				values["HTTP_SECURE"] = tc.secure
			}
			cfg, err := config.LoadFrom(func(key string) (string, bool) {
				value, ok := values[key]
				return value, ok
			})
			if err != nil {
				t.Fatal(err)
			}
			cfg.Database.SQLitePath = ":memory:"
			cfg.Database.MaxOpenConns = 1
			cfg.Database.MaxIdleConns = 1
			app, err := New(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { app.Close() })
			request := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/setup", nil)
			response := httptest.NewRecorder()
			app.Handler.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if tc.status != http.StatusOK {
				if len(response.Result().Cookies()) != 0 {
					t.Fatal("rejected host received cookies")
				}
				return
			}
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == "setup_csrf" {
					if cookie.Secure != tc.cookieSecure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
						t.Fatalf("setup cookie = %+v", cookie)
					}
					return
				}
			}
			t.Fatal("missing setup CSRF cookie")
		})
	}
}

func TestNewBootstrapsFirstOwnerFromEnvironment(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	cfg.Bootstrap = config.Bootstrap{
		AdminEmail: "owner@example.com", AdminPassword: "long-enough-password",
		AdminWorkspace: "Example",
	}
	app, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { app.Close() })

	bootstrapped, err := app.Identity.IsBootstrapped(context.Background())
	if err != nil || !bootstrapped {
		t.Fatalf("bootstrap state = %t, %v", bootstrapped, err)
	}
	userID, err := app.Identity.AuthenticateLocal(context.Background(), "owner@example.com", "long-enough-password")
	if err != nil || !app.Identity.IsInstanceAdmin(context.Background(), userID) {
		t.Fatalf("first instance admin = %q, %v", userID, err)
	}
}

func TestNewRejectsPartialBootstrapOnEmptyDatabase(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	cfg.Bootstrap.AdminEmail = "owner@example.com"
	cfg.Bootstrap.AdminPassword = "partial-password"
	_, err = New(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("partial bootstrap error = %v", err)
	}
	if strings.Contains(err.Error(), "partial-password") {
		t.Fatalf("partial bootstrap error leaked password: %v", err)
	}
}

func TestNewIgnoresBootstrapEnvironmentAfterCompletion(t *testing.T) {
	t.Parallel()

	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = filepath.Join(t.TempDir(), "app.db")
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	first, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Identity.BootstrapFirstOwner(context.Background(), identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{}); err != nil {
		first.Close()
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.Bootstrap.AdminEmail = "stale@example.com"
	cfg.Bootstrap.AdminPassword = "stale-password"
	second, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New() with stale bootstrap environment = %v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
}
