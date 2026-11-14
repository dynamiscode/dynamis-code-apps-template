package clients

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

// queryCase is one list request and the display names it returns, in order.
type queryCase struct {
	name    string
	sort    SortInput
	filters []FilterInput
	want    []string
}

// exerciseClientQueries seeds one workspace and checks the filter and sort
// rules. SQLite and PostgreSQL both run it, so the expected results match.
func exerciseClientQueries(t *testing.T, f fixture) {
	t.Helper()
	company := defineTypedField(t, f, FieldTypeText)
	score := defineTypedField(t, f, FieldTypeNumber)
	joined := defineTypedField(t, f, FieldTypeDate)
	vip := defineTypedField(t, f, FieldTypeYesNo)
	tier := defineChoiceField(t, f, "Tier")
	gold := addOptionForQuery(t, f, tier, "Gold")
	silver := addOptionForQuery(t, f, tier, "Silver")
	bronze := addOptionForQuery(t, f, tier, "Bronze")

	create := func(name, email string) Client {
		t.Helper()
		client, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: name, Email: email}, identity.AuditContext{})
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
		return client
	}
	set := func(client Client, field Field, value string) Client {
		t.Helper()
		updated, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, field.ID, value, identity.AuditContext{})
		if err != nil {
			t.Fatalf("SetValue(%s, %q) error = %v", client.DisplayName, value, err)
		}
		return updated
	}

	// Creation order is id order: Ada, Bea, Cy, Dan, Eve, Fay.
	ada := create("Ada", "ada@example.com")
	ada = set(ada, company, "Analytical Engines")
	ada = set(ada, score, "9")
	ada = set(ada, joined, "2026-01-10")
	ada = set(ada, vip, "true")
	ada = set(ada, tier, gold.ID.String())
	bea := create("Bea", "")
	bea = set(bea, company, "bread co")
	bea = set(bea, score, "10")
	bea = set(bea, joined, "2026-03-01")
	bea = set(bea, vip, "false")
	bea = set(bea, tier, silver.ID.String())
	create("Cy", "")
	dan := create("Dan", "dan@example.com")
	dan = set(dan, company, "ANALYTICS hub")
	dan = set(dan, score, "12.5")
	dan = set(dan, joined, "2025-12-31")
	dan = set(dan, vip, "true")
	dan = set(dan, tier, bronze.ID.String())
	eve := create("Eve", "eve@example.com")
	eve = set(eve, company, "Ünïcode Corp")
	eve = set(eve, score, "-3")
	eve = set(eve, joined, "2026-01-10")
	fay := create("Fay", "")
	fay = set(fay, company, "analytics")
	fay = set(fay, score, "10")
	fay = set(fay, vip, "false")

	// Default order is newest first, so the seed order reversed.
	defaultOrder := []string{"Fay", "Eve", "Dan", "Cy", "Bea", "Ada"}
	if got := listNamesForQuery(t, f, SortInput{}); !slices.Equal(got, defaultOrder) {
		t.Fatalf("default order = %v, want %v", got, defaultOrder)
	}

	// Choice sort orders by the stored option short ID, not by label, so the
	// expected order comes from sorting those IDs.
	tierKeys := []string{gold.ID.String(), silver.ID.String(), bronze.ID.String()}
	slices.Sort(tierKeys)
	tierNames := map[string]string{gold.ID.String(): "Ada", silver.ID.String(): "Bea", bronze.ID.String(): "Dan"}
	tierWant := []string{tierNames[tierKeys[0]], tierNames[tierKeys[1]], tierNames[tierKeys[2]], "Cy", "Eve", "Fay"}

	cases := []queryCase{
		// Text: contains and equals ignore ASCII case. Non-ASCII is compared exactly.
		{name: "text contains ignores case", filters: []FilterInput{flt(company.ID.String(), OpContains, "ANALYTIC")}, want: []string{"Fay", "Dan", "Ada"}},
		{name: "text equals ignores case", filters: []FilterInput{flt(company.ID.String(), OpEquals, "BREAD CO")}, want: []string{"Bea"}},
		{name: "text does not fold non-ASCII", filters: []FilterInput{flt(company.ID.String(), OpContains, "ünï")}, want: []string{}},
		{name: "text matches non-ASCII as typed", filters: []FilterInput{flt(company.ID.String(), OpContains, "Ünï")}, want: []string{"Eve"}},
		// Number: canonical equality, numeric order for before, after, and between.
		{name: "number equals canonical value", filters: []FilterInput{flt(score.ID.String(), OpEquals, "10.0")}, want: []string{"Fay", "Bea"}},
		{name: "number before is numeric", filters: []FilterInput{flt(score.ID.String(), OpBefore, "10")}, want: []string{"Eve", "Ada"}},
		{name: "number after is numeric", filters: []FilterInput{flt(score.ID.String(), OpAfter, "9")}, want: []string{"Fay", "Dan", "Bea"}},
		{name: "number between is inclusive", filters: []FilterInput{flt(score.ID.String(), OpBetween, "9", "10")}, want: []string{"Fay", "Bea", "Ada"}},
		// Date: text order is calendar order for YYYY-MM-DD.
		{name: "date equals", filters: []FilterInput{flt(joined.ID.String(), OpEquals, "2026-01-10")}, want: []string{"Eve", "Ada"}},
		{name: "date before is strict", filters: []FilterInput{flt(joined.ID.String(), OpBefore, "2026-01-10")}, want: []string{"Dan"}},
		{name: "date after is strict", filters: []FilterInput{flt(joined.ID.String(), OpAfter, "2026-01-10")}, want: []string{"Bea"}},
		{name: "date between is inclusive", filters: []FilterInput{flt(joined.ID.String(), OpBetween, "2025-12-31", "2026-01-10")}, want: []string{"Eve", "Dan", "Ada"}},
		// Yes/no and single choice.
		{name: "yes/no true", filters: []FilterInput{flt(vip.ID.String(), OpEquals, "true")}, want: []string{"Dan", "Ada"}},
		{name: "yes/no false", filters: []FilterInput{flt(vip.ID.String(), OpEquals, "false")}, want: []string{"Fay", "Bea"}},
		{name: "choice equals one option", filters: []FilterInput{flt(tier.ID.String(), OpEquals, gold.ID.String())}, want: []string{"Ada"}},
		{name: "choice equals several options", filters: []FilterInput{flt(tier.ID.String(), OpEquals, gold.ID.String(), bronze.ID.String())}, want: []string{"Dan", "Ada"}},
		// Filters combine with AND, and an empty value never matches.
		{name: "filters combine", filters: []FilterInput{
			flt(company.ID.String(), OpContains, "analytic"),
			flt(vip.ID.String(), OpEquals, "true"),
			flt(score.ID.String(), OpBefore, "10"),
		}, want: []string{"Ada"}},
		{name: "common email contains", filters: []FilterInput{flt(FieldEmail, OpContains, "DAN@")}, want: []string{"Dan"}},
		{name: "common display name equals", filters: []FilterInput{flt(FieldDisplayName, OpEquals, "bea")}, want: []string{"Bea"}},
		// Sort: one field, empty values last, ties by id in the sort direction.
		{name: "sort email ascending", sort: SortInput{Field: FieldEmail}, want: []string{"Ada", "Dan", "Eve", "Bea", "Cy", "Fay"}},
		{name: "sort email descending", sort: SortInput{Field: FieldEmail, Descending: true}, want: []string{"Eve", "Dan", "Ada", "Fay", "Cy", "Bea"}},
		{name: "sort number ascending", sort: SortInput{Field: score.ID.String()}, want: []string{"Eve", "Ada", "Bea", "Fay", "Dan", "Cy"}},
		{name: "sort number descending", sort: SortInput{Field: score.ID.String(), Descending: true}, want: []string{"Dan", "Fay", "Bea", "Ada", "Eve", "Cy"}},
		{name: "sort date ascending", sort: SortInput{Field: joined.ID.String()}, want: []string{"Dan", "Ada", "Eve", "Bea", "Cy", "Fay"}},
		{name: "sort date descending", sort: SortInput{Field: joined.ID.String(), Descending: true}, want: []string{"Bea", "Eve", "Ada", "Dan", "Fay", "Cy"}},
		{name: "sort text is byte order", sort: SortInput{Field: company.ID.String()}, want: []string{"Dan", "Ada", "Fay", "Bea", "Eve", "Cy"}},
		{name: "sort text descending", sort: SortInput{Field: company.ID.String(), Descending: true}, want: []string{"Eve", "Bea", "Fay", "Ada", "Dan", "Cy"}},
		{name: "sort choice by option ID", sort: SortInput{Field: tier.ID.String()}, want: tierWant},
		// Filter and sort compose.
		{name: "sort within a filter", sort: SortInput{Field: score.ID.String()}, filters: []FilterInput{flt(vip.ID.String(), OpEquals, "false")}, want: []string{"Bea", "Fay"}},
	}
	for _, tc := range cases {
		if got := listNamesForQuery(t, f, tc.sort, tc.filters...); !slices.Equal(got, tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}

	// Pages follow the sort exactly, one row at a time across the tie at 10.
	full := []string{"Eve", "Ada", "Bea", "Fay", "Dan", "Cy"}
	var paged []string
	cursor := ""
	for range full {
		page, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 1, Cursor: cursor, Sort: SortInput{Field: score.ID.String()}})
		if err != nil || len(page.Clients) != 1 {
			t.Fatalf("paged List() = %+v, %v", page, err)
		}
		paged = append(paged, page.Clients[0].DisplayName)
		cursor = page.NextCursor
	}
	if !slices.Equal(paged, full) || cursor != "" {
		t.Fatalf("paged number sort = %v with cursor %q, want %v and no cursor", paged, cursor, full)
	}

	// A cursor belongs to the sort that issued it.
	first, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 1, Sort: SortInput{Field: score.ID.String()}})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first sorted page = %+v, %v", first, err)
	}
	if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 1, Cursor: first.NextCursor, Sort: SortInput{Field: joined.ID.String()}}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cursor from another sort error = %v, want ErrInvalidCursor", err)
	}
	if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 1, Cursor: first.NextCursor}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("sorted cursor on the default order error = %v, want ErrInvalidCursor", err)
	}

	// Bad sort or filter input is ErrInvalidInput on every path.
	invalid := map[string]ListInput{
		"unknown sort field":        {Sort: SortInput{Field: "nope"}},
		"unknown filter field":      {Filters: []FilterInput{flt("nope", OpEquals, "x")}},
		"before on text":            {Filters: []FilterInput{flt(company.ID.String(), OpBefore, "x")}},
		"contains on number":        {Filters: []FilterInput{flt(score.ID.String(), OpContains, "1")}},
		"between on common text":    {Filters: []FilterInput{flt(FieldEmail, OpBetween, "a", "b")}},
		"empty text value":          {Filters: []FilterInput{flt(company.ID.String(), OpContains, "")}},
		"between with one value":    {Filters: []FilterInput{flt(score.ID.String(), OpBetween, "9")}},
		"between from after to":     {Filters: []FilterInput{flt(score.ID.String(), OpBetween, "10", "9")}},
		"number that is not finite": {Filters: []FilterInput{flt(score.ID.String(), OpEquals, "abc")}},
		"date that is not real":     {Filters: []FilterInput{flt(joined.ID.String(), OpEquals, "2026-02-30")}},
		"yes/no that is not a bool": {Filters: []FilterInput{flt(vip.ID.String(), OpEquals, "yes")}},
		"choice given by label":     {Filters: []FilterInput{flt(tier.ID.String(), OpEquals, "Gold")}},
		"choice with no options":    {Filters: []FilterInput{flt(tier.ID.String(), OpEquals)}},
	}
	for name, input := range invalid {
		input.Limit = 50
		if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: List() error = %v, want ErrInvalidInput", name, err)
		}
	}

	// Archived clients leave every list. The archive use case lands in #303, so
	// this sets the column directly.
	if _, err := f.db.ExecContext(f.ctx, database.Rebind(f.service.driver,
		"UPDATE clients SET archived_at = ? WHERE id = ?"), "2026-02-01T00:00:00.000000000Z", int64(eve.ID)); err != nil {
		t.Fatal(err)
	}
	if got := listNamesForQuery(t, f, SortInput{}); !slices.Equal(got, []string{"Fay", "Dan", "Cy", "Bea", "Ada"}) {
		t.Fatalf("default order with an archived client = %v", got)
	}
	if got := listNamesForQuery(t, f, SortInput{Field: score.ID.String()}, flt(vip.ID.String(), OpEquals, "true")); !slices.Equal(got, []string{"Ada", "Dan"}) {
		t.Fatalf("filtered list with an archived client = %v", got)
	}

	// An archived field keeps its values but is not a form field, so queries
	// refuse it.
	if _, err := f.service.ArchiveField(f.ctx, f.owner, f.workspaceID, vip.ID, identity.AuditContext{}); err != nil {
		t.Fatalf("ArchiveField() error = %v", err)
	}
	if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 50, Filters: []FilterInput{flt(vip.ID.String(), OpEquals, "true")}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("filter on an archived field error = %v, want ErrInvalidInput", err)
	}
}

