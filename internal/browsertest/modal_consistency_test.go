//go:build browser

package browsertest

import (
	"fmt"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

func TestModalControlConsistency(t *testing.T) {
	for _, viewport := range []struct {
		name   string
		width  int64
		height int64
	}{
		{"desktop", 1280, 900},
		{"narrow", 375, 800},
	} {
		for _, modal := range []struct {
			name, path, trigger, dialog, close, submit string
		}{
			{"stage", "/engagement-stages", "#stages details > summary", `[aria-labelledby="stage-add-title"]`, "[data-detail-cancel]", `form:has(#stage-name) button[type=submit]`},
			{"engagement", "/engagements", "[data-engagement-create]", "[data-engagement-dialog]", "[data-engagement-cancel]", `form:has(#engagement-name) button[type=submit]:not([formnovalidate])`},
		} {
			t.Run(viewport.name+"/"+modal.name, func(t *testing.T) {
				tab := newTab(t, viewport.width, viewport.height, "")
				signIn(t, tab)
				openWorkspaceSettings(t, tab, modal.path)
				run(t, tab, chromedp.Focus(modal.trigger), chromedp.KeyEvent("\r"))
				t.Cleanup(func() {
					if t.Failed() {
						state := eval[string](t, tab, fmt.Sprintf(`(() => { const d = document.querySelector(%s), a = document.activeElement; return JSON.stringify({url: location.href, open: d?.open, modal: d?.matches(':modal'), active: a?.outerHTML, trigger: document.querySelector(%s)?.outerHTML}); })()`, jsString(modal.dialog), jsString(modal.trigger)))
						t.Logf("dialog focus state: %s", state)
					}
				})
				waitVisible(t, tab, modal.dialog+":modal")

				controlsUsable := eval[bool](t, tab, fmt.Sprintf(`(() => {
					const dialog = document.querySelector(%s);
					const close = dialog?.querySelector(%s);
					const submit = dialog?.querySelector(%s);
					if (!close || !submit) return false;
					for (const control of [close, submit]) {
						control.scrollIntoView({block: 'nearest', inline: 'nearest'});
						const rect = control.getBoundingClientRect();
						if (rect.width <= 0 || rect.height <= 0 || rect.left < 0 || rect.right > innerWidth || rect.top < 0 || rect.bottom > innerHeight) return false;
					}
					return true;
				})()`, jsString(modal.dialog), jsString(modal.close), jsString(modal.submit)))
				if !controlsUsable {
					t.Fatal("modal controls are missing or cannot be brought into the viewport")
				}

				closeSelector := modal.dialog + " " + modal.close
				run(t, tab, chromedp.Focus(closeSelector))
				expectFocused(t, tab, closeSelector)
				// Dispatch Enter as one key press; KeyEvent sends a separate char after focus returns.
				call(t, tab, input.DispatchKeyEvent, input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyDown, Key: "Enter", Code: "Enter", Text: "\r", UnmodifiedText: "\r", WindowsVirtualKeyCode: 13})
				call(t, tab, input.DispatchKeyEvent, input.DispatchKeyEventParams{Type: input.DispatchKeyEventTypeKeyUp, Key: "Enter", Code: "Enter", WindowsVirtualKeyCode: 13})
				eventually(t, tab, fmt.Sprintf(`!document.querySelector(%s)?.open && document.activeElement === document.querySelector(%s)`, jsString(modal.dialog), jsString(modal.trigger)), "keyboard close did not close the modal and restore focus")
			})
		}
	}
}

func TestEngagementCreateNotesLayout(t *testing.T) {
	for _, viewport := range []struct {
		name   string
		width  int64
		height int64
	}{
		{"desktop", 1280, 900},
		{"narrow", 375, 800},
	} {
		for _, javascript := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/javascript-%t", viewport.name, javascript), func(t *testing.T) {
				tab := newTab(t, viewport.width, viewport.height, "")
				signIn(t, tab)
				workspace := openWorkspaceSettings(t, tab, "/engagements")
				if !javascript {
					call(t, tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
					navigate(t, tab, workspace+"/engagements/new")
				} else {
					click(t, tab, "[data-engagement-create]")
					waitVisible(t, tab, "[data-engagement-dialog]:modal")
				}
				notesSelector := "#engagement-notes"
				if javascript {
					notesSelector = "[data-notes-inline] [data-notes-rich] .ProseMirror"
				}
				waitVisible(t, tab, notesSelector)
				notesVisible := eval[bool](t, tab, fmt.Sprintf(`(() => {
					const notes = document.querySelector(%s);
					const client = document.querySelector('#engagement-client-picker');
					const stage = document.querySelector('#engagement-stage');
					if (!notes || !client || !stage || !notes.getBoundingClientRect().width || !notes.getBoundingClientRect().height || notes.closest('details:not([open])')) return false;
					return [notes, client, stage].every((field) => {
						const rect = field.getBoundingClientRect();
						return rect.width > 0 && rect.height > 0 && rect.left >= 0 && rect.right <= innerWidth;
					});
				})()`, jsString(notesSelector)))
				if !notesVisible {
					t.Fatal("notes, client, or stage field is missing, hidden, or outside the viewport")
				}
				if javascript {
					run(t, tab, chromedp.SendKeys(notesSelector, "Editable notes"))
					eventually(t, tab, `document.querySelector('[data-notes-inline] [data-notes-rich] .ProseMirror').textContent.includes('Editable notes') && document.querySelector('#engagement-notes').value.includes('Editable notes')`, "visual notes did not retain and synchronize typed text")
				} else {
					fill(t, tab, notesSelector, "Editable notes")
					if got := eval[string](t, tab, `document.querySelector('#engagement-notes').value`); got != "Editable notes" {
						t.Fatalf("notes value = %q, want editable text", got)
					}
				}
			})
		}
	}
}
