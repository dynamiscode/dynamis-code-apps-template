package molecules

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

type columnStreamWriter struct {
	strings.Builder
	streamed bool
}

func (w *columnStreamWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "column-0099") {
		w.streamed = strings.Contains(w.String(), "column-0000")
	}
	return w.Builder.Write(p)
}

type columnFailWriter struct{ err error }

func (w columnFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestListColumnsMenuStreamsPanel(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en"}
	for i := 0; i < 200; i++ {
		name := fmt.Sprintf("column-%04d", i)
		data.ListColumns = append(data.ListColumns, viewmodel.ListColumn{Key: name, Label: name, Visible: true})
	}
	var output columnStreamWriter
	if err := ListColumnsMenu(data, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !output.streamed || !strings.Contains(output.String(), "column-0199") {
		t.Fatal("columns did not stream through item 100 and finish")
	}
	original := atoms.Dropdown(atoms.DropdownProps{
		Class: "list-menu", SummaryClass: "btn btn-sm",
		SummaryAttrs: templ.Attributes{"aria-label": data.T("clients.columns"), "title": data.T("clients.columns")},
		Summary:      listColumnsSummary(data), Panel: listColumnsPanel(data, ""),
		PanelClass: "list-columns-panel bg-base-100 rounded-box shadow-lg border border-base-200 p-3 w-80 max-w-[90vw]",
		PanelAttrs: templ.Attributes{"aria-label": data.T("clients.columns")},
	})
	var expected strings.Builder
	if err := original.Render(context.Background(), &expected); err != nil {
		t.Fatal(err)
	}
	if output.String() != expected.String() {
		t.Fatal("columns menu output differs from original")
	}
	want := errors.New("write failed")
	if err := ListColumnsMenu(data, "").Render(context.Background(), columnFailWriter{want}); !errors.Is(err, want) {
		t.Fatalf("writer error: got %v, want %v", err, want)
	}
}
