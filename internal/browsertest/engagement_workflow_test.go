//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

func selectEngagementStage(t *testing.T, tab context.Context, name string) {
	t.Helper()
	index := eval[int](t, tab, fmt.Sprintf(`[...document.querySelector('#engagement-stage').options].findIndex(o => o.textContent.trim() === %s && !o.disabled)`, jsString(name)))
	if index < 0 {
		t.Fatalf("stage %q not available", name)
	}
	click(t, tab, "#engagement-stage")
	press(t, tab, kb.Home)
	for range index {
		press(t, tab, kb.ArrowDown)
	}
	press(t, tab, "\r")
	eventually(t, tab, fmt.Sprintf(`document.querySelector('#engagement-stage').selectedOptions[0].textContent.trim() === %s`, jsString(name)), "stage selection did not change")
}

func TestEngagementFallbackAndRoles(t *testing.T) {
	owner := newTab(t, 1280, 900, "")
	signIn(t, owner)
	workspace := openWorkspaceSettings(t, owner, "/engagement-stages")
	suffix := fmt.Sprint(time.Now().UnixNano())
	firstStage, secondStage := "Browser first "+suffix, "Browser second "+suffix
	call(t, owner, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	for _, name := range []string{firstStage, secondStage} {
		navigate(t, owner, workspace+"/engagement-stages")
		click(t, owner, "#stages details > summary")
		fill(t, owner, "#stage-name", name)
		click(t, owner, `form:has(#stage-name) button[type=submit]`)
		eventually(t, owner, fmt.Sprintf(`document.body.textContent.includes(%s)`, jsString(name)), "stage was not created without JavaScript")
	}

	navigate(t, owner, workspace+"/clients/new")
	fill(t, owner, "#client-display-name", "Engagement client "+suffix)
	click(t, owner, `form:has(#client-display-name) button[type=submit]`)
	eventually(t, owner, `/\/clients\/[^/?]+$/.test(location.pathname)`, "client creation did not open detail")
	click(t, owner, `a[href*="/engagements/new?client="]`)
	waitVisible(t, owner, "#engagement-name")
	name := "Fallback engagement " + suffix
	fill(t, owner, "#engagement-name", name)
	selectEngagementStage(t, owner, firstStage)
	click(t, owner, `form:has(#engagement-name) .modal-action button[type=submit]`)
	eventually(t, owner, fmt.Sprintf(`/\/engagements\/[^/?]+$/.test(location.pathname) && document.body.textContent.includes(%s)`, jsString(name)), "engagement was not created without JavaScript")
	detail := eval[string](t, owner, "location.pathname")
	click(t, owner, `[data-inline-title]:has(#engagement-name) > summary`)
	updated := "Edited engagement " + suffix
	fill(t, owner, "#engagement-name", updated)
	click(t, owner, `[data-inline-title]:has(#engagement-name) button[type=submit]`)
	eventually(t, owner, fmt.Sprintf(`document.querySelector('h1')?.textContent.includes(%s)`, jsString(updated)), "engagement name was not saved without JavaScript")
	selectEngagementStage(t, owner, secondStage)
	eval[any](t, owner, `window.__stageBeforeSubmit = true`)
	click(t, owner, `form:has(#engagement-stage) [data-confirm-select-fallback] button[type=submit]`)
	eventually(t, owner, fmt.Sprintf(`window.__stageBeforeSubmit !== true && document.readyState === 'complete' && document.querySelector('#engagement-stage')?.selectedOptions[0].textContent.trim() === %s`, jsString(secondStage)), "stage move was not saved without JavaScript")
	navigate(t, owner, detail)
	eventually(t, owner, fmt.Sprintf(`document.querySelector('h1')?.textContent.includes(%s) && document.querySelector('#engagement-stage')?.selectedOptions[0].textContent.trim() === %s`, jsString(updated), jsString(secondStage)), "name or stage did not persist after reload")
	click(t, owner, `[data-inline-title]:has(#engagement-name) > summary`)
	fill(t, owner, "#engagement-name", "   ")
	click(t, owner, `[data-inline-title]:has(#engagement-name) button[type=submit]`)
	eventually(t, owner, fmt.Sprintf(`document.querySelector('[data-inline-title] summary h1')?.textContent.includes(%s) && document.querySelector('#engagement-name').value === '   ' && Boolean(document.querySelector('[role=alert]'))`, jsString(updated)), "rejected name did not preserve saved heading and draft")
	navigate(t, owner, detail)
	click(t, owner, `details:has(#engagement-next-action) > summary`)
	fill(t, owner, "#engagement-next-action", "   ")
	click(t, owner, "#engagement-next-action-owner")
	press(t, owner, kb.ArrowDown)
	press(t, owner, "\r")
	eventually(t, owner, `document.querySelector('#engagement-next-action-owner').value !== ''`, "owner was not selected")
	click(t, owner, `form:has(#engagement-next-action) button[type=submit]`)
	eventually(t, owner, `document.querySelector('#engagement-next-action').value === '   ' && document.querySelector('#engagement-next-action-owner').value !== '' && Boolean(document.querySelector('#engagement-next-action-error'))`, "rejected action did not retain its draft and error")
	if eval[bool](t, owner, `!document.querySelector('dd')?.textContent.includes('No next action')`) {
		t.Fatal("rejected action replaced the saved summary")
	}
	navigate(t, owner, detail)

	call(t, owner, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: false})
	navigate(t, owner, workspace+"/engagements")
	waitVisible(t, owner, "[data-engagement-create]")
	if !eval[bool](t, owner, `Boolean(document.querySelector('.feature-list-heading-actions details > summary'))`) {
		t.Fatal("owner cannot manage engagement settings")
	}
	viewer := inviteMember(t, owner, workspace, "engagement-viewer-"+suffix+"@example.com", "viewer")
	member := newTab(t, 1280, 900, "")
	signInAs(t, member, fixtureMemberEmail, fixtureMemberPass)
	for _, tc := range []struct {
		name   string
		tab    context.Context
		writer bool
	}{{"owner", owner, true}, {"member", member, true}, {"viewer", viewer, false}} {
		t.Run(tc.name, func(t *testing.T) {
			navigate(t, tc.tab, workspace+"/engagements")
			waitVisible(t, tc.tab, "#engagement-search")
			eventually(t, tc.tab, fmt.Sprintf(`document.querySelector('#engagement-view')?.textContent.includes(%s)`, jsString(updated)), "engagement is missing from the list")
			if got := eval[bool](t, tc.tab, `Boolean(document.querySelector('[data-engagement-create]'))`); got != tc.writer {
				t.Fatalf("create control visible=%v, want %v", got, tc.writer)
			}
			if got := eval[bool](t, tc.tab, `Boolean(document.querySelector('.feature-list-heading-actions details > summary'))`); got != (tc.name == "owner") {
				t.Fatalf("settings control visible=%v for %s", got, tc.name)
			}
			navigate(t, tc.tab, detail)
			eventually(t, tc.tab, fmt.Sprintf(`document.querySelector('h1')?.textContent.includes(%s)`, jsString(updated)), "engagement detail unavailable")
			if got := eval[bool](t, tc.tab, `Boolean(document.querySelector('form:has(#engagement-stage)'))`); got != tc.writer {
				t.Fatalf("stage editor visible=%v, want %v", got, tc.writer)
			}
			if !tc.writer && !eval[bool](t, tc.tab, fmt.Sprintf(`document.body.textContent.includes(%s)`, jsString(secondStage))) {
				t.Fatal("viewer cannot read engagement stage")
			}
		})
	}
	// A member can save changes; a viewer sees the saved result without edit controls.
	navigate(t, member, detail)
	click(t, member, `[data-inline-title]:has(#engagement-name) > summary`)
	memberName := "Member edit " + suffix
	fill(t, member, "#engagement-name", memberName)
	click(t, member, `[data-inline-title]:has(#engagement-name) button[type=submit]`)
	eventually(t, member, fmt.Sprintf(`document.querySelector('h1')?.textContent.includes(%s)`, jsString(memberName)), "member edit was not saved")
	navigate(t, viewer, detail)
	if !strings.Contains(eval[string](t, viewer, "document.body.innerText"), memberName) {
		t.Fatal("viewer did not see member's saved name")
	}
}

