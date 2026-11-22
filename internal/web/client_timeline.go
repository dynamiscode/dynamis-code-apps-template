package web

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func timelineOptions(data viewmodel.PageData, kind, selected string, values []string) []atoms.SelectOption {
	options := make([]atoms.SelectOption, 0, len(values))
	found := selected == ""
	for _, value := range values {
		options = append(options, atoms.SelectOption{Value: value, Label: data.T("clients.timeline_" + kind + "_" + value), Selected: value == selected})
		found = found || value == selected
	}
	if !found {
		options = append(options, atoms.SelectOption{Value: selected, Label: selected, Selected: true})
	}
	return options
}

func timelineContains(entries []clients.TimelineEntry, id clients.TimelineEntryID) bool {
	for _, entry := range entries {
		if entry.ID == id {
			return true
		}
	}
	return false
}

func (h *Handler) addClientTimelineEntry(w http.ResponseWriter, r *http.Request) {
	if h.clients == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	clientID := clients.ParseClientIDOrZero(r.PathValue("clientId"))
	if _, _, _, ok := h.workspaceSession(w, r, workspaceID, identity.ResourcesWrite); !ok {
		return
	}
	draft := viewmodel.TimelineDraft{Type: r.FormValue("type"), Channel: r.FormValue("channel"), Text: r.FormValue("text"), Corrects: r.FormValue("corrects")}
	input := clients.TimelineInput{Type: draft.Type, Channel: draft.Channel, Text: draft.Text}
	var err error
	if draft.Corrects != "" {
		var id clients.TimelineEntryID
		id, err = clients.ParseTimelineEntryID(draft.Corrects)
		if err == nil {
			input.CorrectsID = &id
		} else {
			err = clients.ErrTimelineCorrection
		}
	}
	if err == nil {
		_, err = h.clients.AddTimelineEntry(r.Context(), principal, workspaceID, clientID, input, auditContext(r))
	}
	if err != nil {
		key := ""
		switch {
		case errors.Is(err, clients.ErrTimelineTextRequired):
			key = "clients.timeline_error_required"
		case errors.Is(err, clients.ErrTimelineTextTooLong):
			key = "clients.timeline_error_long"
		case errors.Is(err, clients.ErrTimelineType):
			key = "clients.timeline_error_type"
		case errors.Is(err, clients.ErrTimelineChannel):
			key = "clients.timeline_error_channel"
		case errors.Is(err, clients.ErrTimelineCorrection):
			key = "clients.timeline_error_correction"
		case errors.Is(err, clients.ErrInvalidInput):
			key = "clients.timeline_error_invalid"
		}
		if key != "" {
			h.renderClientDetailWithTimeline(w, r, http.StatusUnprocessableEntity, key, nil, draft)
		} else {
			h.clientError(w, r, err)
		}
		return
	}
	h.redirect(w, r, h.clientsPath(workspaceID)+"/"+clientID.String())
}
