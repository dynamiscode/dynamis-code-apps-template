//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

func createArchiveClient(t *testing.T, tab context.Context, listPath, name string) string {
	t.Helper()
	navigate(t, tab, listPath+"/new")
	fillByLabel(t, tab, "Display name", name)
	clickByText(t, tab, "Create client")
	eventually(t, tab, `/\/clients\/[^/?]+$/.test(location.pathname)`, "client creation did not open detail")
	return eval[string](t, tab, "location.pathname")
}

func TestClientArchiveModal(t *testing.T) {
	for _, surface := range []string{"list", "detail"} {
		t.Run(surface, func(t *testing.T) {
			tab := newTab(t, 1280, 900, "")
			openClients(t, tab)
			listPath := eval[string](t, tab, "location.pathname")
			prefix := fmt.Sprintf("Archive modal %d", time.Now().UnixNano())
			other := prefix + " other"
			name := prefix + " selected"
			createArchiveClient(t, tab, listPath, other)
			detailPath := createArchiveClient(t, tab, listPath, name)
			listQuery := "?" + url.Values{"filter": {"displayName:contains:" + prefix}}.Encode()
			if surface == "list" {
				navigate(t, tab, listPath+listQuery)
				waitVisible(t, tab, "#client-list")
			} else {
				navigate(t, tab, detailPath)
			}

			trigger := fmt.Sprintf(`button[data-client-archive][data-client-name=%q]`, name)
			if surface == "list" {
				trigger = ".list-view-table " + trigger
			}
			if surface == "list" {
				click(t, tab, `.list-view-table tr:has(button[data-client-archive][data-client-name=`+fmt.Sprintf("%q", name)+`]) summary`)
			} else {
				click(t, tab, `#detail-main details.list-menu > summary`)
			}
			start := eval[string](t, tab, "location.href")
			run(t, tab, chromedp.Focus(trigger), chromedp.KeyEvent("\r"))
			eventually(t, tab, fmt.Sprintf(`document.querySelector('[data-client-archive-dialog]')?.matches(':modal') && document.querySelector('[data-client-archive-name]')?.textContent.includes(%s)`, jsString(name)), "archive dialog did not open for selected client")
			if got := eval[string](t, tab, "location.href"); got != start {
				t.Fatalf("opening archive dialog navigated: %q -> %q", start, got)
			}
			click(t, tab, "[data-client-archive-cancel]")
			eventually(t, tab, fmt.Sprintf(`!document.querySelector('[data-client-archive-dialog]').open && document.activeElement === document.querySelector(%s)`, jsString(trigger)), "cancel did not close dialog and return focus")
			navigate(t, tab, detailPath)
			if eval[bool](t, tab, `document.body.textContent.includes('This client is archived. It is hidden from the default list.')`) {
				t.Fatal("cancel archived the client")
			}
			if surface == "list" {
				navigate(t, tab, listPath+listQuery)
				waitVisible(t, tab, "#client-list")
				click(t, tab, `.list-view-table tr:has(button[data-client-archive][data-client-name=`+fmt.Sprintf("%q", name)+`]) summary`)
			} else {
				click(t, tab, `#detail-main details.list-menu > summary`)
			}
			run(t, tab, chromedp.Focus(trigger), chromedp.KeyEvent("\r"))
			waitVisible(t, tab, "[data-client-archive-dialog]:modal")
			press(t, tab, "\x1b")
			eventually(t, tab, fmt.Sprintf(`!document.querySelector('[data-client-archive-dialog]').open && document.activeElement === document.querySelector(%s)`, jsString(trigger)), "Escape did not close dialog and return focus")
			run(t, tab, chromedp.KeyEvent("\r"))
			waitVisible(t, tab, "[data-client-archive-dialog]:modal")
			run(t, tab, chromedp.MouseClickXY(5, 5))
			eventually(t, tab, fmt.Sprintf(`!document.querySelector('[data-client-archive-dialog]').open && document.activeElement === document.querySelector(%s)`, jsString(trigger)), "backdrop did not close dialog and return focus")
			run(t, tab, chromedp.KeyEvent("\r"))
			waitVisible(t, tab, "[data-client-archive-dialog]:modal")
			click(t, tab, "[data-client-archive-confirm]")
			eventually(t, tab, fmt.Sprintf(`location.pathname === %s && document.body.textContent.includes('This client is archived. It is hidden from the default list.')`, jsString(detailPath)), "confirmation did not archive selected client")
			if surface == "list" {
				eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll('a')].some(a => a.pathname === %s && a.search === %s)`, jsString(listPath), jsString(listQuery)), "archive lost the list return query")
				navigate(t, tab, listPath+listQuery)
				eventually(t, tab, fmt.Sprintf(`document.querySelector('#client-list') && document.querySelector('#client-list').textContent.includes(%s) && !document.querySelector('#client-list').textContent.includes(%s)`, jsString(other), jsString(name)), "archive changed the wrong client or selected client remained active")
			}
		})
	}
}

func TestClientArchiveWithoutJavaScript(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	listPath := eval[string](t, tab, "location.pathname")
	name := fmt.Sprintf("Archive fallback %d", time.Now().UnixNano())
	detailPath := createArchiveClient(t, tab, listPath, name)
	call(t, tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	navigate(t, tab, detailPath)
	click(t, tab, `#detail-main details.list-menu > summary`)
	click(t, tab, "[data-client-archive]")
	waitVisible(t, tab, "#client-archive-confirm")
	if !eval[bool](t, tab, fmt.Sprintf(`document.querySelector('#client-archive-confirm').textContent.includes(%s) && !document.body.textContent.includes('This client is archived. It is hidden from the default list.')`, jsString(name))) {
		t.Fatal("fallback confirmation did not identify the active client")
	}
	click(t, tab, `button[name="confirm_archive"][value="yes"]`)
	eventually(t, tab, `document.body.textContent.includes('This client is archived. It is hidden from the default list.')`, "fallback confirmation did not archive client")
}
