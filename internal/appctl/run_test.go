package appctl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/bootstrap"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestRunManipulatesItemsThroughREST(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns = 1, 1
	app, err := bootstrap.New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	owner, err := app.Identity.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "CLI test",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := app.Identity.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.Identity.CreateAPIToken(ctx, actor, "CLI test",
		[]identity.Permission{identity.ResourcesRead, identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler)
	t.Cleanup(server.Close)
	env := func(key string) string {
		switch key {
		case "APPCTL_BASE_URL":
			return server.URL
		case "APPCTL_TOKEN":
			return token.Secret
		default:
			return ""
		}
	}

	createdRaw := runOK(t, env, "items", "create", "--workspace", owner.WorkspaceID,
		"--title", "CLI item", "--idempotency-key", "cli-key-1")
	var created items.Item
	if err := json.Unmarshal([]byte(createdRaw), &created); err != nil || created.Title != "CLI item" {
		t.Fatalf("created = %s, error = %v", createdRaw, err)
	}
	listedRaw := runOK(t, env, "items", "list", "--workspace", owner.WorkspaceID, "--limit", "1")
	var page items.Page
	if err := json.Unmarshal([]byte(listedRaw), &page); err != nil || len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("listed = %s, error = %v", listedRaw, err)
	}
	gotRaw := runOK(t, env, "items", "get", "--workspace", owner.WorkspaceID, "--item", created.ID)
	var got items.Item
	if err := json.Unmarshal([]byte(gotRaw), &got); err != nil || got.ID != created.ID {
		t.Fatalf("got = %s, error = %v", gotRaw, err)
	}
	updatedRaw := runOK(t, env, "items", "update", "--workspace", owner.WorkspaceID,
		"--item", created.ID, "--version", "1", "--set-status", "complete")
	var updated items.Item
	if err := json.Unmarshal([]byte(updatedRaw), &updated); err != nil || updated.Status != items.Complete {
		t.Fatalf("updated = %s, error = %v", updatedRaw, err)
	}
	deletedRaw := runOK(t, env, "items", "delete", "--workspace", owner.WorkspaceID,
		"--item", created.ID, "--version", "2")
	if strings.TrimSpace(deletedRaw) != `{"deleted":true}` {
		t.Fatalf("deleted = %s", deletedRaw)
	}

	readToken, err := app.Identity.CreateAPIToken(ctx, actor, "read only",
		[]identity.Permission{identity.ResourcesRead}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	readEnv := func(key string) string {
		if key == "APPCTL_TOKEN" {
			return readToken.Secret
		}
		return env(key)
	}
	var stdout, stderr bytes.Buffer
	exit := Run([]string{"items", "create", "--workspace", owner.WorkspaceID,
		"--title", "blocked", "--idempotency-key", "cli-key-2"}, readEnv, &stdout, &stderr)
	if exit != 3 || stdout.Len() != 0 || !json.Valid(stderr.Bytes()) || strings.Contains(stderr.String(), readToken.Secret) {
		t.Fatalf("scoped failure exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}
}

func TestRunRejectsUnsafeConfigurationAndRedirects(t *testing.T) {
	secret := "do-not-print-this-token"
	var stdout, stderr bytes.Buffer
	exit := Run([]string{"items", "list", "--workspace", "0123456789abcdef0123456789abcdef",
		"--item", "0123456789abcdef0123456789abcdef"},
		func(key string) string {
			if key == "APPCTL_TOKEN" {
				return secret
			}
			return ""
		}, &stdout, &stderr)
	if exit != 2 || !strings.Contains(stderr.String(), "not valid for this command") {
		t.Fatalf("irrelevant flag exit=%d stderr=%q", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	exit = Run([]string{"items", "list", "--workspace", "0123456789abcdef0123456789abcdef",
		"--base-url", "http://user:" + secret + "@example.com"},
		func(key string) string {
			if key == "APPCTL_TOKEN" {
				return secret
			}
			return ""
		}, &stdout, &stderr)
	if exit != 2 || strings.Contains(stderr.String(), secret) {
		t.Fatalf("unsafe configuration exit=%d stderr=%q", exit, stderr.String())
	}

	followed := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed = true }))
	t.Cleanup(target.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	t.Cleanup(redirect.Close)
	stdout.Reset()
	stderr.Reset()
	exit = Run([]string{"items", "list", "--workspace", "0123456789abcdef0123456789abcdef"},
		func(key string) string {
			if key == "APPCTL_TOKEN" {
				return secret
			}
			if key == "APPCTL_BASE_URL" {
				return redirect.URL
			}
			return ""
		}, &stdout, &stderr)
	if exit != 1 || followed || strings.Contains(stderr.String(), secret) {
		t.Fatalf("redirect exit=%d followed=%v stderr=%q", exit, followed, stderr.String())
	}
}

func runOK(t *testing.T, env func(string) string, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if exit := Run(args, env, &stdout, &stderr); exit != 0 {
		t.Fatalf("Run(%v) exit=%d stderr=%s", args, exit, stderr.String())
	}
	if stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
		t.Fatalf("Run(%v) stdout=%q stderr=%q", args, stdout.String(), stderr.String())
	}
	return stdout.String()
}
