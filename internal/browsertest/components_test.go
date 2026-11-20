//go:build browser

package browsertest

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
)

// TestResponsiveListComponent ports scripts/playwright/components.spec.mjs. It renders the
// responsive list fixture produced by TestResponsiveListBrowserFixture.
func TestResponsiveListComponent(t *testing.T) {
	fixturePath := filepath.Join(t.TempDir(), "fixture.html")
	generate := exec.Command("go", "test", "./internal/web", "-run", "^TestResponsiveListBrowserFixture$", "-count=1")
	generate.Dir = repoRoot
	generate.Env = append(os.Environ(), "COMPONENT_FIXTURE="+fixturePath)
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v\n%s", err, out)
	}
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	componentsCSS, err := os.ReadFile(filepath.Join(repoRoot, "internal/web/assets/components.css"))
	if err != nil {
		t.Fatal(err)
	}
	appCSS, err := os.ReadFile(filepath.Join(repoRoot, "internal/web/assets/app.css"))
	if err != nil {
		t.Fatal(err)
	}
	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString(fixture)

	cases := []struct {
		name, theme, visible, hidden string
		width                        int64
	}{
		{"mobile light", "light", "card-action", "table-action", 390},
		{"mobile dark", "dark", "card-action", "table-action", 390},
		{"desktop light", "light", "table-action", "card-action", 1280},
		{"desktop dark", "dark", "table-action", "card-action", 1280},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := newTab(t, c.width, 900, "")
			run(t, tab, chromedp.Navigate(dataURL))
			eval[any](t, tab, fmt.Sprintf(`(() => {
				document.documentElement.setAttribute("data-theme", %s);
				for (const css of [%s, %s]) {
					const style = document.createElement("style");
					style.textContent = css;
					document.head.appendChild(style);
				}
			})()`, jsString(c.theme), jsString(string(componentsCSS)), jsString(string(appCSS))))

			waitVisible(t, tab, "#"+c.visible)
			if hidden := eval[bool](t, tab, fmt.Sprintf(`(() => { const el = document.getElementById(%s); if (!el) return true; const style = getComputedStyle(el); const box = el.getBoundingClientRect(); return style.display === "none" || style.visibility === "hidden" || (box.width === 0 && box.height === 0); })()`, jsString(c.hidden))); !hidden {
				t.Fatalf("#%s is visible, want hidden", c.hidden)
			}
			waitVisible(t, tab, "#standalone-action")
			if count := eval[int](t, tab, `[...document.querySelectorAll("button")].filter((b) => (b.getAttribute("aria-label") || b.textContent).trim() === "Open item" && b.getClientRects().length > 0 && getComputedStyle(b).visibility !== "hidden" && !b.closest('[aria-hidden="true"]')).length`); count != 1 {
				t.Fatalf("Open item buttons = %d, want 1", count)
			}
			eval[any](t, tab, `document.getElementById("before").focus()`)
			press(t, tab, "\t")
			expectFocused(t, tab, "#"+c.visible)
			press(t, tab, "\t")
			expectFocused(t, tab, "#standalone-action")
			expectAxeClean(t, tab)
		})
	}
}
