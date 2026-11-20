package web

import (
	"errors"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
)

// clientFieldRows attaches each single-choice field's options. The list is read
// with the caller's principal, so it stays inside the caller's workspace.
func (h *Handler) clientFieldRows(
	request *http.Request,
	principal identity.Principal,
	workspaceID identity.WorkspaceID,
	fields []clients.Field,
) ([]viewmodel.ClientFieldRow, error) {
	rows := make([]viewmodel.ClientFieldRow, 0, len(fields))
	for _, field := range fields {
		row := viewmodel.ClientFieldRow{Field: field}
		if field.Type == clients.FieldTypeSingleChoice {
			options, err := h.clients.ListOptions(request.Context(), principal, workspaceID, field.ID)
			if err != nil {
				return nil, err
			}
			row.Options = options
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// clientValueRows attaches each single-choice value's choices and selected label.
func (h *Handler) clientValueRows(
	request *http.Request,
	principal identity.Principal,
	workspaceID identity.WorkspaceID,
	values []clients.FieldValue,
) ([]viewmodel.ClientValueRow, error) {
	rows := make([]viewmodel.ClientValueRow, 0, len(values))
	for _, value := range values {
		row := viewmodel.ClientValueRow{FieldValue: value}
		if value.Type == clients.FieldTypeSingleChoice {
			options, err := h.clients.ListOptions(request.Context(), principal, workspaceID, value.FieldID)
			if err != nil {
				return nil, err
			}
			for _, option := range options {
				selected := value.Value != nil && *value.Value == option.ID.String()
				row.Choices = append(row.Choices, viewmodel.ChoiceView{ID: option.ID.String(), Label: option.Label, Selected: selected})
				if selected {
					row.ValueLabel = option.Label
				}
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (h *Handler) createClientFieldOption(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	_, err := h.clients.AddOption(request.Context(), principal, workspaceID, fieldID,
		request.FormValue("label"), auditContext(request))
	switch {
	case err == nil:
		h.redirect(writer, request, h.clientFieldsPath(workspaceID))
	case errors.Is(err, clients.ErrOptionLimit):
		h.renderClientFieldList(writer, request, http.StatusConflict, "clients.error_option_limit", clients.FieldInput{})
	case errors.Is(err, clients.ErrFieldArchived):
		h.renderClientFieldList(writer, request, http.StatusConflict, "clients.error_field_archived", clients.FieldInput{})
	case errors.Is(err, clients.ErrNotChoiceField):
		h.renderClientFieldList(writer, request, http.StatusUnprocessableEntity, "clients.error_not_choice", clients.FieldInput{})
	case errors.Is(err, clients.ErrInvalidInput):
		h.renderClientFieldList(writer, request, http.StatusUnprocessableEntity, "clients.error_option_label", clients.FieldInput{})
	default:
		h.clientError(writer, request, err)
	}
}

func (h *Handler) deleteClientFieldOption(writer http.ResponseWriter, request *http.Request) {
	if h.clients == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	fieldID := clients.ParseFieldIDOrZero(request.PathValue("fieldId"))
	optionID := clients.ParseOptionIDOrZero(request.PathValue("optionId"))
	if err := h.clients.DeleteOption(request.Context(), principal, workspaceID, fieldID, optionID, auditContext(request)); err != nil {
		h.clientError(writer, request, err)
		return
	}
	h.redirect(writer, request, h.clientFieldsPath(workspaceID))
}
