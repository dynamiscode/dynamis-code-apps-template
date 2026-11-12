package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// itemReviewPromptName is the only prompt. It exists so completion has a real
// prompt ref to complete against; MCP completion cannot target tool arguments.
const itemReviewPromptName = "item_review_v1"

// ponytail: completion scans only the newest 100 items of a workspace (one
// list page). Add cursor paging, or an exact-ID lookup, when workspaces outgrow it.
const completionScanLimit = 100

var itemStatuses = []string{string(items.Active), string(items.Complete)}

func (h *handler) addPrompts(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name:        itemReviewPromptName,
		Description: "Prepare a review of one item before changing its status.",
		Arguments: []*mcp.PromptArgument{
			{Name: "workspaceId", Description: "Workspace that owns the item.", Required: true},
			{Name: "itemId", Description: "Item to review in that workspace.", Required: true},
			{Name: "status", Description: "Proposed status: active or complete.", Required: true},
		},
	}, h.itemReviewPrompt)
}

func (h *handler) itemReviewPrompt(ctx context.Context, request *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	arguments := request.Params.Arguments
	workspaceID, itemID, status := identity.ParseWorkspaceIDOrZero(arguments["workspaceId"]), arguments["itemId"], arguments["status"]
	if workspaceID == 0 || !validID(itemID) || !isItemStatus(status) {
		return nil, invalidParameters()
	}
	item, err := h.items.Get(ctx, principalFromExtra(request.Extra), workspaceID, itemID)
	if err != nil {
		return nil, publicError(err)
	}
	text := fmt.Sprintf(
		"Item %s in workspace %s is %s at version %d. Confirm whether it should become %s, "+
			"then call items_update_v1 with that version.",
		item.ID, item.WorkspaceID, item.Status, item.Version, status,
	)
	return &mcp.GetPromptResult{
		Description: "Item status review",
		Messages: []*mcp.PromptMessage{{
			Role: mcp.Role("user"), Content: &mcp.TextContent{Text: text},
		}},
	}, nil
}

func (h *handler) complete(ctx context.Context, request *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	ref := request.Params.Ref
	if ref == nil || ref.Type != "ref/prompt" || ref.Name != itemReviewPromptName {
		return completeValues([]string{}, false), nil
	}
	argument := request.Params.Argument
	switch argument.Name {
	case "status":
		return completeValues(prefixMatches(itemStatuses, argument.Value), false), nil
	case "itemId":
		values, hasMore, err := h.completeItemIDs(ctx, request)
		if err != nil {
			return nil, err
		}
		return completeValues(values, hasMore), nil
	default:
		return completeValues([]string{}, false), nil
	}
}

// completeItemIDs returns item IDs from one bounded page of the workspace the
// caller names. Unauthorized or malformed workspaces yield no values, so the
// response never distinguishes missing from forbidden workspaces.
func (h *handler) completeItemIDs(ctx context.Context, request *mcp.CompleteRequest) ([]string, bool, error) {
	empty := []string{}
	var scope map[string]string
	if request.Params.Context != nil {
		scope = request.Params.Context.Arguments
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(scope["workspaceId"])
	if workspaceID == 0 {
		return empty, false, nil
	}
	input := items.ListInput{Sort: "-created_at", Limit: completionScanLimit}
	if status := scope["status"]; isItemStatus(status) {
		input.Status = items.Status(status)
	}
	page, err := h.items.List(ctx, principalFromExtra(request.Extra), workspaceID, input)
	if errors.Is(err, identity.ErrForbidden) {
		return empty, false, nil
	}
	if err != nil {
		return nil, false, publicError(err)
	}
	ids := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		if strings.HasPrefix(item.ID, request.Params.Argument.Value) {
			ids = append(ids, item.ID)
		}
	}
	return ids, page.NextCursor != "", nil
}

func completeValues(values []string, hasMore bool) *mcp.CompleteResult {
	details := mcp.CompletionResultDetails{Values: values, HasMore: hasMore}
	if !hasMore {
		details.Total = len(values)
	}
	return &mcp.CompleteResult{Completion: details}
}

func prefixMatches(options []string, prefix string) []string {
	matches := []string{}
	for _, option := range options {
		if strings.HasPrefix(option, prefix) {
			matches = append(matches, option)
		}
	}
	return matches
}

func isItemStatus(value string) bool {
	return value == string(items.Active) || value == string(items.Complete)
}

func principalFromExtra(extra *mcp.RequestExtra) identity.Principal {
	if extra == nil || extra.TokenInfo == nil {
		return identity.Principal{}
	}
	value, _ := extra.TokenInfo.Extra["principal"].(identity.Principal)
	return value
}
