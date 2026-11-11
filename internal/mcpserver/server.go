package mcpserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxResultBytes  = 1 << 20
	approvalInputID = "human-approval"
)

var approvalSchema = map[string]any{
	"type": "object", "additionalProperties": false,
	"properties": map[string]any{"confirm": map[string]any{"type": "boolean", "title": "Confirm this change"}},
	"required":   []string{"confirm"},
}

// toolsListTTLMs is the freshness hint for tools/list. The tool catalog changes
// only on deployment, so clients may poll it at most every five minutes.
const toolsListTTLMs = 300_000

var (
	falseValue = false
	trueValue  = true
)

type handler struct {
	identity *identity.Service
	items    *items.Service
	logger   *slog.Logger
	tools    map[string]*mcp.Tool
}

// Contract types. The SDK derives each tool schema from these Go types, so the
// published constraints live on the types in contractTypes.
type (
	resourceID     string
	newTitle       string
	itemTitle      string
	idempotencyKey string
	searchText     string
	sortOrder      string
	pageLimit      int
	cursorToken    string
	nextCursor     string
	itemVersion    int64
)

// contractTypes holds the keyword constraints that jsonschema-go cannot derive
// from a Go type: patterns, bounds, enums, and defaults.
var contractTypes = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[resourceID]():     {Type: "string", Pattern: "^[0-9a-f]{32}$"},
	reflect.TypeFor[newTitle]():       {Type: "string", MinLength: new(1), MaxLength: new(200)},
	reflect.TypeFor[itemTitle]():      {Type: "string", MaxLength: new(200)},
	reflect.TypeFor[idempotencyKey](): {Type: "string", MinLength: new(1), MaxLength: new(255)},
	reflect.TypeFor[searchText](): {
		Type: "string", MinLength: new(1), MaxLength: new(100),
		Description: "Case-insensitive literal substring of title.",
	},
	reflect.TypeFor[sortOrder](): {
		Type: "string", Enum: []any{"created_at", "-created_at"}, Default: json.RawMessage(`"-created_at"`),
	},
	reflect.TypeFor[items.Status](): {Type: "string", Enum: []any{"active", "complete"}},
	reflect.TypeFor[pageLimit](): {
		Type: "integer", Minimum: new(1.0), Maximum: new(100.0), Default: json.RawMessage("50"),
	},
	reflect.TypeFor[cursorToken](): {Type: "string", MaxLength: new(512)},
	reflect.TypeFor[nextCursor]():  {Type: "string", MaxLength: new(700)},
	reflect.TypeFor[itemVersion](): {Type: "integer", Minimum: new(1.0)},
	reflect.TypeFor[time.Time]():   {Type: "string", Format: "date-time"},
}

type listInput struct {
	WorkspaceID resourceID   `json:"workspaceId"`
	Status      items.Status `json:"status,omitempty"`
	Search      searchText   `json:"search,omitempty"`
	Sort        sortOrder    `json:"sort,omitempty"`
	Limit       pageLimit    `json:"limit,omitempty"`
	Cursor      cursorToken  `json:"cursor,omitempty"`
}

type itemInput struct {
	WorkspaceID resourceID `json:"workspaceId"`
	ItemID      resourceID `json:"itemId"`
}

type createInput struct {
	WorkspaceID    resourceID     `json:"workspaceId"`
	Title          newTitle       `json:"title"`
	IdempotencyKey idempotencyKey `json:"idempotencyKey"`
}

type updateInput struct {
	WorkspaceID resourceID   `json:"workspaceId"`
	ItemID      resourceID   `json:"itemId"`
	Version     itemVersion  `json:"version"`
	Title       newTitle     `json:"title,omitempty"`
	Status      items.Status `json:"status,omitempty"`
}

type deleteInput struct {
	WorkspaceID resourceID  `json:"workspaceId"`
	ItemID      resourceID  `json:"itemId"`
	Version     itemVersion `json:"version"`
}

