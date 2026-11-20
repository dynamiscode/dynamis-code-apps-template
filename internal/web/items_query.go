package web

import (
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/url"
	"strconv"
	"strings"
)

// itemFilterFields are the fields the items list can filter on. The item service takes one title and
// one status, so each field can appear once.
var itemFilterFields = []listFilterField{
	{
		Key:      "status",
		LabelKey: "items.status",
		Ops:      []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}},
		Choices: []listFilterChoice{
			{Value: string(items.Active), LabelKey: "items.active"},
			{Value: string(items.Complete), LabelKey: "items.complete"},
		},
	},
}

// itemListParams are the query keys the items list accepts. The search and status keys are the links
// from before the shared filters; they still work, as conditions.
var itemListParams = map[string]bool{
	"filter": true, "match": true, "filter_field": true, "filter_op": true, "filter_value": true, "filter_value_to": true,
	"filter_error": true, listPartKey: true, listFullKey: true, "q": true, "search": true, "status": true, "sort": true, "limit": true, "page": true,
}

func parseItemListQuery(query url.Values, defaultLimit int) (viewmodel.ItemListQuery, bool) {
	for key, entries := range query {
		if !itemListParams[key] || len(entries) != 1 && key != "filter" && key != "filter_value" {
			return viewmodel.ItemListQuery{}, false
		}
	}
	input, ok := parseListFilters(query, itemFilterFields)
	if !ok {
		return viewmodel.ItemListQuery{}, false
	}
	// The search box sends q; the older search link still works. Search always combines with AND.
	search := strings.TrimSpace(query.Get("q"))
	if search == "" {
		search = strings.TrimSpace(query.Get("search"))
	}
	if len(search) > 100 {
		return viewmodel.ItemListQuery{}, false
	}
	if status := query.Get("status"); status != "" {
		field, _ := listFieldByKey(itemFilterFields, "status")
		if !listChoiceValueValid(field, status) {
			return viewmodel.ItemListQuery{}, false
		}
		input.Filters = append(input.Filters, viewmodel.ListFilter{Field: "status", Op: "equals", Value: status})
	}
	form := viewmodel.ItemListQuery{Sort: query.Get("sort"), Match: input.Match, Filter: input, Search: search}
	seen := map[string]bool{}
	for _, filter := range input.Filters {
		if seen[filter.Field] || strings.Contains(filter.Value, ",") && filter.Field == "status" {
			return viewmodel.ItemListQuery{}, false
		}
		seen[filter.Field] = true
		if filter.Field == "status" {
			form.Status = filter.Value
		}
	}
	if form.Sort == "" {
		form.Sort = "-created_at"
	}
	if form.Sort != "created_at" && form.Sort != "-created_at" {
		return viewmodel.ItemListQuery{}, false
	}
	form.Limit = defaultLimit
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return viewmodel.ItemListQuery{}, false
		}
		form.Limit = limit
	}
	return form, true
}
