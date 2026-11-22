package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// clientToolNames lists every client and custom-field tool, in name order.
var clientToolNames = []string{
	"client_field_options_add_v1", "client_field_options_delete_v1", "client_field_options_list_v1",
	"client_fields_archive_v1", "client_fields_define_v1", "client_fields_delete_v1",
	"client_fields_list_v1", "client_fields_restore_v1",
	"clients_archive_v1", "clients_create_v1", "clients_delete_v1", "clients_get_v1", "clients_list_v1",
	"clients_timeline_add_v1", "clients_timeline_list_v1", "clients_unarchive_v1", "clients_update_v1", "clients_value_clear_v1", "clients_value_set_v1",
	"clients_values_list_v1",
}

// connectClient opens a session with a bearer token. confirm answers every
// human-approval prompt, so destructive calls either run or are declined.
func connectClient(t *testing.T, handler http.Handler, secret string, confirm bool) *mcp.ClientSession {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "clients-test", Version: "1.0.0"}, &mcp.ClientOptions{
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if confirm {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirm": true}}, nil
			}
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, HTTPClient: &http.Client{Transport: &authTransport{token: secret}},
		DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// tokenWithScopes issues a token for a new user with role in the test workspace.
// Scopes cannot exceed the role's permissions, so a viewer can only hold read.
func tokenWithScopes(t *testing.T, state *testState, role identity.Role, scopes ...identity.Permission) string {
	t.Helper()
	ctx := context.Background()
	var userID identity.UserID
	email := string(role) + "-" + strings.Join(scopesText(scopes), "-") + "@example.com"
	if err := state.db.QueryRowContext(ctx, "INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id",
		email, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := state.auth.AddMember(ctx, state.actor, userID, role, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	principal, err := state.auth.Authorize(ctx, userID, state.workspaceKey, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	token, err := state.auth.CreateAPIToken(ctx, principal, email, scopes, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return token.Secret
}

func scopesText(scopes []identity.Permission) []string {
	text := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		text = append(text, string(scope))
	}
	return text
}

func createClientTool(t *testing.T, session *mcp.ClientSession, workspaceID, name string) clientOutput {
	t.Helper()
	result := callTool(t, session, "clients_create_v1", map[string]any{
		"workspaceId": workspaceID, "displayName": name, "email": "ops@acme.example",
		"phone": "+1 555 0100", "notes": "Renewal in Q3",
	})
	if result.IsError {
		t.Fatalf("clients_create_v1 = %s", contentText(result))
	}
	return decodeOutput[clientOutput](t, result)
}

func defineFieldTool(t *testing.T, session *mcp.ClientSession, workspaceID, label, fieldType string) fieldOutput {
	t.Helper()
	result := callTool(t, session, "client_fields_define_v1", map[string]any{
		"workspaceId": workspaceID, "label": label, "type": fieldType,
	})
	if result.IsError {
		t.Fatalf("client_fields_define_v1 = %s", contentText(result))
	}
	return decodeOutput[fieldOutput](t, result)
}

// TestMCPClientToolsAreRegisteredOnlyWhenClientsIsComposed proves the Clients
// surface is absent without the Clients service and present with it.
func TestMCPClientToolsAreRegisteredOnlyWhenClientsIsComposed(t *testing.T) {
	without := connectSession(t, newTestState(t, config.MCP{}))
	listed, err := without.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range listed.Tools {
		if strings.HasPrefix(tool.Name, "client") {
			t.Fatalf("tool %s registered without the Clients service", tool.Name)
		}
	}
	if result, err := without.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "clients_list_v1", Arguments: map[string]any{"workspaceId": "x"},
	}); err == nil && (result == nil || !result.IsError) {
		t.Fatalf("clients_list_v1 without the Clients service = %+v", result)
	}

	state := newTestStateWith(t, config.MCP{}, true)
	listed, err = connectSession(t, state).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tool := range listed.Tools {
		if strings.HasPrefix(tool.Name, "client") {
			got = append(got, tool.Name)
		}
	}
	if !reflect.DeepEqual(got, clientToolNames) {
		t.Fatalf("client tools = %v, want %v", got, clientToolNames)
	}
}

// TestMCPClientToolContractMatchesPublishedSchemas pins the client tool catalog
// the way the Items contract is pinned. Regenerate testdata only for an intentional
// contract change.
func TestMCPClientToolContractMatchesPublishedSchemas(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	listed, err := connectSession(t, state).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var clientTools []*mcp.Tool
	for _, tool := range listed.Tools {
		if strings.HasPrefix(tool.Name, "client") {
			clientTools = append(clientTools, tool)
		}
	}
	encoded, err := json.Marshal(clientTools)
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("testdata/clients-tools-v1.json")
	if err != nil || json.Unmarshal(golden, &want) != nil {
		t.Fatalf("read published contract: %v", err)
	}
	if !reflect.DeepEqual(canonicalSchema(got), canonicalSchema(want)) {
		t.Fatalf("client tools drifted from testdata/clients-tools-v1.json\ngot:  %s", encoded)
	}
}

// TestMCPClientLifecycleUsesSharedService walks create, read, edit, archive,
// unarchive, and permanent delete, including stale-version and approval refusals.
func TestMCPClientLifecycleUsesSharedService(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	session := connectClient(t, state.handler, state.token.Secret, true)
	created := createClientTool(t, session, state.workspaceID, "Acme Ltd")
	if created.Version != 1 || created.ArchivedAt != nil || created.CreatedByUserID == nil {
		t.Fatalf("created = %+v", created)
	}

	got := decodeOutput[clientOutput](t, callTool(t, session, "clients_get_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(created.ID),
	}))
	if got.ID != created.ID || got.DisplayName != "Acme Ltd" {
		t.Fatalf("get = %+v", got)
	}

	updated := decodeOutput[clientOutput](t, callTool(t, session, "clients_update_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(created.ID), "version": 1,
		"displayName": "Acme", "phone": "",
	}))
	if updated.DisplayName != "Acme" || updated.Phone != "" || updated.Version != 2 {
		t.Fatalf("updated = %+v", updated)
	}
	stale := callTool(t, session, "clients_update_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(created.ID), "version": 1, "displayName": "Stale",
	})
	if !stale.IsError || contentText(stale) != "client version is stale" {
		t.Fatalf("stale update = %+v", stale)
	}

	archived := decodeOutput[clientOutput](t, callTool(t, session, "clients_archive_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(created.ID),
	}))
	if archived.ArchivedAt == nil {
		t.Fatalf("archived = %+v", archived)
	}
	active := decodeOutput[clientPageOutput](t, callTool(t, session, "clients_list_v1", map[string]any{
		"workspaceId": state.workspaceID,
	}))
	if len(active.Clients) != 0 {
		t.Fatalf("active list after archive = %+v", active.Clients)
	}
	onlyArchived := decodeOutput[clientPageOutput](t, callTool(t, session, "clients_list_v1", map[string]any{
		"workspaceId": state.workspaceID, "archived": "archived",
	}))
	if len(onlyArchived.Clients) != 1 || onlyArchived.Clients[0].ID != created.ID {
		t.Fatalf("archived list = %+v", onlyArchived.Clients)
	}

	unarchived := decodeOutput[clientOutput](t, callTool(t, session, "clients_unarchive_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(created.ID),
	}))
	if unarchived.ArchivedAt != nil {
		t.Fatalf("unarchived = %+v", unarchived)
	}

	declined := connectClient(t, state.handler, state.token.Secret, false)
	refused := callTool(t, declined, "clients_delete_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(created.ID),
	})
	if !refused.IsError || !strings.Contains(contentText(refused), "human approval") {
		t.Fatalf("delete without approval = %+v", refused)
	}
	deleted := decodeOutput[deletedOutput](t, callTool(t, session, "clients_delete_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(created.ID),
	}))
	if !deleted.Deleted {
		t.Fatalf("deleted = %+v", deleted)
	}
	missing := callTool(t, session, "clients_get_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(created.ID),
	})
	if !missing.IsError || contentText(missing) != "client not found" {
		t.Fatalf("get after delete = %+v", missing)
	}
}

