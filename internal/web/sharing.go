package web

import (
	"errors"
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/sharing"
)

func (h *Handler) shareMutation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	itemID := request.PathValue("itemId")
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	switch request.FormValue("action") {
	case "create":
		lifetime, ok := shareFormLifetime(request.FormValue("lifetime"))
		if !ok {
			h.renderItems(writer, request, http.StatusUnprocessableEntity, "The public link expiration is invalid.")
			return
		}
		link, err := h.sharing.Create(request.Context(), principal, workspaceID, itemID, lifetime, auditContext(request))
		if err != nil {
			if errors.Is(err, sharing.ErrNotFound) {
				h.renderError(writer, http.StatusNotFound)
				return
			}
			h.renderItems(writer, request, http.StatusUnprocessableEntity, "The public link could not be created.")
			return
		}
		h.renderItems(writer, request, http.StatusOK, "", publicShareURL(h.publicURL, link.Token))
	case "revoke":
		if err := h.sharing.Revoke(request.Context(), principal, workspaceID, itemID, request.FormValue("link_id"), auditContext(request)); err != nil {
			h.renderItems(writer, request, http.StatusConflict, "The public link could not be revoked.")
			return
		}
		h.afterMutation(writer, request)
	default:
		h.renderItems(writer, request, http.StatusBadRequest, "The public link action is invalid.")
	}
}

func shareFormLifetime(value string) (time.Duration, bool) {
	switch value {
	case "", "7":
		return sharing.DefaultLifetime, true
	case "30":
		return sharing.MaximumLifetime, true
	default:
		return 0, false
	}
}

func publicShareURL(publicURL, token string) string {
	path := "/share/" + token
	if publicURL == "" {
		return path
	}
	return trimRightSlash(publicURL) + path
}

func trimRightSlash(value string) string {
	for len(value) > 0 && value[len(value)-1] == '/' {
		value = value[:len(value)-1]
	}
	return value
}