func addOptionForQuery(t *testing.T, f fixture, field Field, label string) Option {
	t.Helper()
	option, err := f.service.AddOption(f.ctx, f.owner, f.workspaceID, field.ID, label, identity.AuditContext{})
	if err != nil {
		t.Fatalf("AddOption(%s) error = %v", label, err)
	}
	return option
}

func listNamesForQuery(t *testing.T, f fixture, sortBy SortInput, filters ...FilterInput) []string {
	t.Helper()
	page, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{Limit: 50, Sort: sortBy, Filters: filters})
	if err != nil {
		t.Fatalf("List(%+v, %+v) error = %v", sortBy, filters, err)
	}
	names := []string{}
	for _, client := range page.Clients {
		names = append(names, client.DisplayName)
	}
	return names
}

func flt(field, op string, values ...string) FilterInput {
	return FilterInput{Field: field, Op: op, Value: strings.Join(values, ",")}
}

// TestClientQueriesAcrossFieldTypes runs the query rules on SQLite.
func TestClientQueriesAcrossFieldTypes(t *testing.T) {
	exerciseClientQueries(t, newFixture(t))
}

// TestListCursorBindsSortAndFilters refuses a cursor from another sort or filter
// set, so a page cannot silently resume a different list.
func TestListCursorBindsSortAndFilters(t *testing.T) {
	f := newFixture(t)
	score := defineTypedField(t, f, FieldTypeNumber)
	vip := defineTypedField(t, f, FieldTypeYesNo)
	for _, name := range []string{"Ada", "Bea", "Cy"} {
		client, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: name}, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version, vip.ID, "true", identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.SetValue(f.ctx, f.member, f.workspaceID, client.ID, client.Version+1, score.ID, "1", identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{
		Limit: 1, Sort: SortInput{Field: score.ID.String()}, Filters: []FilterInput{flt(vip.ID.String(), OpEquals, "true")},
	})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	for name, input := range map[string]ListInput{
		"other filters": {Limit: 1, Sort: SortInput{Field: score.ID.String()}},
		"other sort":    {Limit: 1, Sort: SortInput{Field: score.ID.String(), Descending: true}, Filters: []FilterInput{flt(vip.ID.String(), OpEquals, "true")}},
	} {
		input.Cursor = page.NextCursor
		if _, err := f.service.List(f.ctx, f.viewer, f.workspaceID, input); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("%s: List() error = %v, want ErrInvalidCursor", name, err)
		}
	}
	next, err := f.service.List(f.ctx, f.viewer, f.workspaceID, ListInput{
		Limit: 1, Cursor: page.NextCursor, Sort: SortInput{Field: score.ID.String()}, Filters: []FilterInput{flt(vip.ID.String(), OpEquals, "true")},
	})
	if err != nil || len(next.Clients) != 1 {
		t.Fatalf("same query next page = %+v, %v", next, err)
	}
}
