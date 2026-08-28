package mcpserver

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maxResultBytes = 1 << 20

var (
	falseValue = false
	trueValue  = true
)

type handler struct {
	identity *identity.Service
	items    *items.Service
	logger   *slog.Logger
}

type listInput struct {
	WorkspaceID string       `json:"workspaceId"`
	Status      items.Status `json:"status"`
	Search      string       `json:"search"`
	Sort        string       `json:"sort"`
	Limit       int          `json:"limit"`
	Cursor      string       `json:"cursor"`
}

type itemInput struct {
	WorkspaceID string `json:"workspaceId"`
	ItemID      string `json:"itemId"`
}

type createInput struct {
	WorkspaceID    string `json:"workspaceId"`
	Title          string `json:"title"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type updateInput struct {
	WorkspaceID string        `json:"workspaceId"`
	ItemID      string        `json:"itemId"`
	Version     int64         `json:"version"`
	Title       *string       `json:"title"`
	Status      *items.Status `json:"status"`
}

type deleteInput struct {
	WorkspaceID string `json:"workspaceId"`
	ItemID      string `json:"itemId"`
	Version     int64  `json:"version"`
}

type deleteOutput struct {
	Deleted bool   `json:"deleted"`
	ItemID  string `json:"itemId"`
}

type toolError struct {
	category string
	message  string
}

func (e *toolError) Error() string { return e.message }

type toolRun struct {
	workspaceID string
	targetID    string
	output      any
	err         error
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
	h := &handler{identity: identityService, items: itemService, logger: logger}
	server := mcp.NewServer(&mcp.Implementation{
		Name: "dynamis-code-apps-template", Version: "0.1.0",
	}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{}, PageSize: 100})
	h.addTools(server)
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
	h.add(server, &mcp.Tool{
		Name: "items_list_v1", Description: "List one workspace's items with bounded cursor pagination.",
		Annotations: read, InputSchema: listInputSchema(), OutputSchema: pageSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) toolRun {
		var input listInput
		if err := decode(req, &input); err != nil || !validID(input.WorkspaceID) || len(input.Cursor) > 700 {
			return toolRun{err: invalidParameters()}
		}
		if input.Sort == "" {
			input.Sort = "-created_at"
		}
		if input.Limit == 0 {
			input.Limit = 50
		}
		page, err := h.items.List(ctx, principal(req), input.WorkspaceID, items.ListInput{
			Status: input.Status, Search: input.Search, Sort: input.Sort,
			Limit: input.Limit, Cursor: input.Cursor,
		})
		return toolRun{workspaceID: input.WorkspaceID, output: page, err: publicError(err)}
	})
	h.add(server, &mcp.Tool{
		Name: "items_get_v1", Description: "Get one item in an authorized workspace.",
		Annotations: read, InputSchema: itemInputSchema(), OutputSchema: itemSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) toolRun {
		var input itemInput
		if err := decode(req, &input); err != nil || !validID(input.WorkspaceID) || !validID(input.ItemID) {
			return toolRun{err: invalidParameters()}
		}
		item, err := h.items.Get(ctx, principal(req), input.WorkspaceID, input.ItemID)
		return toolRun{workspaceID: input.WorkspaceID, targetID: input.ItemID, output: item, err: publicError(err)}
	})
	h.add(server, &mcp.Tool{
		Name: "items_create_v1", Description: "Create an item using a caller-supplied idempotency key.",
		Annotations: write, InputSchema: createInputSchema(), OutputSchema: itemSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) toolRun {
		var input createInput
		if err := decode(req, &input); err != nil || !validID(input.WorkspaceID) {
			return toolRun{err: invalidParameters()}
		}
		result, err := h.items.Create(ctx, principal(req), input.WorkspaceID, input.Title,
			input.IdempotencyKey, auditContext(req))
		return toolRun{workspaceID: input.WorkspaceID, targetID: result.Item.ID, output: result.Item, err: publicError(err)}
	})
	h.add(server, &mcp.Tool{
		Name: "items_update_v1", Description: "Update an item when its version matches.",
		Annotations: destructive, InputSchema: updateInputSchema(), OutputSchema: itemSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) toolRun {
		var input updateInput
		if err := decode(req, &input); err != nil || !validID(input.WorkspaceID) || !validID(input.ItemID) {
			return toolRun{err: invalidParameters()}
		}
		if !approved(req) {
			return toolRun{workspaceID: input.WorkspaceID, targetID: input.ItemID,
				err: &toolError{category: "approval_required", message: "human approval is required"}}
		}
		item, err := h.items.Update(ctx, principal(req), input.WorkspaceID, input.ItemID,
			input.Version, items.UpdateInput{Title: input.Title, Status: input.Status}, auditContext(req))
		return toolRun{workspaceID: input.WorkspaceID, targetID: input.ItemID, output: item, err: publicError(err)}
	})
	h.add(server, &mcp.Tool{
		Name: "items_delete_v1", Description: "Permanently delete an item after client-side human approval.",
		Annotations: destructive, InputSchema: deleteInputSchema(), OutputSchema: deleteOutputSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) toolRun {
		var input deleteInput
		if err := decode(req, &input); err != nil || !validID(input.WorkspaceID) || !validID(input.ItemID) {
			return toolRun{err: invalidParameters()}
		}
		if !approved(req) {
			return toolRun{workspaceID: input.WorkspaceID, targetID: input.ItemID,
				err: &toolError{category: "approval_required", message: "human approval is required"}}
		}
		err := h.items.Delete(ctx, principal(req), input.WorkspaceID, input.ItemID,
			input.Version, auditContext(req))
		return toolRun{workspaceID: input.WorkspaceID, targetID: input.ItemID,
			output: deleteOutput{Deleted: err == nil, ItemID: input.ItemID}, err: publicError(err)}
	})
}

func approved(request *mcp.CallToolRequest) bool {
	return request.Extra != nil && request.Extra.Header.Get("Mcp-Human-Approval") == "true"
}

func (h *handler) add(server *mcp.Server, tool *mcp.Tool, run func(context.Context, *mcp.CallToolRequest) toolRun) {
	server.AddTool(tool, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		started := time.Now()
		result := run(ctx, request)
		if result.err == nil {
			encoded, err := json.Marshal(result.output)
			if err != nil || len(encoded) > maxResultBytes {
				result.err = &toolError{category: "internal", message: "internal failure"}
			} else {
				var structured any
				if json.Unmarshal(encoded, &structured) != nil {
					result.err = &toolError{category: "internal", message: "internal failure"}
				} else {
					result.output = structured
					if err := h.audit(ctx, request, tool, result, started); err != nil {
						h.logger.Error("MCP audit failed", "request_id", requestID(request))
						return errorResult("internal failure"), nil
					}
					return &mcp.CallToolResult{
						Content:           []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
						StructuredContent: result.output,
					}, nil
				}
			}
		}
		if err := h.audit(ctx, request, tool, result, started); err != nil {
			h.logger.Error("MCP audit failed", "request_id", requestID(request))
			return errorResult("internal failure"), nil
		}
		return errorResult(result.err.Error()), nil
	})
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

func decode(request *mcp.CallToolRequest, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(request.Params.Arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("arguments must contain one JSON object")
	}
	return nil
}

func principal(request *mcp.CallToolRequest) identity.Principal {
	if request.Extra == nil || request.Extra.TokenInfo == nil {
		return identity.Principal{}
	}
	value, _ := request.Extra.TokenInfo.Extra["principal"].(identity.Principal)
	return value
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

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": properties, "required": required,
	}
}

var idSchema = map[string]any{"type": "string", "pattern": "^[0-9a-f]{32}$"}

func listInputSchema() any {
	return objectSchema(map[string]any{
		"workspaceId": idSchema,
		"status":      map[string]any{"type": "string", "enum": []string{"active", "complete"}},
		"search":      map[string]any{"type": "string", "minLength": 1, "maxLength": 100, "description": "Case-insensitive literal substring of title."},
		"sort":        map[string]any{"type": "string", "enum": []string{"created_at", "-created_at"}, "default": "-created_at"},
		"limit":       map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 50},
		"cursor":      map[string]any{"type": "string", "maxLength": 512},
	}, "workspaceId")
}

func itemInputSchema() any {
	return objectSchema(map[string]any{"workspaceId": idSchema, "itemId": idSchema}, "workspaceId", "itemId")
}

func createInputSchema() any {
	return objectSchema(map[string]any{
		"workspaceId":    idSchema,
		"title":          map[string]any{"type": "string", "minLength": 1, "maxLength": 200},
		"idempotencyKey": map[string]any{"type": "string", "minLength": 1, "maxLength": 255},
	}, "workspaceId", "title", "idempotencyKey")
}

func updateInputSchema() any {
	schema := objectSchema(map[string]any{
		"workspaceId": idSchema, "itemId": idSchema,
		"version": map[string]any{"type": "integer", "minimum": 1},
		"title":   map[string]any{"type": "string", "minLength": 1, "maxLength": 200},
		"status":  map[string]any{"type": "string", "enum": []string{"active", "complete"}},
	}, "workspaceId", "itemId", "version")
	schema["anyOf"] = []any{
		map[string]any{"required": []string{"title"}}, map[string]any{"required": []string{"status"}},
	}
	return schema
}

func deleteInputSchema() any {
	return objectSchema(map[string]any{
		"workspaceId": idSchema, "itemId": idSchema,
		"version": map[string]any{"type": "integer", "minimum": 1},
	}, "workspaceId", "itemId", "version")
}

func itemSchema() any {
	return objectSchema(map[string]any{
		"id": idSchema, "workspaceId": idSchema,
		"createdByUserId": map[string]any{"type": []string{"string", "null"}},
		"title":           map[string]any{"type": "string", "maxLength": 200},
		"status":          map[string]any{"type": "string", "enum": []string{"active", "complete"}},
		"version":         map[string]any{"type": "integer", "minimum": 1},
		"createdAt":       map[string]any{"type": "string", "format": "date-time"},
		"updatedAt":       map[string]any{"type": "string", "format": "date-time"},
	}, "id", "workspaceId", "createdByUserId", "title", "status", "version", "createdAt", "updatedAt")
}

func pageSchema() any {
	return objectSchema(map[string]any{
		"items":      map[string]any{"type": "array", "maxItems": 100, "items": itemSchema()},
		"nextCursor": map[string]any{"type": "string", "maxLength": 700},
	}, "items")
}

func deleteOutputSchema() any {
	return objectSchema(map[string]any{
		"deleted": map[string]any{"type": "boolean"}, "itemId": idSchema,
	}, "deleted", "itemId")
}
