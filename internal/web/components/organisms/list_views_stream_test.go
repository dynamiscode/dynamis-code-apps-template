package organisms

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

type viewStreamWriter struct {
	strings.Builder
	streamed bool
}

func (w *viewStreamWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "view-0999") {
		w.streamed = strings.Contains(w.String(), "view-0000")
	}
	return w.Builder.Write(p)
}

type viewFailWriter struct{ err error }

func (w viewFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestListViewSwitcherStreamsPanel(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en"}
	for i := 0; i < 1500; i++ {
		data.ListViews = append(data.ListViews, viewmodel.ListViewOption{ID: int64(i + 1), Name: fmt.Sprintf("view-%04d", i)})
	}
	var output viewStreamWriter
	if err := ListViewSwitcher(data).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !output.streamed || !strings.Contains(output.String(), "view-1499") {
		t.Fatal("views did not stream through item 1000 and finish")
	}
	original := atoms.Dropdown(atoms.DropdownProps{
		Class: "list-menu", SummaryClass: "btn btn-sm",
		SummaryAttrs: templ.Attributes{"aria-label": data.T("views.title"), "title": data.T("views.title")},
		Summary:      listViewSummary(data), Panel: listViewPanel(data),
		PanelClass: "list-views-panel bg-base-100 rounded-box shadow-lg border border-base-200 p-3 w-96 max-w-[90vw]",
	})
	var expected strings.Builder
	if err := original.Render(context.Background(), &expected); err != nil {
		t.Fatal(err)
	}
	if output.String() != expected.String() {
		t.Fatal("view menu output differs from original")
	}
	want := errors.New("write failed")
	if err := ListViewSwitcher(data).Render(context.Background(), viewFailWriter{want}); !errors.Is(err, want) {
		t.Fatalf("writer error: got %v, want %v", err, want)
	}
}
