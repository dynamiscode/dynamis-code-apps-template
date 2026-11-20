package viewmodel

import (
	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"strings"
)

func (query FileListQuery) ListInput() appfiles.ListInput {
	input := appfiles.ListInput{Limit: query.Limit, Page: query.Page, Search: query.Search, MIME: query.MIME, Match: query.Match, Sort: strings.TrimPrefix(query.Sort, "-"), Descending: strings.HasPrefix(query.Sort, "-")}
	if query.HasMinSize {
		input.MinSize = &query.MinSize
	}
	if query.HasMaxSize {
		input.MaxSize = &query.MaxSize
	}
	return input
}
