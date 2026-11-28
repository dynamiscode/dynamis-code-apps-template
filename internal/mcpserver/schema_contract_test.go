package mcpserver

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// TestMCPToolContractMatchesPublishedSchemas proves that typed tool derivation
// keeps names, descriptions, annotations, and input/output schemas equal to the
// contract published before migration. Regenerate testdata only for an
// intentional, versioned contract change.
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
	state := newTestStateWith(t, config.MCP{}, true)
	session := connectSession(t, state)
	for index := range 51 {
		createClientTool(t, session, state.workspaceID, fmt.Sprintf("default-%d", index))
	}
	page := decodeOutput[clientPageOutput](t, callTool(t, session, "clients_list_v1", map[string]any{
		"workspaceId": state.workspaceID,
	}))
	if len(page.Clients) != 50 || page.Total != 51 || page.Page != 1 {
		t.Fatalf("default page = %d items, total %d, page %d", len(page.Clients), page.Total, page.Page)
	}
}

// TestMCPRejectedArgumentsAreAuditedAsInvalidParameters pins that arguments the
// published schema rejects still return the safe invalid-parameters error and
// write one failed audit record with no workspace or item attribution, because
// the caller's workspace is unauthorized input at that point.
func TestMCPRejectedArgumentsAreAuditedAsInvalidParameters(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	session := connectSession(t, state)
	cases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"limit above maximum", "clients_list_v1", map[string]any{"workspaceId": state.workspaceID, "limit": 101}},
		{"limit below minimum", "clients_list_v1", map[string]any{"workspaceId": state.workspaceID, "limit": 0}},
		{"unknown field", "clients_create_v1", map[string]any{
			"workspaceId": state.workspaceID, "displayName": "x", "unknown": true,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := callTool(t, session, tc.tool, tc.args)
			if !result.IsError || contentText(result) != "parameters are invalid" {
				t.Fatalf("result = %+v", result)
			}
			var action, outcome, workspaceID, metadata string
			err := state.db.QueryRow(`SELECT action, outcome, COALESCE(CAST(workspace_id AS TEXT), ''), metadata
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