func TestEngagementDialogsRestoreKeyboardFocus(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signIn(t, tab)
	workspace := openWorkspaceSettings(t, tab, "/engagements")
	trigger := "[data-engagement-create]"
	run(t, tab, chromedp.Focus(trigger), chromedp.KeyEvent("\r"))
	waitVisible(t, tab, "[data-engagement-dialog]:modal")
	press(t, tab, "\x1b")
	eventually(t, tab, `!document.querySelector('[data-engagement-dialog]').open && document.activeElement === document.querySelector('[data-engagement-create]')`, "Escape did not close create dialog and restore focus")
	run(t, tab, chromedp.KeyEvent("\r"))
	waitVisible(t, tab, "[data-engagement-dialog]:modal")
	click(t, tab, "[data-engagement-cancel]")
	expectFocused(t, tab, trigger)

	navigate(t, tab, workspace+"/engagement-stages")
	stageTrigger := "#stages details > summary"
	run(t, tab, chromedp.Focus(stageTrigger), chromedp.KeyEvent("\r"))
	waitVisible(t, tab, `[data-detail-dialog][aria-labelledby="stage-add-title"]:modal`)
	press(t, tab, "\x1b")
	eventually(t, tab, `!document.querySelector('[aria-labelledby="stage-add-title"]').open && document.activeElement === document.querySelector('#stages details > summary')`, "Escape did not close stage dialog and restore focus")
}

