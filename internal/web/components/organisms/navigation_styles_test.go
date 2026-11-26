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
		".workspace-main :is(a.link, p > a:not(.btn, .detail-back))",
		"box-decoration-break: clone",
		".list-page-size-options a, .workspace-main a.btn-ghost { border: 1px solid var(--border-strong); }",
	} {
		if !strings.Contains(string(css), rule) {
			t.Errorf("missing navigation boundary or wrapping rule %q", rule)
		}
	}
}

func TestSidebarNavigationAccessibilityStyles(t *testing.T) {
	css, err := os.ReadFile("../../assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range []string{
		".sidebar-navigation .menu li > a { min-height: 2.75rem; color: var(--ink); }",
		".sidebar-navigation .menu li > a.menu-active { color: var(--menu-active-fg); }",
		".sidebar-navigation .menu li > a:focus-visible { outline: .2rem solid var(--focus-outline);",
	} {
		if !strings.Contains(string(css), rule) {
			t.Errorf("missing sidebar accessibility rule %q", rule)
		}
	}
}