// TestMCPCustomFieldsAndValuesUseSharedService covers field definitions, choice
// options, value set and clear, archive and restore, and the archived-first rule
// for permanent field delete.
func TestMCPCustomFieldsAndValuesUseSharedService(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	session := connectClient(t, state.handler, state.token.Secret, true)
	tier := defineFieldTool(t, session, state.workspaceID, "Tier", "text")
	segment := defineFieldTool(t, session, state.workspaceID, "Segment", "single_choice")
	option := decodeOutput[optionOutput](t, callTool(t, session, "client_field_options_add_v1", map[string]any{
		"workspaceId": state.workspaceID, "fieldId": string(segment.ID), "label": "Enterprise",
	}))
	if option.FieldID != segment.ID || option.Label != "Enterprise" {
		t.Fatalf("option = %+v", option)
	}
	fields := decodeOutput[fieldsOutput](t, callTool(t, session, "client_fields_list_v1", map[string]any{
		"workspaceId": state.workspaceID,
	}))
	if len(fields.Fields) != 2 || fields.Fields[0].ID != tier.ID {
		t.Fatalf("fields = %+v", fields.Fields)
	}
	options := decodeOutput[optionsOutput](t, callTool(t, session, "client_field_options_list_v1", map[string]any{
		"workspaceId": state.workspaceID, "fieldId": string(segment.ID),
	}))
	if len(options.Options) != 1 || options.Options[0].ID != option.ID {
		t.Fatalf("options = %+v", options.Options)
	}

	client := createClientTool(t, session, state.workspaceID, "Globex")
	setTier := decodeOutput[clientOutput](t, callTool(t, session, "clients_value_set_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(client.ID), "version": 1,
		"fieldId": string(tier.ID), "value": "Gold",
	}))
	if setTier.Version != 2 {
		t.Fatalf("set value = %+v", setTier)
	}
	setSegment := callTool(t, session, "clients_value_set_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(client.ID), "version": 2,
		"fieldId": string(segment.ID), "value": string(option.ID),
	})
	if setSegment.IsError {
		t.Fatalf("set choice value = %s", contentText(setSegment))
	}
	values := decodeOutput[valuesOutput](t, callTool(t, session, "clients_values_list_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(client.ID),
	}))
	if len(values.Values) != 2 || values.Values[0].Value == nil || *values.Values[0].Value != "Gold" {
		t.Fatalf("values = %+v", values.Values)
	}

	cleared := decodeOutput[clientOutput](t, callTool(t, session, "clients_value_clear_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(client.ID), "version": 3, "fieldId": string(tier.ID),
	}))
	if cleared.Version != 4 {
		t.Fatalf("clear value = %+v", cleared)
	}

	early := callTool(t, session, "client_fields_delete_v1", map[string]any{
		"workspaceId": state.workspaceID, "fieldId": string(tier.ID),
	})
	if !early.IsError || contentText(early) != "custom field must be archived before it is deleted" {
		t.Fatalf("delete active field = %+v", early)
	}
	archived := decodeOutput[fieldOutput](t, callTool(t, session, "client_fields_archive_v1", map[string]any{
		"workspaceId": state.workspaceID, "fieldId": string(tier.ID),
	}))
	if archived.ArchivedAt == nil {
		t.Fatalf("archived field = %+v", archived)
	}
	restored := decodeOutput[fieldOutput](t, callTool(t, session, "client_fields_restore_v1", map[string]any{
		"workspaceId": state.workspaceID, "fieldId": string(tier.ID),
	}))
	if restored.ArchivedAt != nil {
		t.Fatalf("restored field = %+v", restored)
	}
	decodeOutput[fieldOutput](t, callTool(t, session, "client_fields_archive_v1", map[string]any{
		"workspaceId": state.workspaceID, "fieldId": string(tier.ID),
	}))
	if deleted := decodeOutput[deletedOutput](t, callTool(t, session, "client_fields_delete_v1", map[string]any{
		"workspaceId": state.workspaceID, "fieldId": string(tier.ID),
	})); !deleted.Deleted {
		t.Fatalf("field delete = %+v", deleted)
	}
	if deleted := decodeOutput[deletedOutput](t, callTool(t, session, "client_field_options_delete_v1", map[string]any{
		"workspaceId": state.workspaceID, "fieldId": string(segment.ID), "optionId": string(option.ID),
	})); !deleted.Deleted {
		t.Fatalf("option delete = %+v", deleted)
	}
}

