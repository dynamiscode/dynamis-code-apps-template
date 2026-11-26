package web

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/components/organisms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

type memberStreamWriter struct {
	strings.Builder
	seenThousand bool
}

func (w *memberStreamWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "member-0999") {
		w.seenThousand = strings.Contains(w.String(), "member-0000")
	}
	return w.Builder.Write(p)
}

type memberErrorWriter struct{ err error }

func (w memberErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestMemberListStreamsAndPropagatesErrors(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{
		Catalog: catalog, Locale: "en", MemberTotal: 1500, ListTotal: 1500,
		HasVisibleClientColumns: true,
		ListColumns:             []viewmodel.ListColumn{{Key: "id", Visible: true}},
	}
	for i := 0; i < 1500; i++ {
		data.ListRows = append(data.ListRows, viewmodel.ListRow{ID: fmt.Sprintf("member-%04d", i)})
	}
	var output memberStreamWriter
	if err := memberListView(data).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !output.seenThousand || !strings.Contains(output.String(), "member-1499") {
		t.Fatal("member rows did not stream through row 1000 and finish")
	}
	var original strings.Builder
	results := organisms.ListResults(data, data.MemberListQuery.Limit, listPageSizeOptions(data.MemberListQuery.Limit, memberPageSizes), data.ClientQuerySuffix, memberRows(data), templ.NopComponent)
	if err := organisms.ListScreen(data, "members.search", memberListTools(data), results).Render(context.Background(), &original); err != nil {
		t.Fatal(err)
	}
	if output.String() != original.String() {
		t.Fatal("member list output differs from original list components")
	}
	want := errors.New("write failed")
	if err := memberResults(data).Render(context.Background(), memberErrorWriter{want}); !errors.Is(err, want) {
		t.Fatalf("writer error: got %v, want %v", err, want)
	}
}
