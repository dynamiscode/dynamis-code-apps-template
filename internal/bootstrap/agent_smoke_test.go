package bootstrap

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

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

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	loginPage, err := browser.Get(server.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	loginPage.Body.Close()
	var loginCSRF string
	loginURL, _ := url.Parse(server.URL + "/login")
	for _, cookie := range jar.Cookies(loginURL) {
		if cookie.Name == "login_csrf" {
			loginCSRF = cookie.Value
		}
	}
	loginResponse, err := browser.PostForm(server.URL+"/login", url.Values{
		"email": {"smoke@example.com"}, "password": {"long-enough-password"},
		"csrf": {loginCSRF},
	})
	if err != nil {
		t.Fatal(err)
	}
	loginResponse.Body.Close()
	if loginResponse.StatusCode != http.StatusOK || !strings.HasSuffix(loginResponse.Request.URL.Path, "/") {
		t.Fatalf("browser login status=%d path=%s", loginResponse.StatusCode, loginResponse.Request.URL.Path)
	}
	itemsPage, err := browser.Get(server.URL + "/workspaces/" + owner.WorkspaceID.String() + "/items")
	if err != nil {
		t.Fatal(err)
	}
	itemsPage.Body.Close()
	if itemsPage.StatusCode != http.StatusOK {
		t.Fatalf("browser items status = %d", itemsPage.StatusCode)
	}

	endpoint := server.URL + "/api/v1/workspaces/" + owner.WorkspaceID.String() + "/items"
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
	exit := appctl.Run([]string{"items", "get", "--workspace", owner.WorkspaceID.String(), "--item", created.ID.String()},
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
			"workspaceId": owner.WorkspaceID.String(), "itemId": created.ID.String(),
		},
	})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("MCP get result=%+v error=%v", result, err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !bytes.Contains([]byte(text.Text), []byte(created.ID.String())) {
		t.Fatalf("MCP content = %#v", result.Content)
	}

	streamContext, cancelStream := context.WithTimeout(ctx, 5*time.Second)
	defer cancelStream()
	streamRequest, err := http.NewRequestWithContext(streamContext, http.MethodGet,
		server.URL+"/workspaces/"+owner.WorkspaceID.String()+"/items/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	streamResponse, err := browser.Do(streamRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer streamResponse.Body.Close()
	if streamResponse.StatusCode != http.StatusOK {
		t.Fatalf("SSE status = %d", streamResponse.StatusCode)
	}
	ready := make(chan struct{})
	events := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(streamResponse.Body)
		readySent := false
		resync := false
		for scanner.Scan() {
			line := scanner.Text()
			if line == "event: resync" && !readySent {
				resync = true
			}
			if strings.HasPrefix(line, "data: ") {
				if resync {
					close(ready)
					readySent = true
					resync = false
					continue
				}
				events <- line
				return
			}
		}
	}()
	select {
	case <-ready:
	case <-streamContext.Done():
		t.Fatal("SSE initial resync timed out")
	}
	secondRequest, err := http.NewRequest(http.MethodPost, endpoint,
		bytes.NewBufferString(`{"title":"SSE smoke item"}`))
	if err != nil {
		t.Fatal(err)
	}
	secondRequest.Header.Set("Authorization", "Bearer "+token.Secret)
	secondRequest.Header.Set("Content-Type", "application/json")
	secondRequest.Header.Set("Idempotency-Key", "sse-smoke-key")
	secondResponse, err := http.DefaultClient.Do(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer secondResponse.Body.Close()
	var second items.Item
	if secondResponse.StatusCode != http.StatusCreated || json.NewDecoder(secondResponse.Body).Decode(&second) != nil {
		t.Fatalf("second REST create status = %d", secondResponse.StatusCode)
	}
	select {
	case event := <-events:
		if !strings.Contains(event, second.ID.String()) {
			t.Fatalf("SSE event = %s, want item %s", event, second.ID.String())
		}
	case <-streamContext.Done():
		t.Fatal("SSE item notification timed out")
	}
}

func TestAgentOAuthMetadataRouteIsOptIn(t *testing.T) {
	ctx := context.Background()
	const (
		metadataPath = "/.well-known/oauth-protected-resource/mcp"
		challenge    = `Bearer resource_metadata="https://app.example.com/.well-known/oauth-protected-resource/mcp"`
	)
	newApp := func(t *testing.T, oauth bool) *App {
		t.Helper()
		cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
		if err != nil {
			t.Fatal(err)
		}
		cfg.Database.SQLitePath = ":memory:"
		cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns = 1, 1
		if oauth {
			cfg.PublicURL = "https://app.example.com"
			cfg.MCP.OAuthAuthorizationServer = "https://idp.example.com/tenant"
			cfg.MCP.OAuthResourceURL = "https://app.example.com/mcp"
		}
		app, err := New(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { app.Close() })
		return app
	}

	newBuiltInApp := func(t *testing.T) *App {
		t.Helper()
		cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
		if err != nil {
			t.Fatal(err)
		}
		cfg.Database.SQLitePath = ":memory:"
		cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns = 1, 1
		cfg.PublicURL = "https://app.example.com"
		cfg.MCP.OAuthBuiltIn = true
		cfg.MCP.OAuthAuthorizationServer = "https://app.example.com"
		cfg.MCP.OAuthResourceURL = "https://app.example.com/mcp"
		app, err := New(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { app.Close() })
		return app
	}

	enabled := newApp(t, true)
	metadata := httptest.NewRecorder()
	enabled.Handler.ServeHTTP(metadata, httptest.NewRequest(http.MethodGet, metadataPath, nil))
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"resource":"https://app.example.com/mcp"`) {
		t.Fatalf("enabled metadata = %d %s", metadata.Code, metadata.Body.String())
	}
	challengeResponse := httptest.NewRecorder()
	enabled.Handler.ServeHTTP(challengeResponse, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`)))
	if challengeResponse.Code != http.StatusUnauthorized || challengeResponse.Header().Get("WWW-Authenticate") != challenge {
		t.Fatalf("enabled /mcp challenge = %d WWW-Authenticate=%q", challengeResponse.Code, challengeResponse.Header().Get("WWW-Authenticate"))
	}

	builtIn := newBuiltInApp(t)
	authorize := httptest.NewRecorder()
	builtIn.Handler.ServeHTTP(authorize, httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code", nil))
	if authorize.Code != http.StatusBadRequest {
		t.Fatalf("built-in consent without parameters = %d, want 400", authorize.Code)
	}
	serverMetadata := httptest.NewRecorder()
	builtIn.Handler.ServeHTTP(serverMetadata, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	if serverMetadata.Code != http.StatusOK || !strings.Contains(serverMetadata.Body.String(), `"registration_endpoint":"https://app.example.com/oauth/register"`) {
		t.Fatalf("built-in AS metadata = %d %s", serverMetadata.Code, serverMetadata.Body.String())
	}

	disabled := newApp(t, false)
	absentAuthorize := httptest.NewRecorder()
	disabled.Handler.ServeHTTP(absentAuthorize, httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil))
	if absentAuthorize.Code != http.StatusNotFound {
		t.Fatalf("disabled consent route = %d, want 404", absentAuthorize.Code)
	}
	absent := httptest.NewRecorder()
	disabled.Handler.ServeHTTP(absent, httptest.NewRequest(http.MethodGet, metadataPath, nil))
	if absent.Code != http.StatusNotFound {
		t.Fatalf("disabled metadata = %d, want 404", absent.Code)
	}
	plain := httptest.NewRecorder()
	disabled.Handler.ServeHTTP(plain, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`)))
	if plain.Code != http.StatusUnauthorized || plain.Header().Get("WWW-Authenticate") != "" {
		t.Fatalf("disabled /mcp challenge = %d WWW-Authenticate=%q", plain.Code, plain.Header().Get("WWW-Authenticate"))
	}
}
