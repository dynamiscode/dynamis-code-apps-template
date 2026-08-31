package web

import (
	"context"
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/webhooks"
)

type webhookManager interface {
	SecretKeyConfigured() bool
	List(context.Context, identity.Principal, string) ([]webhooks.Webhook, error)
	ListDeliveries(context.Context, identity.Principal, string, string) ([]webhooks.Delivery, error)
	Create(context.Context, identity.Principal, string, webhooks.CreateInput, identity.AuditContext) (webhooks.NewWebhook, error)
	RotateSecret(context.Context, identity.Principal, string, string, identity.AuditContext) (string, error)
	Delete(context.Context, identity.Principal, string, string, identity.AuditContext) error
}

func (h *Handler) webhooksPage(writer http.ResponseWriter, request *http.Request) {
	if h.webhooks == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	h.renderWebhooksPage(writer, request, "", "")
}

func (h *Handler) webhookDeliveriesPage(writer http.ResponseWriter, request *http.Request) {
	if h.webhooks == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	h.renderWebhooksPage(writer, request, request.PathValue("webhookId"), "")
}

func (h *Handler) renderWebhooksPage(writer http.ResponseWriter, request *http.Request, selectedID, secret string) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.WebhooksRead)
	if !ok {
		return
	}
	h.renderWebhooksPageForPrincipal(writer, request, workspaceID, principal, session, csrf, selectedID, secret)
}

func (h *Handler) renderWebhooksPageForPrincipal(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID string,
	principal identity.Principal,
	session identity.Session,
	csrf, selectedID, secret string,
) {
	h.renderWebhooksPageForPrincipalWithStatus(writer, request, workspaceID, principal, session, csrf, selectedID, secret, "", http.StatusOK)
}

func (h *Handler) renderWebhookMutationError(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID string,
	principal identity.Principal,
	session identity.Session,
	csrf, message string,
) {
	h.renderWebhooksPageForPrincipalWithStatus(writer, request, workspaceID, principal, session, csrf, "", "", message, http.StatusUnprocessableEntity)
}

func (h *Handler) renderWebhooksPageForPrincipalWithStatus(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID string,
	principal identity.Principal,
	session identity.Session,
	csrf, selectedID, secret, message string,
	status int,
) {
	data := pageData{
		Title: "Webhooks", NavPage: "webhooks", NavSection: "settings", Error: message, CSRF: csrf,
		Workspace: workspaceByID(nil, workspaceID), WebhookSecret: secret,
		CanManage: principal.Permissions[identity.WebhooksManage], WebhookSecretKeyConfigured: h.webhooks.SecretKeyConfigured(),
	}
	registered, err := h.webhooks.List(request.Context(), principal, workspaceID)
	if err != nil {
		if secret != "" {
			data.WebhookReadbackFailed = true
			h.render(writer, status, "webhooks.html", data)
			return
		}
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data.Webhooks = registered
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		if secret != "" {
			data.WebhookReadbackFailed = true
			h.render(writer, status, "webhooks.html", data)
			return
		}
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	data.Workspace = workspaceByID(workspaces, workspaceID)
	data.Workspaces = workspaces
	if selectedID != "" {
		for _, webhook := range registered {
			if webhook.ID == selectedID {
				data.SelectedWebhook = webhook
				break
			}
		}
		if data.SelectedWebhook.ID == "" {
			h.renderError(writer, http.StatusNotFound)
			return
		}
		data.WebhookDeliveries, err = h.webhooks.ListDeliveries(request.Context(), principal, workspaceID, selectedID)
		if err != nil {
			if errors.Is(err, webhooks.ErrNotFound) {
				h.renderError(writer, http.StatusNotFound)
				return
			}
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
	}
	h.render(writer, status, "webhooks.html", data)
}

func (h *Handler) webhookMutation(writer http.ResponseWriter, request *http.Request) {
	if h.webhooks == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.managementPrincipal(writer, request, workspaceID, identity.WebhooksManage)
	if !ok {
		return
	}
	action, webhookID := request.FormValue("action"), request.PathValue("webhookId")
	switch action {
	case "create":
		if webhookID != "" {
			h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The requested webhook action is invalid.")
			return
		}
		created, err := h.webhooks.Create(request.Context(), principal, workspaceID, webhooks.CreateInput{
			Name: request.FormValue("name"), URL: request.FormValue("url"), Events: request.Form["events"],
		}, auditContext(request))
		if err != nil {
			switch {
			case errors.Is(err, webhooks.ErrInvalidInput):
				h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The webhook input is invalid.")
			case errors.Is(err, webhooks.ErrLimit):
				h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The workspace webhook limit was reached. Delete one before retrying.")
			case errors.Is(err, webhooks.ErrSecretKey):
				h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "")
			default:
				h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The webhook could not be created.")
			}
			return
		}
		h.renderWebhooksPageForPrincipal(writer, request, workspaceID, principal, session, csrf, "", created.Secret)
	case "rotate":
		if webhookID == "" {
			h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The requested webhook action is invalid.")
			return
		}
		secret, err := h.webhooks.RotateSecret(request.Context(), principal, workspaceID, webhookID, auditContext(request))
		if err != nil {
			switch {
			case errors.Is(err, webhooks.ErrNotFound):
				h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The webhook could not be found.")
			case errors.Is(err, webhooks.ErrSecretKey):
				h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "")
			default:
				h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The webhook could not be rotated.")
			}
			return
		}
		h.renderWebhooksPageForPrincipal(writer, request, workspaceID, principal, session, csrf, "", secret)
	case "delete":
		if webhookID == "" {
			h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The requested webhook action is invalid.")
			return
		}
		if err := h.webhooks.Delete(request.Context(), principal, workspaceID, webhookID, auditContext(request)); err != nil {
			if errors.Is(err, webhooks.ErrNotFound) {
				h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The webhook could not be found.")
				return
			}
			h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The webhook could not be deleted.")
			return
		}
		h.redirect(writer, request, "/workspaces/"+workspaceID+"/settings/webhooks")
	default:
		h.renderWebhookMutationError(writer, request, workspaceID, principal, session, csrf, "The requested webhook action is invalid.")
	}
}
