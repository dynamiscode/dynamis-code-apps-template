package web

import (
	"fmt"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func assertListStreamsRows(t *testing.T, render func(viewmodel.PageData) templ.Component) {
	t.Helper()
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en", ListTotal: 1500, HasVisibleClientColumns: true, ListColumns: []viewmodel.ListColumn{{Key: "id", Visible: true}}}
	for i := 0; i < 1500; i++ {
		data.ListRows = append(data.ListRows, viewmodel.ListRow{ID: fmt.Sprintf("member-%04d", i)})
	}
	var output memberStreamWriter
	if err := render(data).Render(t.Context(), &output); err != nil {
		t.Fatal(err)
	}
	if !output.seenThousand || !strings.Contains(output.String(), "member-1499") {
		t.Fatal("rows did not stream through row 1000 and finish")
	}
}

func assertListPreservesRendering(t *testing.T, render, original func(viewmodel.PageData) templ.Component) {
	t.Helper()
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []bool{false, true} {
		for _, rows := range []bool{false, true} {
			data := viewmodel.PageData{Catalog: catalog, Locale: "en", ListFragment: fragment, HasVisibleClientColumns: true, ListColumns: []viewmodel.ListColumn{{Key: "id", Visible: true}}}
			if rows {
				data.ListRows = []viewmodel.ListRow{{ID: "row-1"}}
				data.ListTotal = 1
			}
			var got, want strings.Builder
			if err := render(data).Render(t.Context(), &got); err != nil {
				t.Fatal(err)
			}
			if err := original(data).Render(t.Context(), &want); err != nil {
				t.Fatal(err)
			}
			if got.String() != want.String() {
				t.Errorf("fragment=%v rows=%v: output differs", fragment, rows)
			}
		}
	}
}
