package web

import (
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// TestApplyColumnOrderKeepsTheNameFirstAndFollowsTheSavedSequence checks that the name column stays first, that
// saved columns follow their saved positions, and that a column with no saved position keeps its declared place
// after the ordered ones.
func TestApplyColumnOrderKeepsTheNameFirstAndFollowsTheSavedSequence(t *testing.T) {
	columns := []viewmodel.ListColumn{{Key: "name"}, {Key: "email"}, {Key: "phone"}, {Key: "notes"}, {Key: "created"}}
	keys := func(columns []viewmodel.ListColumn) []string {
		out := make([]string, 0, len(columns))
		for _, column := range columns {
			out = append(out, column.Key)
		}
		return out
	}
	cases := []struct {
		name  string
		order []string
		want  []string
	}{
		{"no saved order keeps the declared order", nil, []string{"name", "email", "phone", "notes", "created"}},
		{"saved order is followed", []string{"name", "notes", "phone", "email", "created"}, []string{"name", "notes", "phone", "email", "created"}},
		{"name stays first even when saved later", []string{"notes", "name", "email"}, []string{"name", "notes", "email", "phone", "created"}},
		{"unsaved columns keep their place after the ordered ones", []string{"created", "email"}, []string{"name", "created", "email", "phone", "notes"}},
	}
	for _, tc := range cases {
		if got := keys(applyColumnOrder(columns, tc.order)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := keys(applyColumnOrder(columns, []string{"email"})); !reflect.DeepEqual(got[0], "name") {
		t.Fatalf("the name column must stay first, got %v", got)
	}
}

// hxPostOutsideColumns reports whether a page has an htmx post other than the column preference posts, which are
// the only posts a list page may make in place.
func hxPostOutsideColumns(body string) bool {
	columns := regexp.MustCompile(`hx-post="[^"]*/preferences/columns/[^"]*"`)
	return strings.Contains(columns.ReplaceAllString(body, ""), "hx-post=")
}

// TestListLinksDoNotCarryTheResultsMarker checks that a link built from a results request, such as a column change,
// does not carry the fragment marker, so a sort or filter link opens the normal list.
func TestListLinksDoNotCarryTheResultsMarker(t *testing.T) {
	query := url.Values{"part": {listPartResults}, "all": {"1"}, "sort": {"email"}, "order": {"desc"}}
	copied := cloneClientQuery(query)
	if _, ok := copied[listPartKey]; ok {
		t.Fatal("the results marker leaked into a copied query")
	}
	if _, ok := copied[listFullKey]; ok {
		t.Fatal("the full-list marker leaked into a copied query")
	}
	if copied.Get("sort") != "email" || copied.Get("order") != "desc" {
		t.Fatalf("the list's own query was lost: %v", copied)
	}
}
