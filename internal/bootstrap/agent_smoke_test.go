package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"example.com/dynamis-code/apps-template/internal/appctl"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type smokeTransport struct {
	token string
}

func (transport smokeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header.Set("Authorization", "Bearer "+transport.token)
	clone.Header.Set("X-Request-ID", "0123456789abcdef0123456789abcdef")
	return http.DefaultTransport.RoundTrip(clone)
}

func TestAgentSurfacesLiveSmoke(t *testing.T) {
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns = 1, 1
	app, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	owner, err := app.Identity.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "smoke@example.com", Password: "long-enough-password", WorkspaceName: "Smoke",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := app.Identity.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	token, err := app.Identity.CreateAPIToken(ctx, actor, "smoke",
		[]identity.Permission{identity.ResourcesRead, identity.ResourcesWrite}, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(app.Handler)
	t.Cleanup(server.Close)

	endpoint := server.URL + "/api/v1/workspaces/" + owner.WorkspaceID + "/items"
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBufferString(`{"title":"smoke item"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token.Secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "smoke-key")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var created items.Item
	if response.StatusCode != http.StatusCreated || json.NewDecoder(response.Body).Decode(&created) != nil {
		t.Fatalf("REST create status = %d", response.StatusCode)
	}

	var stdout, stderr bytes.Buffer
	exit := appctl.Run([]string{"items", "get", "--workspace", owner.WorkspaceID, "--item", created.ID},
		func(key string) string {
			if key == "APPCTL_BASE_URL" {
				return server.URL
			}
			if key == "APPCTL_TOKEN" {
				return token.Secret
			}
			return ""
		}, &stdout, &stderr)
	if exit != 0 || stderr.Len() != 0 || !json.Valid(stdout.Bytes()) {
		t.Fatalf("CLI exit=%d stdout=%q stderr=%q", exit, stdout.String(), stderr.String())
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "live-smoke", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:             server.URL + "/mcp",
		HTTPClient:           &http.Client{Transport: smokeTransport{token: token.Secret}},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("MCP connect error = %v", err)
	}
	t.Cleanup(func() { session.Close() })
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "items_get_v1", Arguments: map[string]any{
			"workspaceId": owner.WorkspaceID, "itemId": created.ID,
		},
	})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("MCP get result=%+v error=%v", result, err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !bytes.Contains([]byte(text.Text), []byte(created.ID)) {
		t.Fatalf("MCP content = %#v", result.Content)
	}
}
