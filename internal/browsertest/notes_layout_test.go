//go:build browser

package browsertest

import (
	"fmt"
	"testing"
	"time"
)

func TestNotesEditorLayout(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			tab := newTab(t, width, 900, "dark")
			openClients(t, tab)
			click(t, tab, `a[href$="/clients/new"]`)
			fillByLabel(t, tab, "Display name", fmt.Sprintf("Notes layout %d", time.Now().UnixNano()))
			clickByText(t, tab, "Create client")
			click(t, tab, `[data-notes-editor] summary`)
			waitVisible(t, tab, `[data-notes-rich] .ProseMirror`)
			click(t, tab, `[data-notes-rich] .ProseMirror`)
			check := func(source bool) {
				t.Helper()
				if !eval[bool](t, tab, fmt.Sprintf(`(() => {
					const root = document.querySelector('[data-notes-editor]');
					const toolbar = root.querySelector('[data-notes-toolbar]');
					const area = root.querySelector(%q);
					const input = root.querySelector(%q);
					const actions = root.querySelector('[data-notes-cancel]').parentElement.parentElement;
					const outer = toolbar.parentElement;
					const a = toolbar.getBoundingClientRect(), b = area.getBoundingClientRect(), c = actions.getBoundingClientRect(), d = outer.getBoundingClientRect();
					const style = getComputedStyle(input), outline = getComputedStyle(outer);
					return a.bottom <= b.top + 1 && b.bottom <= c.top + 1 && c.bottom <= d.bottom &&
						c.left >= d.left && c.right <= d.right && Math.abs(b.height - 192) < 1 &&
						parseFloat(style.borderWidth) === 0 && style.outlineStyle === 'none' &&
						parseFloat(outline.outlineWidth) >= 2 && outline.outlineStyle !== 'none' && document.documentElement.scrollWidth <= innerWidth;
				})()`, map[bool]string{false: "[data-notes-rich]", true: "[data-notes-source]"}[source], map[bool]string{false: "[data-notes-rich] .ProseMirror", true: "[data-notes-source]"}[source])) {
					t.Fatal("notes toolbar, editor, and actions overlap or lack a single visible focus boundary")
				}
			}
			check(false)
			click(t, tab, `[data-notes-source-mode]`)
			check(true)
		})
	}
}
