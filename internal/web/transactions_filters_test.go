package web

import (
	"net/url"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/transactions"
)

func TestTransactionsFilterQueryConvertsDirectLinksAndRemovesCapsules(t *testing.T) {
	values := url.Values{"direction": {"in"}, "from": {"2026-01-01"}, "to": {"2026-01-31"}, "page": {"2"}}
	query, fields, input, conditions, ok := transactionsFilterQuery(values, nil, nil)
	if !ok || len(conditions) != 2 || len(input.Filters) != 2 || query.Has("direction") || query.Has("from") || query.Has("to") {
		t.Fatalf("query=%v filters=%+v conditions=%+v ok=%v", query, input.Filters, conditions, ok)
	}
	view := buildListFilterView("/transactions", query, fields, input)
	removed, err := url.Parse(view.Chips[0].RemoveHref)
	if err != nil || strings.Contains(removed.RawQuery, "direction") || len(removed.Query()["filter"]) != 1 {
		t.Fatalf("removal = %q, %v", view.Chips[0].RemoveHref, err)
	}
}

func TestTransactionsFilterQueryRejectsInvalidConditions(t *testing.T) {
	for _, raw := range []string{"direction:equals:invalid", "client_id:equals:invalid", "occurred_on:between:2026-02-01,2026-01-01", "occurred_on:equals:2026-02-30"} {
		_, _, _, _, ok := transactionsFilterQuery(url.Values{"filter": {raw}}, nil, []transactions.Method(nil))
		if ok {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestTransactionsFilterQueryBuildsConditionsForMultipleChoices(t *testing.T) {
	values := url.Values{"filter": {"direction:equals:in,out", "occurred_on:gte:2026-01-01"}, "match": {"any"}}
	_, _, input, conditions, ok := transactionsFilterQuery(values, nil, nil)
	if !ok || input.Match != "any" || len(conditions) != 2 || conditions[0].Value != "in,out" || conditions[1].Op != "gte" {
		t.Fatalf("input=%+v conditions=%+v ok=%v", input, conditions, ok)
	}
}

func TestTransactionsFilterMethodLabels(t *testing.T) {
	methods := []transactions.Method{{ID: "cash", Name: "cash"}, {ID: "card", Name: "Renamed"}, {ID: "custom", Name: "custom"}}
	query := url.Values{"filter": {"payment_method:equals:cash"}}
	canonical, fields, input, _, ok := transactionsFilterQuery(query, nil, methods)
	if !ok {
		t.Fatal("valid method rejected")
	}
	view := buildListFilterView("/transactions", canonical, fields, input)
	if view.Chips[0].ValueKey != "transactions.method_cash" {
		t.Fatalf("cash chip = %+v", view.Chips[0])
	}
	if fields[5].Choices[1].Label != "Renamed" || fields[5].Choices[2].Label != "custom" {
		t.Fatalf("custom method labels = %+v", fields[5].Choices)
	}
}
