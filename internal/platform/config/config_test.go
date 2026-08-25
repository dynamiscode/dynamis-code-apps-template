package config

import (
	"strings"
	"testing"
)

func TestLoadFromDefaultsToSQLite(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFrom(env(nil))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	if cfg.Database.Driver != SQLite {
		t.Fatalf("Driver = %q, want %q", cfg.Database.Driver, SQLite)
	}
	if cfg.Database.SQLitePath != "data/app.db" {
		t.Fatalf("SQLitePath = %q, want data/app.db", cfg.Database.SQLitePath)
	}
	if cfg.Database.MaxOpenConns != 4 || cfg.Database.MaxIdleConns != 2 {
		t.Fatalf(
			"pool = %d/%d, want 4/2",
			cfg.Database.MaxOpenConns,
			cfg.Database.MaxIdleConns,
		)
	}
}

func TestLoadFromValidatesBootstrapEnvironment(t *testing.T) {
	t.Parallel()

	password := "owner-password"
	cfg, err := LoadFrom(env(map[string]string{
		"BOOTSTRAP_ADMIN_EMAIL": "owner@example.com", "BOOTSTRAP_ADMIN_WORKSPACE": "Example",
		"BOOTSTRAP_ADMIN_PASSWORD": password, "BOOTSTRAP_SETUP_TOKEN": "setup-token",
	}))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.Bootstrap.AdminEmail != "owner@example.com" || cfg.Bootstrap.AdminWorkspace != "Example" ||
		cfg.Bootstrap.AdminPassword != password || cfg.Bootstrap.SetupToken != "setup-token" {
		t.Fatalf("Bootstrap = %+v", cfg.Bootstrap)
	}
	for _, values := range []map[string]string{
		{"BOOTSTRAP_ADMIN_EMAIL": "owner@example.com"},
		{"BOOTSTRAP_ADMIN_WORKSPACE": "Example"},
		{"BOOTSTRAP_ADMIN_PASSWORD": password},
	} {
		if _, err := LoadFrom(env(values)); err != nil {
			t.Fatalf("partial bootstrap configuration rejected before database state: %v", err)
		}
	}
	if _, err := LoadFrom(env(map[string]string{
		"BOOTSTRAP_ADMIN_EMAIL": "", "BOOTSTRAP_ADMIN_WORKSPACE": "", "BOOTSTRAP_ADMIN_PASSWORD": "",
	})); err != nil {
		t.Fatalf("empty bootstrap configuration rejected: %v", err)
	}
}

func TestLoadFromRequiresPostgresURLWithoutLeakingValue(t *testing.T) {
	t.Parallel()

	secret := "postgres://user:secret@example.invalid/app"
	_, err := LoadFrom(env(map[string]string{
		"DATABASE_DRIVER": "postgres",
		"DATABASE_URL":    secret,
		"SQLITE_PATH":     "ignored.db",
	}))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}

	_, err = LoadFrom(env(map[string]string{
		"DATABASE_DRIVER": "postgres",
		"DATABASE_URL":    "",
	}))
	if err == nil {
		t.Fatal("LoadFrom() error = nil, want validation error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked DATABASE_URL: %v", err)
	}
}

func TestLoadFromRejectsInvalidPool(t *testing.T) {
	t.Parallel()

	_, err := LoadFrom(env(map[string]string{
		"DATABASE_MAX_OPEN_CONNS": "1",
		"DATABASE_MAX_IDLE_CONNS": "2",
	}))
	if err == nil {
		t.Fatal("LoadFrom() error = nil, want validation error")
	}
}

func TestLoadFromLeavesOIDCDisabledWithoutConfiguration(t *testing.T) {
	t.Parallel()

	cfg, err := LoadFrom(env(map[string]string{"OIDC_ENABLED": "false"}))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.OIDC.Enabled {
		t.Fatal("OIDC.Enabled = true, want false")
	}
}

func TestLoadFromValidatesEnabledOIDCWithoutLeakingSecret(t *testing.T) {
	t.Parallel()

	secret := "oidc-client-secret"
	cfg, err := LoadFrom(env(map[string]string{
		"OIDC_ENABLED":       "true",
		"OIDC_PROVIDER_ID":   "company",
		"OIDC_PROVIDER_NAME": "Company login",
		"OIDC_ISSUER_URL":    "https://id.example.com",
		"OIDC_CLIENT_ID":     "apps-template",
		"OIDC_CLIENT_SECRET": secret,
		"OIDC_REDIRECT_URL":  "https://app.example.com/auth/oidc/company/callback",
	}))
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if !cfg.OIDC.Enabled || cfg.OIDC.ProviderID != "company" {
		t.Fatalf("OIDC = %+v, want enabled company provider", cfg.OIDC)
	}

	_, err = LoadFrom(env(map[string]string{
		"OIDC_ENABLED":       "true",
		"OIDC_PROVIDER_ID":   "company",
		"OIDC_PROVIDER_NAME": "Company login",
		"OIDC_ISSUER_URL":    "http://169.254.169.254",
		"OIDC_CLIENT_ID":     "apps-template",
		"OIDC_CLIENT_SECRET": secret,
		"OIDC_REDIRECT_URL":  "https://app.example.com/callback",
	}))
	if err == nil {
		t.Fatal("LoadFrom() error = nil, want unsafe issuer error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("OIDC validation leaked secret: %v", err)
	}
}

func TestLoadFromRejectsInvalidHTTPLimits(t *testing.T) {
	t.Parallel()

	for key, value := range map[string]string{
		"HTTP_REQUEST_TIMEOUT":          "0s",
		"HTTP_MAX_BODY_BYTES":           "100",
		"HTTP_MAX_PAGE_SIZE":            "101",
		"HTTP_DEFAULT_PAGE_SIZE":        "101",
		"HTTP_AUTH_REQUESTS_PER_MINUTE": "121",
	} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			_, err := LoadFrom(env(map[string]string{key: value}))
			if err == nil {
				t.Fatalf("LoadFrom(%s=%s) error = nil", key, value)
			}
		})
	}
}

func TestLoadFromValidatesOptionalSMTP(t *testing.T) {
	t.Parallel()

	secret := "smtp-secret"
	cfg, err := LoadFrom(env(map[string]string{
		"APP_PUBLIC_URL": "https://app.example.com/",
		"SMTP_HOST":      "smtp.example.com", "SMTP_FROM": "no-reply@example.com",
		"SMTP_USERNAME": "mailer", "SMTP_PASSWORD": secret,
	}))
	if err != nil {
		t.Fatalf("valid SMTP configuration rejected: %v", err)
	}
	if cfg.PublicURL != "https://app.example.com" || cfg.Mail.Port != 587 || cfg.Mail.Host != "smtp.example.com" {
		t.Fatalf("SMTP config = %+v, public URL = %q", cfg.Mail, cfg.PublicURL)
	}
	for _, values := range []map[string]string{
		{"SMTP_HOST": "smtp.example.com"},
		{"SMTP_HOST": "smtp.example.com", "SMTP_FROM": "no-reply@example.com"},
		{"SMTP_HOST": "smtp.example.com", "APP_PUBLIC_URL": "https://app.example.com", "SMTP_FROM": "no-reply@example.com", "SMTP_USERNAME": "mailer"},
		{"APP_PUBLIC_URL": "http://169.254.169.254"},
	} {
		if _, err := LoadFrom(env(values)); err == nil {
			t.Fatalf("invalid SMTP configuration accepted: %#v", values)
		} else if strings.Contains(err.Error(), secret) {
			t.Fatalf("SMTP validation leaked secret: %v", err)
		}
	}
}
func env(values map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
