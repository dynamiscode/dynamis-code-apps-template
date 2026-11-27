package web

import (
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/url"
	"strings"
	"testing"
)

// TestListFilterBackKeepsMenuOpen checks that "Back to fields" keeps the add menu open on the field list
// instead of closing it, and that a plain list URL keeps it closed.
func TestListFilterBackKeepsMenuOpen(t *testing.T) {
	fields := []listFilterField{{Key: "size", LabelKey: "files.size", Ops: []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}}}}
	back, ok := parseListFilters(url.Values{"filter_field": {"size"}, "filter_op": {"equals"}}, fields)
	if !ok {
		t.Fatal("step query refused")
	}
	view := buildListFilterView("/list", url.Values{"filter_field": {"size"}}, fields, back)
	if !strings.HasSuffix(view.BackHref, "filter_field=") {
		t.Fatalf("back link must keep filter_field empty, got %q", view.BackHref)
	}
	fieldList, ok := parseListFilters(url.Values{"filter_field": {""}}, fields)
	if !ok || !fieldList.AddOpen || fieldList.AddField != "" {
		t.Fatalf("empty filter_field must open the field list: %+v, %v", fieldList, ok)
	}
	if view := buildListFilterView("/list", url.Values{"filter_field": {""}}, fields, fieldList); !view.AddOpen || view.Step != nil {
		t.Fatalf("field list view = open %v, step %v", view.AddOpen, view.Step)
	}
	closed, _ := parseListFilters(url.Values{}, fields)
	if closed.AddOpen || buildListFilterView("/list", url.Values{}, fields, closed).AddOpen {
		t.Fatal("plain list must keep the add menu closed")
	}
}

// TestListFilterKeepsAppliedConditions checks that a condition applied from the add step is kept by the
// links built after it: the next field link, Back, and the chip edits all carry every condition.
func TestListFilterKeepsAppliedConditions(t *testing.T) {
	fields := []listFilterField{
		{Key: "size", LabelKey: "files.size", Ops: []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}}},
		{Key: "name", LabelKey: "files.name", Ops: []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}}},
	}
	query := url.Values{"filter": {"size:equals:1"}, "filter_field": {"name"}, "filter_op": {"equals"}, "filter_value": {"x"}}
	input, ok := parseListFilters(query, fields)
	if !ok || len(input.Filters) != 2 {
		t.Fatalf("applied step = %+v, %v", input, ok)
	}
	view := buildListFilterView("/list", query, fields, input)
	for name, href := range map[string]string{"field link": view.Fields[0].Href, "back": view.BackHref} {
		if strings.Count(href, "filter=") != 2 {
			t.Fatalf("%s lost a condition: %q", name, href)
		}
	}
}

// TestListFilterRange checks the shared "between" condition: a stored range needs both ends, the add step
// applies a complete range and keeps an incomplete one open, and the chip and its edit link show both ends.
func TestListFilterRange(t *testing.T) {
	fields := []listFilterField{{
		Key:      "size",
		LabelKey: "files.size",
		Ops:      []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}, {Key: viewmodel.ListOpBetween, LabelKey: "clients.op_between"}},
	}}
	if _, ok := parseListFilters(url.Values{"filter": {"size:between:1"}}, fields); ok {
		t.Fatal("a range with one end must be refused")
	}
	complete, ok := parseListFilters(url.Values{
		"filter_field": {"size"}, "filter_op": {viewmodel.ListOpBetween}, "filter_value": {"1"}, "filter_value_to": {"5"},
	}, fields)
	if !ok || len(complete.Filters) != 1 || complete.Filters[0].Value != "1,5" || complete.AddField != "" {
		t.Fatalf("complete range = %+v, %v", complete, ok)
	}
	open, ok := parseListFilters(url.Values{"filter_field": {"size"}, "filter_op": {viewmodel.ListOpBetween}, "filter_value": {"1"}}, fields)
	if !ok || open.AddField != "size" || open.AddValueTo != "" || len(open.Filters) != 0 {
		t.Fatalf("open range = %+v, %v", open, ok)
	}
	view := buildListFilterView("/list", url.Values{}, fields, complete)
	if len(view.Chips) != 1 || view.Chips[0].Value != "1 and 5" ||
		!strings.Contains(view.Chips[0].EditHref, "filter_value_to=5") {
		t.Fatalf("range chip = %+v", view.Chips)
	}
}

func TestListFilterFieldStepsPreserveOptions(t *testing.T) {
	fields := []listFilterField{
		{Key: "amount", Label: "Amount", InputType: "number", Ops: []listFilterOp{{Key: "equals"}, {Key: viewmodel.ListOpBetween}}},
		{Key: "custom", Label: "Custom field", Ops: []listFilterOp{{Key: "equals"}}, Choices: []listFilterChoice{{Value: "a", Label: "Alpha"}, {Value: "b", Label: "Beta"}}},
	}
	query := url.Values{"search": {"example"}, "filter": {"amount:equals:3"}}
	input, ok := parseListFilters(query, fields)
	if !ok {
		t.Fatal("valid query rejected")
	}
	view := buildListFilterView("/list", query, fields, input)
	amount, custom := view.Fields[0], view.Fields[1]
	if !amount.Step.Range || amount.Step.InputType != "number" || len(amount.Step.Ops) != 2 || !amount.Step.Ops[0].Checked {
		t.Fatalf("amount step = %+v", amount.Step)
	}
	if !custom.Step.IsChoice || len(custom.Step.Choices) != 2 || custom.Step.Choices[1].Label != "Beta" {
		t.Fatalf("custom step = %+v", custom.Step)
	}
	if !strings.Contains(custom.Href, "filter_field=custom") || !strings.Contains(custom.Href, "search=example") || !strings.Contains(custom.Href, "filter=amount") {
		t.Fatalf("fallback link lost query: %q", custom.Href)
	}
}
