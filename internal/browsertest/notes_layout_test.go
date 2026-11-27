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
					const toolbar = root?.querySelector('[data-notes-toolbar]');
					const area = root?.querySelector(%q);
					const cancel = root?.querySelector('[data-notes-cancel]');
					const submit = root?.querySelector('button[type="submit"]');
					if (!root || !toolbar || !area || !cancel || !submit) return false;
					return [toolbar, area, cancel, submit].every((control) => {
						if (control.hidden || control.getClientRects().length === 0) return false;
						const rect = control.getBoundingClientRect();
						return rect.width > 0 && rect.height > 0 && rect.left >= 0 && rect.right <= innerWidth;
					}) && document.documentElement.scrollWidth <= innerWidth;
				})()`, map[bool]string{false: "[data-notes-rich] .ProseMirror", true: "[data-notes-source]"}[source])) {
					t.Fatal("notes toolbar, editor, or actions are missing, hidden, or clipped")
				}
			}
			check(false)
			click(t, tab, `[data-notes-source-mode]`)
			check(true)
		})
	}
}