// TestMCPClientListFiltersSortsAndPages covers filter, sort, and the archived
// selector through the list tool, and a cursor that pages the filtered query.
func TestMCPClientListFiltersSortsAndPages(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	session := connectClient(t, state.handler, state.token.Secret, true)
	for _, name := range []string{"Alpha Corp", "Beta Corp", "Gamma Ltd"} {
		createClientTool(t, session, state.workspaceID, name)
	}
	page := decodeOutput[clientPageOutput](t, callTool(t, session, "clients_list_v1", map[string]any{
		"workspaceId": state.workspaceID, "limit": 1, "sort": "displayName",
		"filters": []map[string]any{{"field": "displayName", "op": "contains", "value": "corp"}},
	}))
	if len(page.Clients) != 1 || page.Clients[0].DisplayName != "Alpha Corp" || page.Total != 2 || page.Page != 1 {
		t.Fatalf("first page = %+v", page)
	}
	next := decodeOutput[clientPageOutput](t, callTool(t, session, "clients_list_v1", map[string]any{
		"workspaceId": state.workspaceID, "limit": 1, "sort": "displayName", "page": 2,
		"filters": []map[string]any{{"field": "displayName", "op": "contains", "value": "corp"}},
	}))
	if len(next.Clients) != 1 || next.Clients[0].DisplayName != "Beta Corp" {
		t.Fatalf("second page = %+v", next.Clients)
	}
	badSort := callTool(t, session, "clients_list_v1", map[string]any{
		"workspaceId": state.workspaceID, "sort": "nosuchfield",
	})
	if !badSort.IsError || contentText(badSort) != "parameters are invalid" {
		t.Fatalf("bad sort = %+v", badSort)
	}
}

