package web

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
)

// clientFieldRow is a custom field with its choice options. Options are set only
// for single-choice fields.
type clientFieldRow struct {
	clients.Field
	Options []clients.Option
}

// clientValueRow is a field's value with the choices a form offers. ValueLabel is
// the selected choice's label, for read-only pages.
type clientValueRow struct {
	clients.FieldValue
	Choices    []choiceView
	ValueLabel string
}

// choiceView is one choice in a select.
type choiceView struct {
	ID       string
	Label    string
	Selected bool
}

// clientFieldRows attaches each single-choice field's options. The list is read
// with the caller's principal, so it stays inside the caller's workspace.
func (h *Handler) clientFieldRows(
	request *http.Request,
	principal identity.Principal,
	workspaceID identity.WorkspaceID,
	fields []clients.Field,
) ([]clientFieldRow, error) {
	rows := make([]clientFieldRow, 0, len(fields))
	for _, field := range fields {
		row := clientFieldRow{Field: field}
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
) ([]clientValueRow, error) {
	rows := make([]clientValueRow, 0, len(values))
	for _, value := range values {
		row := clientValueRow{FieldValue: value}
		if value.Type == clients.FieldTypeSingleChoice {
			options, err := h.clients.ListOptions(request.Context(), principal, workspaceID, value.FieldID)
			if err != nil {
				return nil, err
			}
			for _, option := range options {
				selected := value.Value != nil && *value.Value == option.ID.String()
				row.Choices = append(row.Choices, choiceView{ID: option.ID.String(), Label: option.Label, Selected: selected})
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
