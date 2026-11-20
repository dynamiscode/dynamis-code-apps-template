package viewmodel

import (
	"example.com/dynamis-code/apps-template/internal/clients"
)

func (form ClientQueryForm) ListInput(limit, page int) clients.ListInput {
	input := clients.ListInput{Limit: limit, Page: page, Archived: form.Archived, Search: form.Search}
	if form.Sort != "" {
		input.Sort = clients.SortInput{Field: form.Sort, Descending: form.Order == "desc"}
	}
	input.Filters = form.Filters
	input.Match = form.Match
	return input
}
