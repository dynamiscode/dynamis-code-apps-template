//go:build browser

package browsertest

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/web/components/molecules"
	"github.com/chromedp/chromedp"
)

func TestInlineHeadingEditorKeepsValueGeometry(t *testing.T) {
	var markup strings.Builder
	props := molecules.InlineEditorProps{ID: "heading", Name: "name", Heading: true, Display: "Saved heading", Value: "Saved heading", InputLabel: "Heading", EditLabel: "Edit", Label: "Heading", SaveLabel: "Save", CancelLabel: "Cancel", CancelURL: "/"}
	if err := molecules.InlineEditor(props).Render(context.Background(), &markup); err != nil {
		t.Fatal(err)
	}
	var styles strings.Builder
	for _, name := range []string{"tokens.css", "components.css", "app.css"} {
		css, err := os.ReadFile(filepath.Join(repoRoot, "internal/web/assets", name))
		if err != nil {
			t.Fatal(err)
		}
		styles.Write(css)
	}
	page := "<style>" + styles.String() + "</style>" + markup.String()
	tab := newTab(t, 1280, 900, "")
	run(t, tab, chromedp.Navigate("data:text/html,"+url.PathEscape(page)))
	waitVisible(t, tab, "[data-inline-title] summary")
	before := eval[[]float64](t, tab, `(() => { const r = document.querySelector('[data-inline-title] summary').getBoundingClientRect(); return [r.left, r.top]; })()`)
	run(t, tab, chromedp.Click("[data-inline-title] summary"))
	waitVisible(t, tab, "#heading")
	after := eval[[]float64](t, tab, `(() => { const el = document.querySelector('#heading'), r = el.getBoundingClientRect(), s = getComputedStyle(el); return [r.left + parseFloat(s.paddingLeft) + parseFloat(s.borderLeftWidth), r.top + parseFloat(s.paddingTop) + parseFloat(s.borderTopWidth)]; })()`)
	for i := range before {
		delta := before[i] - after[i]
		if delta < -1 || delta > 1 {
			t.Fatalf("heading text origin shifted: summary=%v editor=%v", before, after)
		}
	}
}
