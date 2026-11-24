package organisms

import (
	"os"
	"strings"
	"testing"
)

func TestSharedNavigationStylesKeepVisibleBorders(t *testing.T) {
	css, err := os.ReadFile("../../assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		".workspace-main :is(a.link, p > a:not(.btn), a.detail-back)",
		"box-decoration-break: clone",
		".list-page-size-options a, .workspace-main a.btn-ghost { border: 1px solid var(--border-strong); }",
		"padding: .4rem .6rem; border: 1px solid var(--border-strong)",
		"padding: .45rem .5rem; border: 1px solid var(--border-strong)",
	} {
		if !strings.Contains(string(css), rule) {
			t.Errorf("missing navigation boundary or wrapping rule %q", rule)
		}
	}
}
