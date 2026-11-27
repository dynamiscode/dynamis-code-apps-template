package web

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/components/actionattrs"
)

func renderEngagementMarkdown(source string) string {
	rendered, err := actionattrs.MarkdownHTML(source)
	if err != nil {
		return ""
	}
	return rendered
}

func (h *Handler) setEngagementNotes(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	if !h.engagementMatchesClient(w, r, actor, workspace) {
		return
	}
	notes := r.FormValue("notes")
	var revision *string
	if values, ok := r.Form["notes_revision"]; ok {
		value := values[0]
		revision = &value
	}
	err := h.engagement.SetNotes(r.Context(), actor, workspace, engagement.ParseID(r.PathValue("engagementId")), notes, revision, auditContext(r))
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(w, http.StatusForbidden)
	case errors.Is(err, engagement.ErrNotFound):
		h.renderError(w, http.StatusNotFound)
	case errors.Is(err, engagement.ErrNotesConflict):
		h.renderClientDetail(w, r, http.StatusConflict, "engagement.notes_conflict", nil)
	case errors.Is(err, engagement.ErrInvalidInput):
		h.renderClientDetail(w, r, http.StatusUnprocessableEntity, "engagement.invalid_notes", nil)
	case err != nil:
		h.renderError(w, http.StatusInternalServerError)
	default:
		h.redirect(w, r, engagementContextURL(clientContextURL(h.clientsPath(workspace)+"/"+r.PathValue("clientId")+"/engagements/"+r.PathValue("engagementId")+"?saved=1", r.URL.Query().Get("return")), engagementReturnPath(workspace, r.URL.Query().Get("engagement_return"))))
	}
}

func (h *Handler) previewEngagementNotes(w http.ResponseWriter, r *http.Request) {
	if h.engagement == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	if _, _, _, ok := h.workspaceSession(w, r, workspace, identity.ResourcesWrite); !ok {
		return
	}
	if _, ok := h.mutationPrincipal(w, r); !ok {
		return
	}
	notes := r.FormValue("notes")
	if !utf8.ValidString(notes) || utf8.RuneCountInString(notes) > 5000 {
		h.renderError(w, http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(renderEngagementMarkdown(notes)))
}