type itemOutput struct {
	ID              resourceID   `json:"id"`
	WorkspaceID     resourceID   `json:"workspaceId"`
	CreatedByUserID *string      `json:"createdByUserId"`
	Title           itemTitle    `json:"title"`
	Status          items.Status `json:"status"`
	Version         itemVersion  `json:"version"`
	CreatedAt       time.Time    `json:"createdAt"`
	UpdatedAt       time.Time    `json:"updatedAt"`
}

type pageOutput struct {
	Items      []itemOutput `json:"items"`
	NextCursor nextCursor   `json:"nextCursor,omitempty"`
}

type deleteOutput struct {
	Deleted bool       `json:"deleted"`
	ItemID  resourceID `json:"itemId"`
}

type toolError struct {
	category string
	message  string
}

func (e *toolError) Error() string { return e.message }

type toolRun struct {
	workspaceID string
	targetID    string
	err         error
	// pending is an input_required result; it is returned without auditing a call outcome.
	pending *mcp.CallToolResult
}

const (
	methodToolsCall    = "tools/call"
	eventToolCalled    = "mcp.tool.called"
	eventRequestCalled = "mcp.request.called"
)

// callRecord carries facts known only to tool adapters to the receiving
// middleware, which writes exactly one audit record per dispatched request.
type callRecord struct {
	handled     bool // a typed tool handler ran, so the SDK accepted the arguments
	prompted    bool // an MRTR confirmation prompt was returned; no call outcome exists yet
	toolName    string
	workspaceID string
	targetID    string
	annotations *mcp.ToolAnnotations
	err         error
}

type callRecordKey struct{}

func recordFor(ctx context.Context) *callRecord {
	if record, ok := ctx.Value(callRecordKey{}).(*callRecord); ok {
		return record
	}
	return &callRecord{}
}

func NewHandler(
	identityService *identity.Service,
	itemService *items.Service,
	cfg config.MCP,
	logger *slog.Logger,
) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &handler{identity: identityService, items: itemService, logger: logger, tools: map[string]*mcp.Tool{}}
	server := mcp.NewServer(&mcp.Implementation{
		Name: "dynamis-code-apps-template", Version: "0.1.0",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{}, PageSize: 100, CompletionHandler: h.complete,
		// The catalog is identical for every authenticated principal and holds no
		// workspace data, so shared caches may keep it. This holds only while
		// addTools registers tools without per-principal filtering; revisit "public"
		// before adding any. Other list results keep the
		// SDK's zero TTL (immediately stale) until they opt in here.
		SetCacheable: func(_ context.Context, req mcp.Request, c *mcp.Cacheable) {
			if _, ok := req.(*mcp.ListToolsRequest); ok {
				c.TTLMs, c.CacheScope = toolsListTTLMs, "public"
			}
		},
	})
	h.addTools(server)
	server.AddReceivingMiddleware(h.observe)
	h.addPrompts(server)
	h.addItemListPrompts(server)
	h.addResources(server)
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{
			Stateless: true, JSONResponse: true, Logger: logger,
			MaxRequestBodyBytes: maxResultBytes, PropagateRequestCancellation: true,
		},
	)
	verifier := func(ctx context.Context, token string, request *http.Request) (*auth.TokenInfo, error) {
		principal, err := identityService.AuthenticateAPIToken(ctx, token, "", identity.AuditContext{
			RequestID: request.Header.Get("X-Request-ID"), SourceAddress: sourceHost(request.RemoteAddr),
		})
		if errors.Is(err, identity.ErrInvalidToken) {
			principal, err = identityService.AuthenticateOAuthAccessToken(ctx, token)
			if errors.Is(err, identity.ErrMFARequired) {
				return nil, auth.ErrInvalidToken
			}
		}
		if errors.Is(err, identity.ErrInvalidToken) {
			return nil, auth.ErrInvalidToken
		}
		if errors.Is(err, identity.ErrForbidden) {
			return nil, fmt.Errorf("%w: insufficient scope", auth.ErrInvalidToken)
		}
		if err != nil {
			return nil, errors.New("token verification failed")
		}
		scopes := make([]string, 0, len(principal.Permissions))
		for scope, allowed := range principal.Permissions {
			if allowed {
				scopes = append(scopes, string(scope))
			}
		}
		return &auth.TokenInfo{
			Scopes: scopes, UserID: principal.UserID,
			Extra: map[string]any{"principal": principal, "sourceAddress": sourceHost(request.RemoteAddr)},
		}, nil
	}
	authenticated := auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
		AllowMissingExpiration: true,
		ResourceMetadataURL:    protectedResourceMetadataURL(cfg),
	})(scopeMiddleware(streamable, insufficientScopeChallenge(cfg)))
	return originMiddleware(invalidTokenChallengeMiddleware(authenticated, cfg), cfg.AllowedOrigins)
}

