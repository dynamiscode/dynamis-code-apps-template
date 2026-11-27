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
)

func TestCustomFieldHeadingAndActions(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openClients(t, tab)
	click(t, tab, `summary[aria-label="Manage"]`)
	clickByText(t, tab, "Manage custom fields")
	waitVisible(t, tab, "[data-client-field-create]")
	path := eval[string](t, tab, "location.pathname")

	for _, width := range []int64{1280, 390} {
		run(t, tab, chromedp.EmulateViewport(width, 900))
		layout := eval[struct {
			Visible, NearHeading, NoOverflow bool
		}](t, tab, `(() => {
			const heading = document.querySelector('.feature-list-heading');
			const title = heading?.querySelector('.workspace-heading');
			const action = heading?.querySelector('.feature-list-heading-actions [data-client-field-create]');
			if (!title || !action) return {};
			const h = title.getBoundingClientRect(), a = action.getBoundingClientRect(), container = heading.getBoundingClientRect();
			return {visible: a.width > 0 && a.height > 0 && getComputedStyle(action).visibility === 'visible',
				nearHeading: innerWidth > 608
					? a.top >= h.top - 2 && a.bottom <= h.bottom + 2 && Math.abs(a.right - container.right) <= 2
					: a.top >= h.bottom - 2 && a.top <= h.bottom + 24 && a.right <= innerWidth,
				noOverflow: document.documentElement.scrollWidth <= document.documentElement.clientWidth};
		})()`)
		if !layout.Visible || !layout.NearHeading || !layout.NoOverflow {
			t.Fatalf("width %d: heading action layout %+v", width, layout)
		}
	}

	name := fmt.Sprintf("Field actions %d", time.Now().UnixNano())
	click(t, tab, "[data-client-field-create]")
	waitVisible(t, tab, "[data-client-field-dialog]:modal")
	fillByLabel(t, tab, "Field label", name)
	click(t, tab, `[data-client-field-dialog] button[type="submit"]`)
	eventually(t, tab, fmt.Sprintf(`Array.from(document.querySelectorAll('#client-field-list tbody tr')).some(row => row.querySelector('th')?.textContent.trim() === %s)`, jsString(name)), "new field missing from table")
	archivePath := eval[string](t, tab, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll('#client-field-list tbody tr')].find(row => row.querySelector('th')?.textContent.trim() === %s);
		return row.querySelector('form[action$="/archive"]').getAttribute('action');
	})()`, jsString(name)))
	fieldPath := strings.TrimSuffix(archivePath, "/archive")
	row := fmt.Sprintf(`tr:has(form[action^=%s])`, jsString(fieldPath+"/"))
	menu := row + ` details.list-menu > summary`

	click(t, tab, menu)
	if !eval[bool](t, tab, fmt.Sprintf(`(() => {
		const row = document.querySelector(%s);
		const archive = row.querySelector('form[action$="/archive"] button');
		const style = getComputedStyle(archive);
		return !row.querySelector('form[action$="/delete"]') && archive.getBoundingClientRect().height >= 44 && parseFloat(style.borderWidth) === 0;
	})()`, jsString(row))) {
		t.Fatal("active field must offer an unboxed archive tile and no delete")
	}
	click(t, tab, row+` form[action$="/archive"] button`)
	eventually(t, tab, fmt.Sprintf(`Boolean(document.querySelector(%s)?.querySelector('form[action$="/restore"]'))`, jsString(row)), "archive did not expose restore")
	click(t, tab, menu)
	click(t, tab, row+` form[action$="/restore"] button`)
	eventually(t, tab, fmt.Sprintf(`Boolean(document.querySelector(%s)?.querySelector('form[action$="/archive"]'))`, jsString(row)), "restore did not expose archive")
	click(t, tab, menu)
	click(t, tab, row+` form[action$="/archive"] button`)
	eventually(t, tab, fmt.Sprintf(`Boolean(document.querySelector(%s)?.querySelector('form[action$="/delete"]'))`, jsString(row)), "archived field lacks delete")
	dialogs := make(chan string, 1)
	_ = chromedp.Listen(tab, func(ev any) {
		if dialog, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			select {
			case dialogs <- dialog.Message:
			default:
			}
			go func() {
				_, _ = chromedp.Run[cdp.Empty](tab, func(ctx context.Context, target *chromedp.Target) (cdp.Empty, error) {
					return cdp.Call(ctx, target, page.HandleJavaScriptDialog, page.HandleJavaScriptDialogParams{Accept: false})
				})
			}()
		}
	})
	click(t, tab, menu)
	disclosure := row + ` details.client-delete > summary`
	run(t, tab, chromedp.Focus(disclosure), chromedp.KeyEvent("\r"))
	if !eval[bool](t, tab, fmt.Sprintf(`(() => {
		const row = document.querySelector(%s);
		const restore = row.querySelector('form[action$="/restore"] button');
		const remove = row.querySelector('details.client-delete > summary');
		const banner = row.querySelector('details.client-delete[open]');
		return [restore, remove].every(item => item && item.getBoundingClientRect().height >= 44 && parseFloat(getComputedStyle(item).borderWidth) === 0)
			&& Boolean(banner?.textContent.includes('cannot be undone'))
			&& Boolean(banner.querySelector('input[name="confirm_delete"][required]'));
	})()`, jsString(row))) {
		t.Fatal("delete banner must show the irreversible warning and require confirmation")
	}
	click(t, tab, row+` form[action$="/delete"] button[type="submit"]`)
	if !eval[bool](t, tab, fmt.Sprintf(`Boolean(document.querySelector(%s + ' input[name="confirm_delete"]:invalid'))`, jsString(row))) {
		t.Fatal("deletion must require explicit confirmation")
	}
	run(t, tab, chromedp.Focus(disclosure), chromedp.KeyEvent("\r"))
	if !eval[bool](t, tab, fmt.Sprintf(`Boolean(document.querySelector(%s)) && !document.querySelector(%s + ' details.client-delete[open]')`, jsString(row), jsString(row))) {
		t.Fatal("canceling deletion must close the banner and keep the field")
	}
	run(t, tab, chromedp.Focus(disclosure), chromedp.KeyEvent("\r"))
	run(t, tab, chromedp.Focus(row+` input[name="confirm_delete"]`), chromedp.KeyEvent(" "))
	run(t, tab, chromedp.Focus(row+` form[action$="/delete"] button[type="submit"]`), chromedp.KeyEvent("\r"))
	eventually(t, tab, fmt.Sprintf(`!document.querySelector(%s)`, jsString(row)), "confirmed deletion did not remove the field")

	name = fmt.Sprintf("Field no JavaScript %d", time.Now().UnixNano())
	click(t, tab, "[data-client-field-create]")
	waitVisible(t, tab, "[data-client-field-dialog]:modal")
	fillByLabel(t, tab, "Field label", name)
	click(t, tab, `[data-client-field-dialog] button[type="submit"]`)
	eventually(t, tab, fmt.Sprintf(`Array.from(document.querySelectorAll('#client-field-list tbody tr')).some(row => row.querySelector('th')?.textContent.trim() === %s)`, jsString(name)), "new field missing from table")
	archivePath = eval[string](t, tab, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll('#client-field-list tbody tr')].find(row => row.querySelector('th')?.textContent.trim() === %s);
		return row.querySelector('form[action$="/archive"]').getAttribute('action');
	})()`, jsString(name)))
	fieldPath = strings.TrimSuffix(archivePath, "/archive")
	row = fmt.Sprintf(`tr:has(form[action^=%s])`, jsString(fieldPath+"/"))
	menu = row + ` details.list-menu > summary`
	click(t, tab, menu)
	click(t, tab, row+` form[action$="/archive"] button`)
	eventually(t, tab, fmt.Sprintf(`Boolean(document.querySelector(%s)?.querySelector('form[action$="/delete"]'))`, jsString(row)), "archived field lacks delete")

	call(t, tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	navigate(t, tab, path)
	click(t, tab, menu)
	click(t, tab, row+` details.client-delete > summary`)
	if !eval[bool](t, tab, fmt.Sprintf(`(() => {
		const banner = document.querySelector(%s + ' details.client-delete[open]');
		return Boolean(banner && banner.textContent.includes('cannot be undone') && banner.querySelector('input[name="confirm_delete"][required]'));
	})()`, jsString(row))) {
		t.Fatal("no-JavaScript delete banner must warn and require confirmation")
	}
	click(t, tab, row+` form[action$="/delete"] button[type="submit"]`)
	if !eval[bool](t, tab, fmt.Sprintf(`Boolean(document.querySelector(%s + ' input[name="confirm_delete"]:invalid'))`, jsString(row))) {
		t.Fatal("no-JavaScript deletion must require explicit confirmation")
	}
	click(t, tab, row+` input[name="confirm_delete"]`)
	click(t, tab, row+` form[action$="/delete"] button[type="submit"]`)
	eventually(t, tab, fmt.Sprintf(`!document.querySelector(%s)`, jsString(row)), "no-JavaScript confirmed deletion did not remove the field")
	select {
	case message := <-dialogs:
		t.Fatalf("delete opened a native browser dialog: %q", message)
	default:
	}
}
