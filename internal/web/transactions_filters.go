package web

import (
	"net/url"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/transactions"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// transactionsFilterQuery converts direct links into removable filters and parses the shared filter controls.
func transactionsFilterQuery(values url.Values, categories []transactions.Category, methods []transactions.Method) (url.Values, []listFilterField, viewmodel.ListFilterInput, []transactions.ListCondition, bool) {
	query := cloneClientQuery(values)
	equals := []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}}
	fields := []listFilterField{
		{Key: "client_id", LabelKey: "transactions.client", Ops: equals},
		{Key: "entry_id", LabelKey: "transactions.entry_id", Ops: equals},
		{Key: "refund_of_entry_id", LabelKey: "transactions.refund_of", Ops: equals},
		{Key: "direction", LabelKey: "transactions.direction", Ops: equals, Choices: []listFilterChoice{{Value: "in", LabelKey: "transactions.in"}, {Value: "out", LabelKey: "transactions.out"}}},
		{Key: "category_id", LabelKey: "transactions.category", Ops: equals},
		{Key: "payment_method", LabelKey: "transactions.method", Ops: equals},
		{Key: "occurred_on", LabelKey: "transactions.date", InputType: "date", Ops: []listFilterOp{{Key: "equals", LabelKey: "clients.op_equals"}, {Key: "between", LabelKey: "clients.op_between"}, {Key: "gte", LabelKey: "transactions.totals_start"}, {Key: "lte", LabelKey: "transactions.totals_end"}}},
	}
	for _, category := range categories {
		fields[4].Choices = append(fields[4].Choices, listFilterChoice{Value: category.ID.String(), Label: category.Name})
	}
	for _, method := range methods {
		choice := listFilterChoice{Value: method.ID, Label: method.Name}
		if method.Name == method.ID {
			switch method.ID {
			case "cash", "check", "card", "transfer":
				choice.LabelKey = "transactions.method_" + method.ID
				choice.Label = ""
			}
		}
		fields[5].Choices = append(fields[5].Choices, choice)
	}
	for _, key := range []string{"client_id", "entry_id", "refund_of_entry_id", "direction", "category_id", "payment_method"} {
		for _, value := range query[key] {
			if value != "" {
				query.Add("filter", key+":equals:"+value)
			}
		}
		query.Del(key)
	}
	from, to := query.Get("from"), query.Get("to")
	query.Del("from")
	query.Del("to")
	switch {
	case from != "" && to != "":
		query.Add("filter", "occurred_on:between:"+from+","+to)
	case from != "":
		query.Add("filter", "occurred_on:gte:"+from)
	case to != "":
		query.Add("filter", "occurred_on:lte:"+to)
	}
	input, ok := parseListFilters(query, fields)
	if !ok {
		return query, fields, input, nil, false
	}
	conditions := make([]transactions.ListCondition, 0, len(input.Filters))
	for _, filter := range input.Filters {
		for _, value := range strings.Split(filter.Value, ",") {
			var err error
			switch filter.Field {
			case "client_id":
				_, err = clients.ParseClientID(value)
			case "entry_id", "refund_of_entry_id":
				_, err = transactions.ParseEntryID(value)
			case "category_id":
				_, err = transactions.ParseCategoryID(value)
			case "occurred_on":
				var date time.Time
				date, err = time.Parse("2006-01-02", value)
				if err == nil && date.Format("2006-01-02") != value {
					return query, fields, input, nil, false
				}
			}
			if err != nil {
				return query, fields, input, nil, false
			}
		}
		if filter.Field == "occurred_on" && filter.Op == "between" {
			from, to, _ := strings.Cut(filter.Value, ",")
			if from > to {
				return query, fields, input, nil, false
			}
		}
		conditions = append(conditions, transactions.ListCondition{Field: filter.Field, Op: filter.Op, Value: filter.Value})
	}
	return query, fields, input, conditions, true
}
