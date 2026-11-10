package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPToolContractMatchesPublishedSchemas proves that typed tool derivation
// keeps names, descriptions, annotations, and input/output schemas equal to the
// contract published before migration. Regenerate testdata only for an
// intentional, versioned contract change.
func TestMCPToolContractMatchesPublishedSchemas(t *testing.T) {
	state := newTestState(t, config.MCP{})
	session := connectSession(t, state)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var got, want any
	encoded, err := json.Marshal(listed.Tools)
	if err != nil || json.Unmarshal(encoded, &got) != nil {
		t.Fatalf("encode listed tools: %v", err)
	}
	golden, err := os.ReadFile("testdata/tools-v1.json")
	if err != nil || json.Unmarshal(golden, &want) != nil {
		t.Fatalf("read published contract: %v", err)
	}
	if !reflect.DeepEqual(canonicalSchema(got), canonicalSchema(want)) {
		t.Fatalf("tools/list drifted from testdata/tools-v1.json\ngot:  %s\nwant: %s", encoded, golden)
	}
}

// canonicalSchema sorts "type" arrays, which JSON Schema treats as sets, so
// ["null","string"] and ["string","null"] compare equal. Object keys already
// compare as maps and every other array keeps its order.
func canonicalSchema(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "type" {
				if types, ok := child.([]any); ok {
					sort.Slice(types, func(i, j int) bool { return types[i].(string) < types[j].(string) })
				}
			}
			canonicalSchema(child)
		}
	case []any:
		for _, child := range typed {
			canonicalSchema(child)
		}
	}
	return value
}

// TestMCPListAppliesPublishedDefaults proves the SDK fills the published limit
// and sort defaults before the handler runs, so omitted arguments list 50 items
// sorted by newest first.
func TestMCPListAppliesPublishedDefaults(t *testing.T) {
	state := newTestState(t, config.MCP{})
	session := connectSession(t, state)
	for index := range 51 {
		callTool(t, session, "items_create_v1", map[string]any{
			"workspaceId": state.workspaceID, "title": "default page", "idempotencyKey": fmt.Sprintf("default-%d", index),
		})
	}
	page := decodeOutput[items.Page](t, callTool(t, session, "items_list_v1", map[string]any{
		"workspaceId": state.workspaceID,
	}))
	if len(page.Items) != 50 || page.NextCursor == "" {
		t.Fatalf("default page = %d items, cursor %q", len(page.Items), page.NextCursor)
	}
}

// TestMCPRejectedArgumentsAreAuditedAsInvalidParameters pins that arguments the
// published schema rejects still return the safe invalid-parameters error and
// write one failed audit record with no workspace or item attribution, because
// the caller's workspace is unauthorized input at that point.
func TestMCPRejectedArgumentsAreAuditedAsInvalidParameters(t *testing.T) {
	state := newTestState(t, config.MCP{})
	session := connectSession(t, state)
	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"limit above maximum", "items_list_v1", map[string]any{"workspaceId": state.workspaceID, "limit": 101}},
		{"limit below minimum", "items_list_v1", map[string]any{"workspaceId": state.workspaceID, "limit": 0}},
		{"unknown field", "items_create_v1", map[string]any{
			"workspaceId": state.workspaceID, "title": "x", "idempotencyKey": "k", "unknown": true,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := callTool(t, session, tc.tool, tc.args)
			if !result.IsError || contentText(result) != "parameters are invalid" {
				t.Fatalf("result = %+v", result)
			}
			var action, outcome, workspaceID, metadata string
			err := state.db.QueryRow(`SELECT action, outcome, COALESCE(workspace_id, ''), metadata
				FROM audit_events WHERE event_type = 'mcp.tool.called' ORDER BY rowid DESC LIMIT 1`).
				Scan(&action, &outcome, &workspaceID, &metadata)
			if err != nil {
				t.Fatal(err)
			}
			if action != tc.tool || outcome != "failure" || workspaceID != "" ||
				!strings.Contains(metadata, `"error_category":"invalid_parameters"`) {
				t.Fatalf("audit = action %q outcome %q workspace %q metadata %s", action, outcome, workspaceID, metadata)
			}
		})
	}
}

func connectSession(t *testing.T, state *testState) *mcp.ClientSession {
	t.Helper()
	server := httptest.NewServer(state.handler)
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "contract-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: &authTransport{token: state.token.Secret}},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