// TestMCPClientToolsRefuseOutOfRoleCallsLikeREST proves the service's shared
// policy decides, and that MCP maps its refusal to the REST authorization message.
// A member cannot hold workspace:update, so the owner-only tools refuse the member's
// token in the service, as REST refuses it in its bearer check. A viewer's read-only
// token is refused by scope before the service runs.
func TestMCPClientToolsRefuseOutOfRoleCallsLikeREST(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	owner := connectClient(t, state.handler, state.token.Secret, true)
	client := createClientTool(t, owner, state.workspaceID, "Initech")
	field := defineFieldTool(t, owner, state.workspaceID, "Region", "text")

	member := connectClient(t, state.handler, tokenWithScopes(t, state, identity.Member,
		identity.ResourcesRead, identity.ResourcesWrite), true)
	refusals := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"member defines a field", "client_fields_define_v1", map[string]any{
			"workspaceId": state.workspaceID, "label": "Nope", "type": "text"}},
		{"member archives a field", "client_fields_archive_v1", map[string]any{
			"workspaceId": state.workspaceID, "fieldId": string(field.ID)}},
		{"member adds a choice option", "client_field_options_add_v1", map[string]any{
			"workspaceId": state.workspaceID, "fieldId": string(field.ID), "label": "Nope"}},
		{"member permanently deletes a client", "clients_delete_v1", map[string]any{
			"workspaceId": state.workspaceID, "clientId": string(client.ID)}},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			result := callTool(t, member, tc.tool, tc.args)
			if !result.IsError || contentText(result) != "access is denied" {
				t.Fatalf("result = %+v", result)
			}
		})
	}
	created := createClientTool(t, member, state.workspaceID, "Member Made")
	if created.ID == "" {
		t.Fatal("member could not create a client")
	}

	viewerRead := connectClient(t, state.handler, tokenWithScopes(t, state, identity.Viewer,
		identity.ResourcesRead), true)
	if result := callTool(t, viewerRead, "clients_get_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(client.ID),
	}); result.IsError {
		t.Fatalf("viewer read = %s", contentText(result))
	}
	// The scope gate answers before the tool runs, so the SDK reports a transport error.
	viewerWrite, err := viewerRead.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "clients_create_v1", Arguments: map[string]any{"workspaceId": state.workspaceID, "displayName": "Nope"},
	})
	if err == nil && !viewerWrite.IsError {
		t.Fatalf("viewer write with read-only token = %+v", viewerWrite)
	}
}

// TestMCPClientIDsAreShortIDsAndMalformedIDsAreRefused checks that outputs carry
// short IDs, integer IDs are refused by the schema, and an ID of another type
// is not found, as REST answers it.
func TestMCPClientIDsAreShortIDsAndMalformedIDsAreRefused(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	session := connectClient(t, state.handler, state.token.Secret, true)
	client := createClientTool(t, session, state.workspaceID, "Umbrella")
	field := defineFieldTool(t, session, state.workspaceID, "Tier", "text")
	if _, err := clients.ParseClientID(string(client.ID)); err != nil {
		t.Fatalf("client id %q is not a short ID: %v", client.ID, err)
	}

	integer := callTool(t, session, "clients_get_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": 1,
	})
	if !integer.IsError || contentText(integer) != "parameters are invalid" {
		t.Fatalf("integer id = %+v", integer)
	}
	foreign := callTool(t, session, "clients_get_v1", map[string]any{
		"workspaceId": state.workspaceID, "clientId": string(field.ID),
	})
	if !foreign.IsError || contentText(foreign) != "client not found" {
		t.Fatalf("field id as client id = %+v", foreign)
	}
}

