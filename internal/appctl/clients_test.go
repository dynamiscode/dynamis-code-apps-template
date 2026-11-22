package appctl

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/bootstrap"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestRunManagesClientsAndFieldsThroughREST(t *testing.T) {
	workspace, env := newClientsCLI(t)

	var field cliField
	decodeCLI(t, runOK(t, env, "fields", "define", "--workspace", workspace,
		"--label", "Source", "--type", "text"), &field)
	if field.Label != "Source" || field.Type != "text" || field.ArchivedAt != nil {
		t.Fatalf("defined field = %+v", field)
	}

	var created cliClient
	decodeCLI(t, runOK(t, env, "clients", "create", "--workspace", workspace,
		"--display-name", "Acme", "--email", "ops@example.com"), &created)
	if created.DisplayName != "Acme" || created.Email != "ops@example.com" || created.Version != 1 {
		t.Fatalf("created client = %+v", created)
	}
	clientArg := created.ID

	var page cliClientPage
	decodeCLI(t, runOK(t, env, "clients", "list", "--workspace", workspace, "--limit", "1",
		"--filter", "displayName:contains:Acme", "--filter", "email:contains:example.com",
		"--sort", "displayName"), &page)
	if len(page.Clients) != 1 || page.Clients[0].ID != created.ID {
		t.Fatalf("listed clients = %+v", page.Clients)
	}

	var shown cliClient
	decodeCLI(t, runOK(t, env, "clients", "show", "--workspace", workspace, "--client", clientArg), &shown)
	if shown.ID != created.ID {
		t.Fatalf("shown client = %+v", shown)
	}

	var edited cliClient
	decodeCLI(t, runOK(t, env, "clients", "edit", "--workspace", workspace, "--client", clientArg,
		"--version", "1", "--set-display-name", "Acme Corp", "--set-email", ""), &edited)
	if edited.DisplayName != "Acme Corp" || edited.Email != "" || edited.Version != 2 {
		t.Fatalf("edited client = %+v", edited)
	}

	var archived cliClient
	decodeCLI(t, runOK(t, env, "clients", "archive", "--workspace", workspace, "--client", clientArg), &archived)
	if archived.ArchivedAt == nil {
		t.Fatalf("archived client has no archivedAt: %+v", archived)
	}
	var archivedPage cliClientPage
	decodeCLI(t, runOK(t, env, "clients", "list", "--workspace", workspace, "--archived", "archived"), &archivedPage)
	if len(archivedPage.Clients) != 1 || archivedPage.Clients[0].ID != created.ID {
		t.Fatalf("archived list = %+v", archivedPage.Clients)
	}

	var restored cliClient
	decodeCLI(t, runOK(t, env, "clients", "unarchive", "--workspace", workspace, "--client", clientArg), &restored)
	if restored.ArchivedAt != nil {
		t.Fatalf("unarchived client still archived: %+v", restored)
	}

	var archivedField cliField
	decodeCLI(t, runOK(t, env, "fields", "archive", "--workspace", workspace, "--field", field.ID), &archivedField)
	if archivedField.ArchivedAt == nil {
		t.Fatalf("archived field has no archivedAt: %+v", archivedField)
	}
}

func TestRunRejectsInvalidClientAndFieldCommandsBeforeRequests(t *testing.T) {
	workspace, _ := newClientsCLI(t)
	validClient := "23456789abcd"
	env := func(key string) string {
		if key == "APPCTL_TOKEN" {
			return "token-not-sent"
		}
		return ""
	}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"show bad client", []string{"clients", "show", "--workspace", workspace, "--client", "!!!!"}, "--client must be a client short ID"},
		{"show missing client", []string{"clients", "show", "--workspace", workspace}, "--client must be a client short ID"},
		{"create without name", []string{"clients", "create", "--workspace", workspace}, "create requires --display-name"},
		{"create blank name", []string{"clients", "create", "--workspace", workspace, "--display-name", "   "}, "create requires --display-name"},
		{"edit without changes", []string{"clients", "edit", "--workspace", workspace, "--client", validClient, "--version", "1"}, "edit requires --client, --version, and at least one --set-"},
		{"edit without version", []string{"clients", "edit", "--workspace", workspace, "--client", validClient, "--set-display-name", "x"}, "edit requires --client, --version, and at least one --set-"},
		{"list zero limit", []string{"clients", "list", "--workspace", workspace, "--limit", "0"}, "list flags are invalid"},
		{"list unknown archived", []string{"clients", "list", "--workspace", workspace, "--archived", "sometimes"}, "list flags are invalid"},
		{"define unknown type", []string{"fields", "define", "--workspace", workspace, "--label", "Tier", "--type", "bogus"}, "--type must be text, number, date, yes_no, or single_choice"},
		{"define without label", []string{"fields", "define", "--workspace", workspace, "--type", "text"}, "define requires --label and --type"},
		{"archive missing field", []string{"fields", "archive", "--workspace", workspace}, "--field must be a client field short ID"},
		{"item flag on clients", []string{"clients", "show", "--workspace", workspace, "--client", validClient, "--item", validClient}, "flags are invalid"},
		{"unknown clients action", []string{"clients", "purge", "--workspace", workspace}, "unknown clients command"},
		{"unknown fields action", []string{"fields", "restore", "--workspace", workspace, "--field", validClient}, "unknown fields command"},
		{"bad workspace", []string{"clients", "list", "--workspace", "no good"}, "--workspace must be a workspace short ID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exit := Run(tc.args, env, &stdout, &stderr); exit != 2 {
				t.Fatalf("Run(%v) exit = %d, want 2; stderr = %s", tc.args, exit, stderr.String())
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stdout = %q, stderr = %q, want detail %q", stdout.String(), stderr.String(), tc.want)
			}
		})
	}
}

// newClientsCLI signs in a workspace owner with the scopes a client needs for
// client and field commands, and serves the application over httptest.
func newClientsCLI(t *testing.T) (string, func(string) string) {
	t.Helper()
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
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Clients CLI test",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := app.Identity.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.Identity.CreateAPIToken(ctx, actor, "Clients CLI test",
		[]identity.Permission{identity.WorkspaceRead, identity.WorkspaceUpdate, identity.ResourcesRead, identity.ResourcesWrite},
		nil, identity.AuditContext{})
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
	return owner.WorkspaceID.String(), env
}

// The types below mirror the REST JSON the tests read. They are local so the CLI
// tests do not import the Clients service, which a generated application
// without the Clients profile removes.
type cliClient struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	Email       string  `json:"email"`
	Version     int64   `json:"version"`
	ArchivedAt  *string `json:"archivedAt"`
}

type cliClientPage struct {
	Clients []cliClient `json:"clients"`
}

type cliField struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Type       string  `json:"type"`
	ArchivedAt *string `json:"archivedAt"`
}

func decodeCLI(t *testing.T, raw string, value any) {
	t.Helper()
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), value); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
}
