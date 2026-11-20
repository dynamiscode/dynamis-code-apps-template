package web

import (
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/url"
	"strconv"
	"strings"
)

// memberFilterFields are the fields the members list can filter on.
var memberFilterFields = []listFilterField{
	{
		Key:      "email",
		LabelKey: "common.email",
		Ops: []listFilterOp{
			{Key: "contains", LabelKey: "clients.op_contains"},
			{Key: "equals", LabelKey: "clients.op_equals"},
		},
	},
	{
		Key:      "role",
		LabelKey: "members.role",
		Ops:      []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}},
		Choices: []listFilterChoice{
			{Value: "owner", LabelKey: "members.owner"},
			{Value: "admin", LabelKey: "members.admin"},
			{Value: "member", LabelKey: "members.member"},
			{Value: "viewer", LabelKey: "members.viewer"},
		},
	},
}

// memberListParams are the query keys the members list accepts. The email and role keys are the
// links from before the shared filters; they still work, as conditions.
var memberListParams = map[string]bool{
	"filter": true, "match": true, "filter_field": true, "filter_op": true, "filter_value": true, "filter_value_to": true,
	"filter_error": true, listPartKey: true, listFullKey: true, "q": true, "email": true, "role": true, "sort": true, "limit": true, "page": true,
}

func parseMemberListQuery(values url.Values, defaultLimit int) (viewmodel.MemberListQuery, bool) {
	for key, entries := range values {
		if !memberListParams[key] {
			return viewmodel.MemberListQuery{}, false
		}
		if len(entries) != 1 && key != "filter" && key != "filter_value" {
			return viewmodel.MemberListQuery{}, false
		}
	}
	filter, ok := parseListFilters(values, memberFilterFields)
	if !ok {
		return viewmodel.MemberListQuery{}, false
	}
	if email := strings.TrimSpace(values.Get("email")); email != "" {
		if len(email) > 320 {
			return viewmodel.MemberListQuery{}, false
		}
		filter.Filters = append(filter.Filters, viewmodel.ListFilter{Field: "email", Op: "contains", Value: email})
	}
	if role := values.Get("role"); role != "" {
		field, _ := listFieldByKey(memberFilterFields, "role")
		if !listChoiceValueValid(field, role) {
			return viewmodel.MemberListQuery{}, false
		}
		filter.Filters = append(filter.Filters, viewmodel.ListFilter{Field: "role", Op: "equals", Value: role})
	}
	form := viewmodel.MemberListQuery{Filter: filter, Sort: values.Get("sort"), Search: strings.TrimSpace(values.Get("q"))}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 0 || limit > 100 {
			return viewmodel.MemberListQuery{}, false
		}
		form.Limit = limit
	}
	form.Page = listPageNumber(values)
	if form.Sort == "" {
		form.Sort = "email"
	}
	if form.Sort != "email" && form.Sort != "-email" && form.Sort != "role" && form.Sort != "-role" &&
		form.Sort != "created_at" && form.Sort != "-created_at" {
		return viewmodel.MemberListQuery{}, false
	}
	if values.Has("page") && form.Limit == 0 {
		form.Limit = defaultLimit
	}
	return form, true
}

// memberMatchesFilter reports whether one member meets one condition.
func memberMatchesFilter(member identity.MemberSummary, filter viewmodel.ListFilter) bool {
	switch filter.Field {
	case "email":
		if filter.Op == "equals" {
			return strings.EqualFold(member.Email, filter.Value)
		}
		return strings.Contains(strings.ToLower(member.Email), strings.ToLower(filter.Value))
	case "role":
		for _, role := range strings.Split(filter.Value, ",") {
			if string(member.Role) == role {
				return true
			}
		}
		return false
	}
	return false
}
