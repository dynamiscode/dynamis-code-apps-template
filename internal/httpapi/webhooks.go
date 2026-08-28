package httpapi

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/webhooks"
)

type createWebhookRequest struct {
	Name   string   `json:"name"`
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

func (h *handler) listWebhooks(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WebhooksRead)
	if !ok {
		return
	}
	if h.webhooks == nil {
		h.internal(writer, request)
		return
	}
	result, err := h.webhooks.List(request.Context(), principal, workspaceID)
	if err != nil {
		h.webhookProblem(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"webhooks": result})
}

func (h *handler) createWebhook(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WebhooksManage)
	if !ok {
		return
	}
	var input createWebhookRequest
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	if h.webhooks == nil {
		h.internal(writer, request)
		return
	}
	created, err := h.webhooks.Create(request.Context(), principal, workspaceID, webhooks.CreateInput{
		Name: input.Name, URL: input.URL, Events: input.Events,
	}, h.auditContext(request))
	if err != nil {
		h.webhookProblem(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusCreated, created)
}

func (h *handler) deleteWebhook(writer http.ResponseWriter, request *http.Request) {
	workspaceID, webhookID := request.PathValue("workspaceId"), request.PathValue("webhookId")
	if !validID(workspaceID) || !validID(webhookID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WebhooksManage)
	if !ok {
		return
	}
	if h.webhooks == nil {
		h.internal(writer, request)
		return
	}
	if err := h.webhooks.Delete(request.Context(), principal, workspaceID, webhookID, h.auditContext(request)); err != nil {
		h.webhookProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) rotateWebhookSecret(writer http.ResponseWriter, request *http.Request) {
	workspaceID, webhookID := request.PathValue("workspaceId"), request.PathValue("webhookId")
	if !validID(workspaceID) || !validID(webhookID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WebhooksManage)
	if !ok {
		return
	}
	if h.webhooks == nil {
		h.internal(writer, request)
		return
	}
	secret, err := h.webhooks.RotateSecret(request.Context(), principal, workspaceID, webhookID, h.auditContext(request))
	if err != nil {
		h.webhookProblem(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusOK, map[string]string{"secret": secret})
}

func (h *handler) listWebhookDeliveries(writer http.ResponseWriter, request *http.Request) {
	workspaceID, webhookID := request.PathValue("workspaceId"), request.PathValue("webhookId")
	if !validID(workspaceID) || !validID(webhookID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WebhooksRead)
	if !ok {
		return
	}
	if h.webhooks == nil {
		h.internal(writer, request)
		return
	}
	result, err := h.webhooks.ListDeliveries(request.Context(), principal, workspaceID, webhookID)
	if err != nil {
		h.webhookProblem(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"deliveries": result})
}

func (h *handler) webhookProblem(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, webhooks.ErrInvalidInput):
		h.invalidRequest(writer, request, "The webhook input is invalid.")
	case errors.Is(err, webhooks.ErrNotFound):
		notFoundProblem(writer, request)
	case errors.Is(err, webhooks.ErrLimit):
		writeProblem(writer, request, http.StatusConflict, "resource-limit", "The workspace webhook limit was reached.")
	case errors.Is(err, webhooks.ErrSecretKey):
		writeProblem(writer, request, http.StatusServiceUnavailable, "webhook-secret-key-missing", "Webhook secret encryption is not configured.")
	default:
		h.internal(writer, request)
	}
}
