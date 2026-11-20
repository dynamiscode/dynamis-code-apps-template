package web

import (
	"net/url"
	"testing"
)

// TestParseFileListQueryConditions checks the files query through the shared model: the size range, the
// legacy size and name links, one condition per field, and the match mode.
func TestParseFileListQueryConditions(t *testing.T) {
	query, ok := parseFileListQuery(url.Values{
		"q": {"report"}, "filter": {"size:between:10,500"}, "match": {"any"},
	}, 50)
	if !ok || query.Search != "report" || query.MinSize != 10 || query.MaxSize != 500 || query.Match != "any" {
		t.Fatalf("conditions = %+v, %v", query, ok)
	}
	if _, ok := parseFileListQuery(url.Values{"filter": {"name:contains:a"}}, 50); ok {
		t.Fatal("a name filter must be refused: the search box takes the name")
	}
	if _, ok := parseFileListQuery(url.Values{"filter": {"size:between:500,10"}}, 50); ok {
		t.Fatal("a size range with its ends reversed must be refused")
	}
	legacy, ok := parseFileListQuery(url.Values{"min_size": {"100"}, "mime": {"application/pdf"}}, 50)
	if !ok || !legacy.HasMinSize || legacy.MinSize != 100 || legacy.MIME != "application/pdf" || legacy.Match != "all" {
		t.Fatalf("legacy links = %+v, %v", legacy, ok)
	}
}
