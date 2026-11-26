package web

import (
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestClientListStreamsRows(t *testing.T) {
	assertListStreamsRows(t, clientListView)
}

func TestClientListPreservesRendering(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []bool{false, true} {
		data := viewmodel.PageData{Catalog: catalog, Locale: "en", ListFragment: fragment}
		var out strings.Builder
		if err := clientListView(data).Render(t.Context(), &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), `id="list-filter-add"`) || strings.Contains(out.String(), `id="client-list-scope"`) {
			t.Fatal("client list must offer filters without separate scope buttons")
		}
		if strings.Contains(out.String(), `id="list-filter-add" hx-swap-oob="true"`) != fragment {
			t.Fatal("filter controls must be refreshed only for results fragments")
		}
	}
}
