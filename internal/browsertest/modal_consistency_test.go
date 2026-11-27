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

				geometry := eval[string](t, tab, fmt.Sprintf(`(() => {
					const dialog = document.querySelector(%s);
					const box = dialog.querySelector('.modal-box');
					const close = dialog.querySelector(%s);
					const submit = dialog.querySelector(%s);
					if (!box || !close || !submit) return 'missing modal controls';
					const style = getComputedStyle(close);
					for (const side of ['Top', 'Right', 'Bottom', 'Left']) {
						const color = style['border' + side + 'Color'];
						if (parseFloat(style['border' + side + 'Width']) > 0 && style['border' + side + 'Style'] !== 'none' && color !== 'transparent' && color !== 'rgba(0, 0, 0, 0)') return 'close has visible border: ' + side + ' ' + color;
					}
					close.scrollIntoView({block: 'nearest', inline: 'nearest'});
					const c = close.getBoundingClientRect(), b = box.getBoundingClientRect(), bs = getComputedStyle(box);
					if (c.width <= 0 || c.height <= 0 || c.left < 0 || c.right > innerWidth || c.top < 0 || c.bottom > innerHeight) return 'close does not fit viewport';
					if (Math.abs(c.right - (b.left + box.clientLeft + box.clientWidth - parseFloat(bs.paddingRight))) > 2) return 'close is not aligned to the right';
					submit.scrollIntoView({block: 'nearest', inline: 'nearest'});
					const s = submit.getBoundingClientRect(), form = submit.form.getBoundingClientRect(), fs = getComputedStyle(submit.form);
					if (s.width <= 0 || s.height <= 0 || s.left < 0 || s.right > innerWidth || s.top < 0 || s.bottom > innerHeight) return 'submit does not fit viewport';
					if (Math.abs(s.right - (form.right - parseFloat(fs.paddingRight) - parseFloat(fs.borderRightWidth))) > 2) return 'submit is not aligned to the right';
					return '';
				})()`, jsString(modal.dialog), jsString(modal.close), jsString(modal.submit)))
				if geometry != "" {
					t.Fatal(geometry)
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
				layout := eval[string](t, tab, fmt.Sprintf(`(() => {
					const name = document.querySelector('#engagement-name');
					const notes = document.querySelector(%s);
					const client = document.querySelector('#engagement-client-picker');
					const stage = document.querySelector('#engagement-stage');
					if (!name || !notes || !client || !stage) return 'missing create fields';
					const n = name.getBoundingClientRect(), text = notes.getBoundingClientRect();
					const c = client.getBoundingClientRect(), s = stage.getBoundingClientRect();
					if (!text.width || !text.height || notes.closest('details:not([open])')) return 'notes are not visible';
					if (text.top < n.bottom || text.left >= n.right || text.right <= n.left) return 'notes are not below name';
					if (s.top - c.bottom < 8) return 'client and stage lack vertical spacing';
					if (text.left < 0 || text.right > innerWidth || c.left < 0 || c.right > innerWidth || s.left < 0 || s.right > innerWidth) return 'fields overflow viewport';
					return '';
				})()`, jsString(notesSelector)))
				if layout != "" {
					t.Fatal(layout)
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
