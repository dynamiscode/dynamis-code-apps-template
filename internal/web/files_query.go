package web

import (
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/url"
	"strconv"
	"strings"
)

// fileFilterFields are the fields the files list can filter on. The name search is separate: it always
// combines with AND. The file service takes one type and one size range, so each field can appear once.
var fileFilterFields = []listFilterField{
	{
		Key:      "type",
		LabelKey: "files.type",
		Ops:      []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}},
	},
	{
		Key:      "size",
		LabelKey: "files.size",
		Ops: []listFilterOp{
			{Key: "equals", LabelKey: "clients.op_equals"},
			{Key: viewmodel.ListOpBetween, LabelKey: "clients.op_between"},
		},
	},
}

// fileListParams are the query keys the files list accepts. The search box sends q; the search, mime,
// min_size, and max_size keys are the links from before, and they still work.
var fileListParams = map[string]bool{
	"filter": true, "match": true, "filter_field": true, "filter_op": true, "filter_value": true, "filter_value_to": true,
	"filter_error": true, listPartKey: true, listFullKey: true, "q": true, "search": true, "mime": true, "min_size": true, "max_size": true, "sort": true, "limit": true, "page": true,
}

func parseFileListQuery(values url.Values, defaultLimit int) (viewmodel.FileListQuery, bool) {
	query := viewmodel.FileListQuery{Sort: values.Get("sort"), Limit: defaultLimit, Page: listPageNumber(values)}
	for key, entries := range values {
		if !fileListParams[key] || len(entries) != 1 && key != "filter" && key != "filter_value" {
			return query, false
		}
	}
	input, ok := parseListFilters(values, fileFilterFields)
	if !ok {
		return query, false
	}
	search := strings.TrimSpace(values.Get("q"))
	if search == "" {
		search = strings.TrimSpace(values.Get("search"))
	}
	query.Search = search
	if mime := strings.TrimSpace(values.Get("mime")); mime != "" {
		input.Filters = append(input.Filters, viewmodel.ListFilter{Field: "type", Op: "equals", Value: mime})
	}
	if minSize, maxSize := values.Get("min_size"), values.Get("max_size"); minSize != "" || maxSize != "" {
		if minSize == "" {
			minSize = "0"
		}
		if maxSize == "" {
			maxSize = "9223372036854775807"
		}
		input.Filters = append(input.Filters, viewmodel.ListFilter{Field: "size", Op: viewmodel.ListOpBetween, Value: minSize + "," + maxSize})
	}
	query.Filter, query.Match = input, input.Match
	seen := map[string]bool{}
	for _, filter := range input.Filters {
		if seen[filter.Field] {
			return query, false
		}
		seen[filter.Field] = true
		switch filter.Field {
		case "type":
			query.MIME = filter.Value
		case "size":
			from, to, _ := strings.Cut(filter.Value, ",")
			if filter.Op == viewmodel.ListOpBetween {
				min, errMin := strconv.ParseInt(from, 10, 64)
				max, errMax := strconv.ParseInt(to, 10, 64)
				if errMin != nil || errMax != nil || min < 0 || max < min {
					return query, false
				}
				query.MinSize, query.HasMinSize = min, true
				query.MaxSize, query.HasMaxSize = max, true
			} else {
				size, err := strconv.ParseInt(from, 10, 64)
				if err != nil || size < 0 {
					return query, false
				}
				query.MinSize, query.HasMinSize = size, true
				query.MaxSize, query.HasMaxSize = size, true
			}
		}
	}
	if len(query.Search) > 200 || len(query.MIME) > 128 {
		return query, false
	}
	if query.Sort == "" {
		query.Sort = "-created_at"
	}
	if query.Sort != "original_name" && query.Sort != "-original_name" && query.Sort != "size" && query.Sort != "-size" && query.Sort != "created_at" && query.Sort != "-created_at" {
		return query, false
	}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return query, false
		}
		query.Limit = limit
	}
	return query, true
}
