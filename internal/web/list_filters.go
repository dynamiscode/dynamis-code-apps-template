package web

import (
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/url"
	"strings"
	"unicode/utf8"
)

// The shared list filter model. A list declares its fields, and this file parses the query, matches
// rows, and builds the chips and the two-step add menu. Each list keeps its own matching of a row.

// listRangeValid reports whether a range value is "from,to" with both sides set.
func listRangeValid(value string) bool {
	parts := strings.SplitN(value, ",", 2)
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

// listFilterOp is one way a field can be compared.
type listFilterOp struct {
	Key      string
	LabelKey string
}

// listFilterChoice is one allowed value of a choice field. A built-in choice has a translation key; a
// custom one has a plain label.
type listFilterChoice struct {
	Value    string
	LabelKey string
	Label    string
}

// listFilterField is a field a list can filter on. A field with choices takes one or more of them.
// InputType is the browser input for a value: "text", "number", or "date".
type listFilterField struct {
	Key       string
	LabelKey  string
	Label     string
	Ops       []listFilterOp
	Choices   []listFilterChoice
	InputType string
}

func listFieldByKey(fields []listFilterField, key string) (listFilterField, bool) {
	for _, field := range fields {
		if field.Key == key {
			return field, true
		}
	}
	return listFilterField{}, false
}

func listFieldHasOp(field listFilterField, op string) bool {
	for _, candidate := range field.Ops {
		if candidate.Key == op {
			return true
		}
	}
	return false
}

func listFieldHasChoice(field listFilterField, value string) bool {
	for _, choice := range field.Choices {
		if choice.Value == value {
			return true
		}
	}
	return false
}

// listChoiceValueValid reports whether a value is valid for a field. Choice fields take one or more
// comma-separated choices, and other fields take any non-empty text.
func listChoiceValueValid(field listFilterField, value string) bool {
	if len(field.Choices) == 0 {
		return value != ""
	}
	for _, part := range strings.Split(value, ",") {
		if !listFieldHasChoice(field, part) {
			return false
		}
	}
	return true
}

// parseListFilters reads the filter, match, and add-step parameters. It refuses an unknown field,
// operator, or choice, and more than the allowed number of conditions. An add step with a value is
// applied; one without a value stays open so the user can pick its value.
func parseListFilters(values url.Values, fields []listFilterField) (viewmodel.ListFilterInput, bool) {
	input := viewmodel.ListFilterInput{Match: values.Get("match")}
	if len(values["match"]) > 1 || input.Match != "" && input.Match != viewmodel.ListMatchAll && input.Match != viewmodel.ListMatchAny {
		return input, false
	}
	if input.Match == "" {
		input.Match = viewmodel.ListMatchAll
	}
	for _, raw := range values["filter"] {
		parts := strings.SplitN(raw, ":", 3)
		if len(parts) != 3 || utf8.RuneCountInString(raw) > 1024 {
			return input, false
		}
		field, ok := listFieldByKey(fields, parts[0])
		if !ok || !listFieldHasOp(field, parts[1]) {
			return input, false
		}
		if parts[1] != viewmodel.ListOpIsEmpty && !listChoiceValueValid(field, parts[2]) {
			return input, false
		}
		if parts[1] == viewmodel.ListOpBetween && !listRangeValid(parts[2]) {
			return input, false
		}
		input.Filters = append(input.Filters, viewmodel.ListFilter{Field: parts[0], Op: parts[1], Value: parts[2]})
	}
	if len(input.Filters) > viewmodel.ListFilterMaxConditions {
		return input, false
	}
	addKey := values.Get("filter_field")
	input.AddOpen = addKey == "" && values.Has("filter_field")
	if addKey == "" {
		return input, true
	}
	field, ok := listFieldByKey(fields, addKey)
	if !ok || len(field.Ops) == 0 {
		return input, false
	}
	op := values.Get("filter_op")
	if op == "" {
		op = field.Ops[0].Key
	}
	if !listFieldHasOp(field, op) {
		return input, false
	}
	if op == viewmodel.ListOpBetween {
		from, to := values.Get("filter_value"), values.Get("filter_value_to")
		if from != "" && to != "" {
			if len(input.Filters) >= viewmodel.ListFilterMaxConditions || !listRangeValid(from+","+to) {
				return input, false
			}
			input.Filters = append(input.Filters, viewmodel.ListFilter{Field: addKey, Op: op, Value: from + "," + to})
			return input, true
		}
		input.AddField, input.AddOp, input.AddValues, input.AddValueTo = addKey, op, []string{from}, to
		return input, true
	}
	addValues := values["filter_value"]
	if op == viewmodel.ListOpIsEmpty || len(addValues) > 0 && addValues[0] != "" {
		value := ""
		if op != viewmodel.ListOpIsEmpty {
			value = strings.Join(addValues, ",")
			if !listChoiceValueValid(field, value) {
				return input, false
			}
		}
		if len(input.Filters) >= viewmodel.ListFilterMaxConditions {
			return input, false
		}
		input.Filters = append(input.Filters, viewmodel.ListFilter{Field: addKey, Op: op, Value: value})
		return input, true
	}
	input.AddField, input.AddOp, input.AddValues = addKey, op, addValues
	return input, true
}

// matchListFilters reports whether a row meets the conditions. With match all, every condition must hold;
// with match any, one must. A list without conditions shows every row.
func matchListFilters(filters []viewmodel.ListFilter, match string, holds func(viewmodel.ListFilter) bool) bool {
	if len(filters) == 0 {
		return true
	}
	if match == viewmodel.ListMatchAny {
		for _, filter := range filters {
			if holds(filter) {
				return true
			}
		}
		return false
	}
	for _, filter := range filters {
		if !holds(filter) {
			return false
		}
	}
	return true
}

// listQueryForAdd is the list query without the page and the add-step inputs. Links that leave the
// add step keep the sort, page size, filters, and match mode.
func listQueryForAdd(query url.Values) url.Values {
	next := url.Values{}
	for key, values := range query {
		switch key {
		case "page", "filter_field", "filter_op", "filter_value", "filter_value_to", "filter_error", listPartKey, listFullKey:
			continue
		}
		next[key] = values
	}
	return next
}

// listFilterQuery returns the query with the given conditions and match mode. The add step is left out.
func listFilterQuery(query url.Values, filters []viewmodel.ListFilter, match string) url.Values {
	next := listQueryForAdd(query)
	next.Del("filter")
	next.Del("match")
	if len(filters) > 0 {
		parts := make([]string, 0, len(filters))
		for _, filter := range filters {
			parts = append(parts, filter.Field+":"+filter.Op+":"+filter.Value)
		}
		next["filter"] = parts
		next.Set("match", match)
	}
	return next
}

// buildListFilterView turns a parsed query into the chips, the field links, and the open add step.
func buildListFilterView(path string, query url.Values, fields []listFilterField, input viewmodel.ListFilterInput) viewmodel.ListFilterView {
	base := listFilterQuery(query, input.Filters, input.Match)
	view := viewmodel.ListFilterView{
		Path:      path,
		Preserved: clientPreservedQuery(base),
		Match:     input.Match,
		ShowMatch: len(input.Filters) > 1,
		CanAdd:    len(input.Filters) < viewmodel.ListFilterMaxConditions,
		AddOpen:   input.AddOpen || input.AddField != "",
	}
	for index, filter := range input.Filters {
		remaining := make([]viewmodel.ListFilter, 0, len(input.Filters)-1)
		remaining = append(remaining, input.Filters[:index]...)
		remaining = append(remaining, input.Filters[index+1:]...)
		field, _ := listFieldByKey(fields, filter.Field)
		chip := viewmodel.ListFilterChip{
			FieldLabelKey: field.LabelKey, FieldLabel: field.Label,
			OpLabelKey: listOpLabelKey(field, filter.Op), Value: filter.Value,
		}
		if filter.Op == viewmodel.ListOpBetween {
			chip.Value = strings.Replace(filter.Value, ",", " and ", 1)
		} else if len(field.Choices) > 0 && filter.Op != viewmodel.ListOpIsEmpty {
			chip.ValueKey, chip.ValueLabel = listChoiceLabel(field, filter.Value)
		}
		chip.RemoveHref = path + "?" + listFilterQuery(query, remaining, input.Match).Encode()
		chip.EditHref = path + "?" + listEditQuery(query, remaining, input.Match, filter).Encode()
		view.Chips = append(view.Chips, chip)
	}
	for _, field := range fields {
		next := cloneClientQuery(base)
		next.Set("filter_field", field.Key)
		view.Fields = append(view.Fields, viewmodel.ListFieldLink{LabelKey: field.LabelKey, Label: field.Label, Href: path + "?" + next.Encode(), Step: listFilterStep(field, viewmodel.ListFilterInput{})})
	}
	backQuery := cloneClientQuery(base)
	backQuery.Set("filter_field", "")
	view.BackHref = path + "?" + backQuery.Encode()
	clear := listFilterQuery(query, nil, input.Match)
	view.ClearHref = path
	if encoded := clear.Encode(); encoded != "" {
		view.ClearHref += "?" + encoded
	}
	if input.AddField != "" {
		field, _ := listFieldByKey(fields, input.AddField)
		step := listFilterStep(field, input)
		view.Step = &step
	}
	return view
}

func listFilterStep(field listFilterField, input viewmodel.ListFilterInput) viewmodel.ListFilterStep {
	step := viewmodel.ListFilterStep{
		FieldKey: field.Key, FieldLabelKey: field.LabelKey, FieldLabel: field.Label,
		IsChoice: len(field.Choices) > 0, InputType: field.InputType,
		Range: listFieldHasOp(field, viewmodel.ListOpBetween), ValueTo: input.AddValueTo,
	}
	if len(input.AddValues) > 0 && !step.IsChoice {
		step.Value = input.AddValues[0]
	}
	for index, op := range field.Ops {
		step.Ops = append(step.Ops, viewmodel.ListOpOption{Key: op.Key, LabelKey: op.LabelKey, Checked: op.Key == input.AddOp || input.AddOp == "" && index == 0})
	}
	for _, choice := range field.Choices {
		selected := false
		for _, value := range input.AddValues {
			selected = selected || value == choice.Value
		}
		step.Choices = append(step.Choices, viewmodel.ListChoiceOption{Value: choice.Value, LabelKey: choice.LabelKey, Label: choice.Label, Selected: selected})
	}
	return step
}

// listEditQuery reopens one condition in the add step. The condition is left out of the list, so
// applying the step adds it back with the changed values.
func listEditQuery(query url.Values, remaining []viewmodel.ListFilter, match string, filter viewmodel.ListFilter) url.Values {
	next := listFilterQuery(query, remaining, match)
	next.Set("filter_field", filter.Field)
	next.Set("filter_op", filter.Op)
	switch {
	case filter.Op == viewmodel.ListOpIsEmpty:
	case filter.Op == viewmodel.ListOpBetween:
		from, to, _ := strings.Cut(filter.Value, ",")
		next.Set("filter_value", from)
		next.Set("filter_value_to", to)
	default:
		for _, value := range strings.Split(filter.Value, ",") {
			next.Add("filter_value", value)
		}
	}
	return next
}

func listOpLabelKey(field listFilterField, op string) string {
	for _, candidate := range field.Ops {
		if candidate.Key == op {
			return candidate.LabelKey
		}
	}
	return ""
}

// listChoiceLabel returns the label of a single choice, as a translation key or as plain text.
func listChoiceLabel(field listFilterField, value string) (string, string) {
	for _, choice := range field.Choices {
		if choice.Value == value {
			return choice.LabelKey, choice.Label
		}
	}
	return "", value
}
