package mcpserver

import (
	"context"
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

const maxResultBytes = 1 << 20

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
}

type auditSlotKey struct{}

// auditSlot records whether a typed handler ran, so the receiving middleware
// audits only the calls the SDK rejects before any handler sees them.
type auditSlot struct{ handled bool }

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
	server.AddReceivingMiddleware(h.auditRejectedCalls)
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
	})(scopeMiddleware(streamable))
	return originMiddleware(authenticated, cfg.AllowedOrigins)
}

func scopeMiddleware(next http.Handler) http.Handler {
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
	if !approved(request) {
		run.err = approvalRequired()
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
	if !approved(request) {
		run.err = approvalRequired()
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
		started := time.Now()
		if slot, ok := ctx.Value(auditSlotKey{}).(*auditSlot); ok {
			slot.handled = true
		}
		out, result := run(ctx, request, in)
		if result.err == nil {
			if encoded, err := json.Marshal(out); err != nil || len(encoded) > maxResultBytes {
				result.err = &toolError{category: "internal", message: "internal failure"}
			}
		}
		if err := h.audit(ctx, request, tool, result, started); err != nil {
			h.logger.Error("MCP audit failed", "request_id", requestID(request))
			return nil, out, errors.New("internal failure")
		}
		return nil, out, result.err
	})
}

// auditRejectedCalls audits tools/call requests the SDK rejects before a typed
// handler runs, and returns the same safe error the handler would have returned.
func (h *handler) auditRejectedCalls(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
		call, ok := request.(*mcp.CallToolRequest)
		if !ok || method != "tools/call" || h.tools[call.Params.Name] == nil {
			return next(ctx, method, request)
		}
		started := time.Now()
		slot := &auditSlot{}
		result, err := next(context.WithValue(ctx, auditSlotKey{}, slot), method, request)
		if slot.handled || err != nil {
			return result, err
		}
		// Rejected arguments are unauthorized input, so the record carries no workspace or item.
		run := toolRun{err: invalidParameters()}
		if err := h.audit(ctx, call, h.tools[call.Params.Name], run, started); err != nil {
			h.logger.Error("MCP audit failed", "request_id", requestID(call))
			return errorResult("internal failure"), nil
		}
		return errorResult(run.err.Error()), nil
	}
}

func approved(request *mcp.CallToolRequest) bool {
	return request.Extra != nil && request.Extra.Header.Get("Mcp-Human-Approval") == "true"
}

func (h *handler) audit(ctx context.Context, request *mcp.CallToolRequest, tool *mcp.Tool, result toolRun, started time.Time) error {
	actor := principal(request)
	clientName, clientVersion := "", ""
	if client := request.ClientInfo(); client != nil {
		clientName, clientVersion = client.Name, client.Version
	}
	category, outcome := "", "success"
	if result.err != nil {
		outcome = "failure"
		if value, ok := result.err.(*toolError); ok {
			category = value.category
		} else {
			category = "internal"
		}
	}
	metadata, _ := json.Marshal(map[string]any{
		"token_id": actor.TokenID, "client_name": clientName,
		"client_version": clientVersion, "tool_version": "v1",
		"annotations": tool.Annotations, "error_category": category,
		"duration_ms": time.Since(started).Milliseconds(),
	})
	return h.identity.RecordAudit(ctx, identity.AuditEvent{
		EventType: "mcp.tool.called", ActorUserID: actor.UserID,
		AuthMethod: actor.AuthMethod, WorkspaceID: result.workspaceID,
		TargetType: "item", TargetID: result.targetID, Action: tool.Name,
		Outcome: outcome, RequestID: requestID(request),
		SourceAddress: sourceAddress(request), Metadata: string(metadata), CreatedAt: time.Now().UTC(),
	})
}

func principal(request *mcp.CallToolRequest) identity.Principal {
	return principalFromExtra(request.Extra)
}

func requestID(request *mcp.CallToolRequest) string {
	if request.Extra == nil {
		return ""
	}
	return request.Extra.Header.Get("X-Request-ID")
}

func sourceAddress(request *mcp.CallToolRequest) string {
	if request.Extra == nil || request.Extra.TokenInfo == nil {
		return ""
	}
	value, _ := request.Extra.TokenInfo.Extra["sourceAddress"].(string)
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

func approvalRequired() error {
	return &toolError{category: "approval_required", message: "human approval is required"}
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
