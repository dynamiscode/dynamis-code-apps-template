package web

import (
	"encoding/json"
	"errors"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"fmt"
	"net/http"
)

// clientEvents streams client list refreshes for one workspace. It shares the
// Items connection rules: read permission, the stream limits, and resync when a
// cursor cannot be replayed.
func (h *Handler) clientEvents(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, _, _, ok := h.workspaceSession(
		writer, request, workspaceID, identity.ResourcesRead,
	)
	if !ok {
		return
	}
	h.serveChangeStream(writer, request, principal.UserID, func(cursor string) (string, bool) {
		return h.sendClientChanges(writer, request, principal, workspaceID, cursor)
	})
}

// sendClientChanges writes the client changes after cursor as client.changed
// events, or a resync when the cursor is unknown. It returns the next cursor.
func (h *Handler) sendClientChanges(
	writer http.ResponseWriter,
	request *http.Request,
	principal identity.Principal,
	workspaceID identity.WorkspaceID,
	cursor string,
) (nextCursor string, ok bool) {
	page, err := h.clients.Changes(request.Context(), principal, workspaceID, cursor, 100)
	if errors.Is(err, clients.ErrInvalidChangeID) {
		// A malformed cursor is unknown to the feed: resync rather than fail the stream.
		page, err = h.clients.Changes(request.Context(), principal, workspaceID, "", 100)
	}
	if err != nil {
		return cursor, false
	}
	if page.Resync {
		if page.Next != "" {
			_, _ = fmt.Fprintf(writer, "id: %s\n", page.Next)
		}
		_, _ = fmt.Fprint(writer, "event: resync\ndata: {\"reason\":\"cursor\"}\n\n")
		return page.Next, true
	}
	nextCursor = cursor
	for _, change := range page.Changes {
		encoded, err := json.Marshal(change)
		if err != nil {
			return nextCursor, false
		}
		_, _ = fmt.Fprintf(
			writer, "id: %s\nevent: client.changed\ndata: %s\n\n",
			change.ID, encoded,
		)
		nextCursor = change.ID.String()
	}
	return nextCursor, true
}