func TestStageAddModalCloseAndCreate(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signIn(t, tab)
	workspace := openWorkspaceSettings(t, tab, "/engagement-stages")
	trigger := "#stages details > summary"
	dialog := `[aria-labelledby="stage-add-title"]`
	closeControl := dialog + ` [data-detail-cancel]`

	click(t, tab, trigger)
	waitVisible(t, tab, dialog+":modal")
	if !eval[bool](t, tab, `(() => {
		const dialog = document.querySelector('[aria-labelledby="stage-add-title"]');
		const close = dialog.querySelector('[data-detail-cancel]');
		const input = dialog.querySelector('#stage-name');
		return Boolean(close?.getAttribute('aria-label')?.trim() && input?.labels?.length && input.required)
			&& ![...dialog.querySelectorAll('button, a')].some(el => el.textContent.trim() === 'Cancel' && el.getClientRects().length);
	})()`) {
		t.Fatal("stage dialog must have a labeled close control and required labeled name, without a visible Cancel")
	}
	click(t, tab, closeControl)
	eventually(t, tab, `!document.querySelector('[aria-labelledby="stage-add-title"]').open && !document.querySelector('#stages details').open && document.activeElement === document.querySelector('#stages details > summary')`, "close control did not close stage dialog and restore focus")

	click(t, tab, trigger)
	waitVisible(t, tab, dialog+":modal")
	run(t, tab, chromedp.MouseClickXY(5, 5))
	eventually(t, tab, `!document.querySelector('[aria-labelledby="stage-add-title"]').open && !document.querySelector('#stages details').open && document.activeElement === document.querySelector('#stages details > summary')`, "backdrop did not close stage dialog and restore focus")

	click(t, tab, trigger)
	waitVisible(t, tab, dialog+":modal")
	name := fmt.Sprintf("Browser modal stage %d", time.Now().UnixNano())
	fill(t, tab, "#stage-name", name)
	dialogs := make(chan string, 1)
	_ = chromedp.Listen(tab, func(ev any) {
		if confirmation, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			select {
			case dialogs <- confirmation.Message:
			default:
			}
			go func() {
				_, _ = chromedp.Run[cdp.Empty](tab, func(ctx context.Context, target *chromedp.Target) (cdp.Empty, error) {
					return cdp.Call(ctx, target, page.HandleJavaScriptDialog, page.HandleJavaScriptDialogParams{Accept: false})
				})
			}()
		}
	})
	click(t, tab, closeControl)
	select {
	case <-dialogs:
	case <-time.After(5 * time.Second):
		t.Fatal("closing a dirty stage draft did not request confirmation")
	}
	eventually(t, tab, fmt.Sprintf(`document.querySelector('[aria-labelledby="stage-add-title"]').matches(':modal') && document.querySelector('#stage-name').value === %s`, jsString(name)), "rejecting discard did not retain the open dialog and draft")
	click(t, tab, `form:has(#stage-name) button[type=submit]`)
	eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll('[data-stage-group=\"open\"] [data-stage-card]')].some(card => card.textContent.includes(%s))`, jsString(name)), "stage was not created in the Open group")
	navigate(t, tab, workspace+"/engagement-stages")
	eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll('[data-stage-group=\"open\"] [data-stage-card]')].some(card => card.textContent.includes(%s))`, jsString(name)), "stage did not persist in the Open group")
}