func scopeMiddleware(next http.Handler, challenge string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token := auth.TokenInfoFromContext(request.Context())
		needed := string(identity.ResourcesRead)
		if writeTools[request.Header.Get("Mcp-Name")] {
			needed = string(identity.ResourcesWrite)
		}
		for _, scope := range token.Scopes {
			if scope == needed {
				next.ServeHTTP(writer, request)
				return
			}
		}
		if challenge != "" {
			writer.Header().Set("WWW-Authenticate", challenge)
		}
		http.Error(writer, "insufficient scope", http.StatusForbidden)
	})
}

var writeTools = map[string]bool{
	"items_create_v1": true,
	"items_update_v1": true,
	"items_delete_v1": true,
}

func originMiddleware(next http.Handler, allowed []string) http.Handler {
	allow := make(map[string]bool, len(allowed))
	for _, origin := range allowed {
		allow[origin] = true
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if origin := request.Header.Get("Origin"); origin != "" && !allow[origin] {
			http.Error(writer, "origin is not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (h *handler) addTools(server *mcp.Server) {
	read := &mcp.ToolAnnotations{
		ReadOnlyHint: true, DestructiveHint: &falseValue,
		IdempotentHint: true, OpenWorldHint: &falseValue,
	}
	write := &mcp.ToolAnnotations{
		ReadOnlyHint: false, DestructiveHint: &falseValue,
		IdempotentHint: true, OpenWorldHint: &falseValue,
	}
	destructive := &mcp.ToolAnnotations{
		ReadOnlyHint: false, DestructiveHint: &trueValue,
		IdempotentHint: true, OpenWorldHint: &falseValue,
	}
	addTool(server, h, &mcp.Tool{
		Name: "items_list_v1", Description: "List one workspace's items with bounded cursor pagination.",
		Annotations: read,
	}, pageBounds, h.listItems)
	addTool(server, h, &mcp.Tool{
		Name: "items_get_v1", Description: "Get one item in an authorized workspace.", Annotations: read,
	}, nil, h.getItem)
	addTool(server, h, &mcp.Tool{
		Name: "items_create_v1", Description: "Create an item using a caller-supplied idempotency key.",
		Annotations: write,
	}, nil, h.createItem)
	addTool(server, h, &mcp.Tool{
		Name: "items_update_v1", Description: "Update an item when its version matches.",
		Annotations: destructive,
	}, titleOrStatus, h.updateItem)
	addTool(server, h, &mcp.Tool{
		Name: "items_delete_v1", Description: "Permanently delete an item after client-side human approval.",
		Annotations: destructive,
	}, nil, h.deleteItem)
}

// pageBounds caps list results at 100 items. toPageOutput never returns null
// for items, so the array is not nullable, which jsonschema-go derives for slices.
func pageBounds(_, output *jsonschema.Schema) {
	list := output.Properties["items"]
	list.Type, list.Types, list.MaxItems = "array", nil, new(100)
}

// titleOrStatus requires at least one updatable field, matching the published schema.
func titleOrStatus(input, _ *jsonschema.Schema) {
	input.AnyOf = []*jsonschema.Schema{{Required: []string{"title"}}, {Required: []string{"status"}}}
}

func (h *handler) listItems(ctx context.Context, request *mcp.CallToolRequest, input listInput) (pageOutput, toolRun) {
	page, err := h.items.List(ctx, principal(request), string(input.WorkspaceID), items.ListInput{
		Status: input.Status, Search: string(input.Search), Sort: string(input.Sort),
		Limit: int(input.Limit), Cursor: string(input.Cursor),
	})
	return toPageOutput(page), toolRun{workspaceID: string(input.WorkspaceID), err: publicError(err)}
}

func (h *handler) getItem(ctx context.Context, request *mcp.CallToolRequest, input itemInput) (itemOutput, toolRun) {
	run := toolRun{workspaceID: string(input.WorkspaceID), targetID: string(input.ItemID)}
	item, err := h.items.Get(ctx, principal(request), string(input.WorkspaceID), string(input.ItemID))
	run.err = publicError(err)
	return toItemOutput(item), run
}

func (h *handler) createItem(ctx context.Context, request *mcp.CallToolRequest, input createInput) (itemOutput, toolRun) {
	result, err := h.items.Create(ctx, principal(request), string(input.WorkspaceID), string(input.Title),
		string(input.IdempotencyKey), auditContext(request))
	return toItemOutput(result.Item), toolRun{
		workspaceID: string(input.WorkspaceID), targetID: result.Item.ID, err: publicError(err),
	}
}

func (h *handler) updateItem(ctx context.Context, request *mcp.CallToolRequest, input updateInput) (itemOutput, toolRun) {
	run := toolRun{workspaceID: string(input.WorkspaceID), targetID: string(input.ItemID)}
	if pending, err := approvalGate(request, "items_update_v1", input,
		fmt.Sprintf("Confirm the update of item %s to version %d.", input.ItemID, input.Version)); pending != nil || err != nil {
		run.pending, run.err = pending, err
		return itemOutput{}, run
	}
	// Zero values mean "absent": the schema already rejects empty title and status.
	var update items.UpdateInput
	if input.Title != "" {
		title := string(input.Title)
		update.Title = &title
	}
	if input.Status != "" {
		status := input.Status
		update.Status = &status
	}
	item, err := h.items.Update(ctx, principal(request), string(input.WorkspaceID), string(input.ItemID),
		int64(input.Version), update, auditContext(request))
	run.err = publicError(err)
	return toItemOutput(item), run
}

func (h *handler) deleteItem(ctx context.Context, request *mcp.CallToolRequest, input deleteInput) (deleteOutput, toolRun) {
	run := toolRun{workspaceID: string(input.WorkspaceID), targetID: string(input.ItemID)}
	if pending, err := approvalGate(request, "items_delete_v1", input,
		fmt.Sprintf("Confirm permanent deletion of item %s at version %d.", input.ItemID, input.Version)); pending != nil || err != nil {
		run.pending, run.err = pending, err
		return deleteOutput{}, run
	}
	err := h.items.Delete(ctx, principal(request), string(input.WorkspaceID), string(input.ItemID),
		int64(input.Version), auditContext(request))
	run.err = publicError(err)
	return deleteOutput{Deleted: err == nil, ItemID: input.ItemID}, run
}

func toItemOutput(item items.Item) itemOutput {
	return itemOutput{
		ID: resourceID(item.ID), WorkspaceID: resourceID(item.WorkspaceID),
		CreatedByUserID: item.CreatedByUserID, Title: itemTitle(item.Title), Status: item.Status,
		Version: itemVersion(item.Version), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func toPageOutput(page items.Page) pageOutput {
	output := pageOutput{Items: make([]itemOutput, 0, len(page.Items)), NextCursor: nextCursor(page.NextCursor)}
	for _, item := range page.Items {
		output.Items = append(output.Items, toItemOutput(item))
	}
	return output
}

// schemaFor derives a tool schema from a contract type. The SDK derives only
// structure (types, required fields, additionalProperties) from Go types.
func schemaFor[T any]() *jsonschema.Schema {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: contractTypes})
	if err != nil {
		panic(fmt.Errorf("derive schema for %v: %w", reflect.TypeFor[T](), err))
	}
	return schema
}

// addTool registers a typed tool. The SDK validates arguments against the
// derived input schema before run; this wrapper owns output limits and audit.
func addTool[In, Out any](
	server *mcp.Server, h *handler, tool *mcp.Tool,
	adjust func(input, output *jsonschema.Schema),
	run func(context.Context, *mcp.CallToolRequest, In) (Out, toolRun),
) {
	input, output := schemaFor[In](), schemaFor[Out]()
	if adjust != nil {
		adjust(input, output)
	}
	tool.InputSchema, tool.OutputSchema = input, output
	h.tools[tool.Name] = tool
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		record := recordFor(ctx)
		record.handled, record.toolName, record.annotations = true, tool.Name, tool.Annotations
		out, result := run(ctx, request, in)
		if result.pending != nil {
			record.prompted, record.workspaceID, record.targetID = true, result.workspaceID, result.targetID
			var none Out
			return result.pending, none, nil
		}
		if result.err == nil {
			if encoded, err := json.Marshal(out); err != nil || len(encoded) > maxResultBytes {
				result.err = &toolError{category: "internal", message: "internal failure"}
			}
		}
		record.workspaceID, record.targetID, record.err = result.workspaceID, result.targetID, result.err
		return nil, out, result.err
	})
}

// observe wraps every method the SDK dispatches to the server. It is the only
// place that audits, times, and recovers panics, so tool and non-tool requests
// share one failure contract. Arguments the SDK rejects before a typed handler
// runs are audited as invalid parameters with the registered tool's identity,
// and callers get the same safe message the handlers use.
func (h *handler) observe(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (result mcp.Result, err error) {
		started := time.Now()
		record := &callRecord{}
		ctx = context.WithValue(ctx, callRecordKey{}, record)
		defer func() {
			if recovered := recover(); recovered != nil {
				h.logger.Error("MCP request panicked", "method", method, "request_id", requestID(req))
				record.err = &toolError{category: "internal", message: "internal failure"}
				result, err = internalFailure(method)
			}
			if record.prompted {
				return
			}
			if auditErr := h.audit(ctx, method, req, record, result, err, started); auditErr != nil {
				h.logger.Error("MCP audit failed", "method", method, "request_id", requestID(req))
				result, err = internalFailure(method)
			}
		}()
		result, err = next(ctx, method, req)
		if call, ok := req.(*mcp.CallToolRequest); ok && method == methodToolsCall && err == nil && !record.handled {
			if tool := h.tools[call.Params.Name]; tool != nil {
				// Rejected arguments are unauthorized input, so the record carries no workspace or item.
				record.toolName, record.annotations, record.err = tool.Name, tool.Annotations, invalidParameters()
				result = errorResult(record.err.Error())
			}
		}
		return result, err
	}
}

func internalFailure(method string) (mcp.Result, error) {
	if method == methodToolsCall {
		return errorResult("internal failure"), nil
	}
	return nil, errors.New("internal failure")
}

// approvalGate blocks destructive calls until the client host confirms them
// through a multi round-trip (SEP-2322) input request. The confirmation is bound
// to the prompted arguments through the request state, so the endpoint stays
// stateless. The gate is a client-host confirmation; it never replaces server
// authorization.
func approvalGate(request *mcp.CallToolRequest, tool string, input any, message string) (*mcp.CallToolResult, error) {
	state := approvalState(tool, input)
	response, answered := request.Params.InputResponses[approvalInputID]
	if !answered || request.Params.RequestState != state {
		return &mcp.CallToolResult{
			InputRequests: mcp.InputRequestMap{approvalInputID: &mcp.ElicitParams{
				Mode: "form", Message: message, RequestedSchema: approvalSchema,
			}},
			RequestState: state,
		}, nil
	}
	if confirmed, ok := response.(*mcp.ElicitResult); ok && confirmed.Action == "accept" && confirmed.Content["confirm"] == true {
		return nil, nil
	}
	return nil, &toolError{category: "approval_declined", message: "human approval was declined"}
}

// approvalState binds an approval to the tool and the exact arguments that were prompted.
func approvalState(tool string, input any) string {
	encoded, _ := json.Marshal(input)
	sum := sha256.Sum256(append([]byte(tool+"\n"), encoded...))
	return hex.EncodeToString(sum[:])
}

func (h *handler) audit(
	ctx context.Context, method string, req mcp.Request, record *callRecord,
	result mcp.Result, err error, started time.Time,
) error {
	actor := principal(req)
	clientName, clientVersion := "", ""
	if client, ok := req.(interface{ ClientInfo() *mcp.Implementation }); ok && client.ClientInfo() != nil {
		clientName, clientVersion = client.ClientInfo().Name, client.ClientInfo().Version
	}
	category := ""
	switch {
	case record.err != nil:
		category = errorCategory(record.err)
	case err != nil:
		category = "protocol_error"
	default:
		if value, ok := result.(*mcp.CallToolResult); ok && value.IsError {
			category = "tool_error"
		}
	}
	outcome := "success"
	if category != "" {
		outcome = "failure"
	}
	fields := map[string]any{
		"token_id": actor.TokenID, "client_name": clientName,
		"client_version": clientVersion, "error_category": category,
		"duration_ms": time.Since(started).Milliseconds(),
	}
	eventType, action, targetType := eventRequestCalled, method, ""
	if method == methodToolsCall {
		eventType, targetType = eventToolCalled, "item"
		if record.toolName != "" {
			action = record.toolName
		} // unregistered names are client input and are never logged
		fields["tool_version"], fields["annotations"] = "v1", record.annotations
	} else {
		fields["method"] = method
	}
	metadata, _ := json.Marshal(fields)
	return h.identity.RecordAudit(ctx, identity.AuditEvent{
		EventType: eventType, ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: record.workspaceID,
		TargetType: targetType, TargetID: record.targetID, Action: action,
		Outcome: outcome, RequestID: requestID(req),
		SourceAddress: sourceAddress(req), Metadata: string(metadata), CreatedAt: time.Now().UTC(),
	})
}

func errorCategory(err error) string {
	if value, ok := err.(*toolError); ok {
		return value.category
	}
	return "internal"
}

func principal(request mcp.Request) identity.Principal {
	return principalFromExtra(request.GetExtra())
}

func requestID(request mcp.Request) string {
	extra := request.GetExtra()
	if extra == nil {
		return ""
	}
	return extra.Header.Get("X-Request-ID")
}

func sourceAddress(request mcp.Request) string {
	extra := request.GetExtra()
	if extra == nil || extra.TokenInfo == nil {
		return ""
	}
	value, _ := extra.TokenInfo.Extra["sourceAddress"].(string)
	return value
}

func auditContext(request *mcp.CallToolRequest) identity.AuditContext {
	return identity.AuditContext{RequestID: requestID(request), SourceAddress: sourceAddress(request)}
}

func publicError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, identity.ErrForbidden):
		return &toolError{category: "forbidden", message: "access is denied"}
	case errors.Is(err, items.ErrNotFound):
		return &toolError{category: "not_found", message: "item not found"}
	case errors.Is(err, items.ErrInvalidCursor):
		return &toolError{category: "invalid_parameters", message: "cursor is invalid"}
	case errors.Is(err, items.ErrInvalidInput):
		return invalidParameters()
	case errors.Is(err, items.ErrIdempotencyConflict):
		return &toolError{category: "conflict", message: "idempotency key conflicts with another request"}
	case errors.Is(err, items.ErrPreconditionFailed):
		return &toolError{category: "precondition_failed", message: "item version is stale"}
	case errors.Is(err, items.ErrLimit):
		return &toolError{category: "resource_limit", message: "workspace item limit reached"}
	default:
		return &toolError{category: "internal", message: "internal failure"}
	}
}

func invalidParameters() error {
	return &toolError{category: "invalid_parameters", message: "parameters are invalid"}
}

func errorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

func sourceHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return ""
	}
	return host
}

func validID(value string) bool {
	if len(value) != 32 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16
}
