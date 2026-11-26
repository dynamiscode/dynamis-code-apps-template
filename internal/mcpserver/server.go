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
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
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
	logger   *slog.Logger
	tools    map[string]*mcp.Tool
}

// Contract types. The SDK derives each tool schema from these Go types, so the
// published constraints live on the types in contractTypes.
type (
	workspaceShortID string
	userShortID      string
	pageLimit        int
	pageNumber       int
)

// contractTypes holds shared keyword constraints that jsonschema-go cannot derive.
var contractTypes = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[userShortID](): {Type: "string", Pattern: "^[2-9a-hjkmnp-z]{4,}$"},
	reflect.TypeFor[pageLimit](): {
		Type: "integer", Minimum: new(1.0), Maximum: new(100.0), Default: json.RawMessage("50"),
	},
	reflect.TypeFor[pageNumber](): {
		Type: "integer", Minimum: new(1.0), Maximum: new(1000000.0),
		Description: "1-based page number. The first page is 1.",
	},
	reflect.TypeFor[time.Time](): {Type: "string", Format: "date-time"},
}

type toolError struct {
	category string
	message  string
}

func (e *toolError) Error() string { return e.message }

type toolRun struct {
	workspaceID identity.WorkspaceID
	targetID    string
	// targetType and targetKey name the audited object for tool calls.
	targetType string
	targetKey  int64
	err        error
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
	workspaceID identity.WorkspaceID
	targetID    string
	targetType  string
	targetKey   int64
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

// Option adds tools for a composed profile.
type Option func(h *handler, server *mcp.Server)

func NewHandler(
	identityService *identity.Service,
	cfg config.MCP,
	logger *slog.Logger,
	options ...Option,
) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &handler{identity: identityService, logger: logger, tools: map[string]*mcp.Tool{}}
	server := mcp.NewServer(&mcp.Implementation{
		Name: "dynamis-code-apps-template", Version: "0.1.0",
	}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{}, PageSize: 100,
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
	for _, option := range options {
		option(h, server)
	}
	server.AddReceivingMiddleware(h.observe)
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
			Scopes: scopes, UserID: principal.UserID.String(),
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

// writeTools lists the mutation tools, which need resources:write. Client tools
// are listed here by name only; they register from clients.go.
var writeTools = map[string]bool{

	"transactions_create_v1":           true,
	"transactions_category_create_v1":  true,
	"transactions_category_rename_v1":  true,
	"transactions_category_archive_v1": true,
	"transactions_method_create_v1":    true,
	"transactions_method_update_v1":    true,
	"transactions_void_v1":             true,
	"clients_timeline_add_v1":          true,
	"clients_create_v1":                true,
	"clients_update_v1":                true,
	"clients_archive_v1":               true,
	"clients_unarchive_v1":             true,
	"clients_delete_v1":                true,
	"clients_value_set_v1":             true,
	"clients_value_clear_v1":           true,
	"client_fields_define_v1":          true,
	"client_fields_archive_v1":         true,
	"client_fields_restore_v1":         true,
	"client_fields_delete_v1":          true,
	"client_field_options_add_v1":      true,
	"client_field_options_delete_v1":   true,
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

// schemaFor derives a tool schema from a contract type. The SDK derives only
// structure (types, required fields, additionalProperties) from Go types.
func schemaFor[T any](types map[reflect.Type]*jsonschema.Schema) *jsonschema.Schema {
	schema, err := jsonschema.For[T](&jsonschema.ForOptions{TypeSchemas: types})
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
	addToolWithTypes(server, h, tool, contractTypes, adjust, run)
}

// addToolWithTypes registers a typed tool whose contract constraints come from
// types. A profile with its own short-ID types passes its merged map.
func addToolWithTypes[In, Out any](
	server *mcp.Server, h *handler, tool *mcp.Tool,
	types map[reflect.Type]*jsonschema.Schema,
	adjust func(input, output *jsonschema.Schema),
	run func(context.Context, *mcp.CallToolRequest, In) (Out, toolRun),
) {
	input, output := schemaFor[In](types), schemaFor[Out](types)
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
			record.targetType, record.targetKey = result.targetType, result.targetKey
			var none Out
			return result.pending, none, nil
		}
		if result.err == nil {
			if encoded, err := json.Marshal(out); err != nil || len(encoded) > maxResultBytes {
				result.err = &toolError{category: "internal", message: "internal failure"}
			}
		}
		record.workspaceID, record.targetID, record.err = result.workspaceID, result.targetID, result.err
		record.targetType, record.targetKey = result.targetType, result.targetKey
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
		"credential_id": actor.TokenShortID(), "client_name": clientName,
		"client_version": clientVersion, "error_category": category,
		"duration_ms": time.Since(started).Milliseconds(),
	}
	eventType, action, targetType, targetKey := eventRequestCalled, method, "", int64(0)
	if method == methodToolsCall {
		eventType, targetType, targetKey = eventToolCalled, record.targetType, record.targetKey
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
		TargetType: targetType, TargetKey: targetKey, Action: action,
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

func principalFromExtra(extra *mcp.RequestExtra) identity.Principal {
	if extra == nil || extra.TokenInfo == nil {
		return identity.Principal{}
	}
	value, _ := extra.TokenInfo.Extra["principal"].(identity.Principal)
	return value
}

func optionalUserShortID(userID *identity.UserID) *userShortID {
	if userID == nil {
		return nil
	}
	short := userShortID(userID.String())
	return &short
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
