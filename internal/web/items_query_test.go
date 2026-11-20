package web

import (
	"net/url"
	"testing"
)

// TestParseItemListQueryConditions checks the items query through the shared filter model: conditions
// and match mode, one condition per field, the legacy search and status links, and invalid values.
func TestParseItemListQueryConditions(t *testing.T) {
	form, ok := parseItemListQuery(url.Values{
		"q": {"invoice"}, "filter": {"status:equals:complete"}, "match": {"any"},
	}, 50)
	if !ok || form.Match != "any" || form.Search != "invoice" || form.Status != "complete" {
		t.Fatalf("conditions = %+v, %v", form, ok)
	}
	if _, ok := parseItemListQuery(url.Values{"filter": {"title:contains:a"}}, 50); ok {
		t.Fatal("a title filter must be refused: the search box takes the title")
	}
	if _, ok := parseItemListQuery(url.Values{"filter": {"status:equals:active,complete"}}, 50); ok {
		t.Fatal("a multi-status condition must be refused: the service takes one status")
	}
	if _, ok := parseItemListQuery(url.Values{"match": {"sometimes"}}, 50); ok {
		t.Fatal("an unknown match mode must be refused")
	}
	legacy, ok := parseItemListQuery(url.Values{"search": {"quarterly"}, "status": {"active"}}, 50)
	if !ok || legacy.Search != "quarterly" || legacy.Status != "active" || legacy.Match != "all" {
		t.Fatalf("legacy links = %+v, %v", legacy, ok)
	}
}