// TestMCPClientWritesAreAuditedWithIntegerTargets checks the MCP audit row for a
// client write carries the client's integer key and no client values.
func TestMCPClientWritesAreAuditedWithIntegerTargets(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	session := connectClient(t, state.handler, state.token.Secret, true)
	created := createClientTool(t, session, state.workspaceID, "Hooli Confidential")
	key, err := clients.ParseClientID(string(created.ID))
	if err != nil {
		t.Fatal(err)
	}
	var targetType, metadata string
	var targetKey int64
	if err := state.db.QueryRow(`SELECT target_type, target_key, metadata FROM audit_events
		WHERE event_type = 'mcp.tool.called' AND action = 'clients_create_v1'`).Scan(&targetType, &targetKey, &metadata); err != nil {
		t.Fatal(err)
	}
	if targetType != "client" || targetKey != int64(key) || strings.Contains(metadata, "Hooli") {
		t.Fatalf("audit target = %q key %d metadata %s", targetType, targetKey, metadata)
	}
}

// TestMCPClientWriteToolsNeedResourcesWriteScope checks a read-only token is refused
// on a client write tool at the scope gate, before any service call.
func TestMCPClientWriteToolsNeedResourcesWriteScope(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	readOnly := tokenWithScopes(t, state, identity.Member, identity.ResourcesRead)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"clients_create_v1",` +
		`"arguments":{"workspaceId":"` + state.workspaceID + `","displayName":"Nope"}}}`
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+readOnly)
	request.Header.Set("Mcp-Name", "clients_create_v1")
	response := httptest.NewRecorder()
	state.handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMCPClientTimeline(t *testing.T) {
	state := newTestStateWith(t, config.MCP{}, true)
	owner := connectClient(t, state.handler, state.token.Secret, true)
	client := createClientTool(t, owner, state.workspaceID, "Timeline")
	args := map[string]any{"workspaceId": state.workspaceID, "clientId": string(client.ID), "type": "note", "text": "First"}
	first := callTool(t, owner, "clients_timeline_add_v1", args)
	if first.IsError {
		t.Fatalf("add: %s", contentText(first))
	}
	entry := decodeOutput[map[string]any](t, first)
	args["type"], args["channel"], args["text"], args["correctsId"] = "call", "phone", "Second", entry["id"]
	if result := callTool(t, owner, "clients_timeline_add_v1", args); result.IsError {
		t.Fatalf("correction: %s", contentText(result))
	}
	viewer := connectClient(t, state.handler, tokenWithScopes(t, state, identity.Viewer, identity.ResourcesRead), true)
	page := decodeOutput[struct {
		Entries []map[string]any `json:"entries"`
	}](t, callTool(t, viewer, "clients_timeline_list_v1", map[string]any{"workspaceId": state.workspaceID, "clientId": string(client.ID), "limit": 1, "page": 2}))
	if len(page.Entries) != 1 || page.Entries[0]["text"] != "Second" || page.Entries[0]["correctsId"] != entry["id"] {
		t.Fatalf("page = %+v", page)
	}
	for _, bad := range []map[string]any{{"type": "bad", "text": "x"}, {"type": "note", "channel": "bad", "text": "x"}, {"type": "note", "text": strings.Repeat("é", 2001)}, {"type": "note", "text": " "}} {
		bad["workspaceId"], bad["clientId"] = state.workspaceID, string(client.ID)
		if result := callTool(t, owner, "clients_timeline_add_v1", bad); !result.IsError {
			t.Fatalf("accepted invalid input: %+v", bad)
		}
	}
	missing := callTool(t, viewer, "clients_timeline_list_v1", map[string]any{"workspaceId": state.workspaceID, "clientId": "2222"})
	if !missing.IsError || contentText(missing) != "client not found" {
		t.Fatalf("missing: %+v", missing)
	}
	result, err := viewer.CallTool(context.Background(), &mcp.CallToolParams{Name: "clients_timeline_add_v1", Arguments: args})
	if err == nil && !result.IsError {
		t.Fatal("viewer write accepted")
	}
}
