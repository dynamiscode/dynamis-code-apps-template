package web

import (
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// clientQueryFromValues parses a Clients list query. choices holds the options of each choice field.
func clientQueryFromValues(query url.Values, fields []clients.Field, choices map[string][]listFilterChoice) (viewmodel.ClientQueryForm, bool) {
	form := viewmodel.ClientQueryForm{
		Sort: query.Get("sort"), Order: query.Get("order"),
		Archived: query.Get("archived"),
	}
	for _, key := range []string{"sort", "order", "archived", "limit", "page"} {
		if len(query[key]) > 1 {
			return form, false
		}
	}
	for key := range query {
		switch key {
		case "sort", "order", "archived", "limit", "page", "q", "filter", "filter_field", "filter_op",
			"filter_value", "filter_value_to", "filter_error", "match", listPartKey, listFullKey:
		default:
			return form, false
		}
	}
	if form.Order != "" && form.Order != "asc" && form.Order != "desc" || form.Order == "desc" && form.Sort == "" {
		return form, false
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return form, false
		}
		form.Limit = limit
	}
	input, ok := parseListFilters(query, clientListFilterFields(fields, choices))
	if !ok {
		return form, false
	}
	form.Filter = input
	form.Search = strings.TrimSpace(query.Get("q"))
	if len(form.Search) > 200 {
		return form, false
	}
	form.Match = input.Match
	form.Filters = toClientFilterInputs(input.Filters)
	return form, true
}

func toClientFilterInputs(filters []viewmodel.ListFilter) []clients.FilterInput {
	out := make([]clients.FilterInput, 0, len(filters))
	for _, filter := range filters {
		out = append(out, clients.FilterInput{Field: filter.Field, Op: filter.Op, Value: filter.Value})
	}
	return out
}

// clientListFilterFields describes the Clients fields for the shared filter model: the built-in fields,
// then each custom field with its operators, its input type, and its choices.
func clientListFilterFields(fields []clients.Field, choices map[string][]listFilterChoice) []listFilterField {
	builtins := []struct{ key, labelKey, kind string }{
		{clients.FieldDisplayName, "clients.display_name", "text"},
		{clients.FieldEmail, "common.email", "text"},
		{clients.FieldPhone, "clients.phone", "text"},
		{clients.FieldNotes, "clients.notes", "text"},
		{clients.FieldCreatedAt, "clients.created_at", "date"},
		{clients.FieldUpdatedAt, "clients.updated_at", "date"},
	}
	out := make([]listFilterField, 0, len(builtins)+len(fields))
	for _, builtin := range builtins {
		out = append(out, listFilterField{
			Key: builtin.key, LabelKey: builtin.labelKey, Ops: clientListOps(builtin.key, fields), InputType: builtin.kind,
		})
	}
	for _, field := range fields {
		id := field.ID.String()
		out = append(out, listFilterField{
			Key: id, Label: field.Label, Ops: clientListOps(id, fields),
			Choices: choices[id], InputType: clientInputKind(field.Type),
		})
	}
	return out
}

// clientListOps converts the Clients operators of a field to the shared operator list.
func clientListOps(field string, fields []clients.Field) []listFilterOp {
	ops := clientFilterOperators(field, fields)
	out := make([]listFilterOp, 0, len(ops))
	for _, op := range ops {
		out = append(out, listFilterOp{Key: op.Key, LabelKey: op.LabelKey})
	}
	return out
}

func clientInputKind(fieldType string) string {
	switch fieldType {
	case clients.FieldTypeNumber:
		return "number"
	case clients.FieldTypeDate:
		return "date"
	}
	return "text"
}

// clientFilterChoices loads the choices of every choice field: the options of single-choice fields, and
// yes and no for yes-or-no fields.
func (h *Handler) clientFilterChoices(
	request *http.Request,
	principal identity.Principal,
	workspaceID identity.WorkspaceID,
	fields []clients.Field,
) (map[string][]listFilterChoice, error) {
	choices := make(map[string][]listFilterChoice)
	for _, field := range fields {
		id := field.ID.String()
		switch field.Type {
		case clients.FieldTypeSingleChoice:
			options, err := h.clients.ListOptions(request.Context(), principal, workspaceID, field.ID)
			if err != nil {
				return nil, err
			}
			for _, option := range options {
				choices[id] = append(choices[id], listFilterChoice{Value: option.ID.String(), Label: option.Label})
			}
		case clients.FieldTypeYesNo:
			choices[id] = []listFilterChoice{
				{Value: "true", LabelKey: "clients.yes"},
				{Value: "false", LabelKey: "clients.no"},
			}
		}
	}
	return choices, nil
}
