//go:build browser

package browsertest

import (
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
)

func TestCatalogCurrencySearchWithoutJavaScript(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signIn(t, tab)
	if eval[bool](t, tab, `location.pathname === "/"`) {
		click(t, tab, ".cards a")
	}
	waitVisible(t, tab, "[data-workspace-home]")
	workspace := eval[string](t, tab, `location.pathname`)
	for _, code := range []string{"USD", "EUR", "BHD"} {
		navigate(t, tab, workspace+"/settings/currencies")
		if eval[bool](t, tab, `Array.from(document.querySelector('select[name="new_code"]').options).some(option => option.value === `+jsString(code)+`)`) {
			run(t, tab, chromedp.SetValue(`select[name="new_code"]`, code), chromedp.Click(`.settings-page form[method="post"] button[type="submit"]`))
			eventually(t, tab, `!Array.from(document.querySelector('select[name="new_code"]')?.options || []).some(option => option.value === `+jsString(code)+`)`, "currency was not added")
		}
	}
	call(t, tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	navigate(t, tab, workspace+"/catalog?create=true")
	waitVisible(t, tab, "#catalog-create-name")
	fill(t, tab, "#catalog-create-name", "Draft entry")
	run(t, tab, chromedp.Click(`[data-catalog-dialog] details > summary`))
	waitVisible(t, tab, "#catalog-create-currency")
	run(t, tab, chromedp.SetValue("#catalog-create-currency", "USD"))
	fill(t, tab, "#catalog-create-currency-search", "eur")
	run(t, tab, chromedp.Click(`[data-catalog-dialog] .entry-picker-fallback-search button`))
	eventually(t, tab, `document.querySelector('#catalog-create-currency option[value="BHD"]')?.hidden === true`, "nonmatching currency was not filtered")
	if !eval[bool](t, tab, `(() => {
		const select = document.querySelector('#catalog-create-currency');
		const usd = select.querySelector('option[value="USD"]');
		const eur = select.querySelector('option[value="EUR"]');
		const bhd = select.querySelector('option[value="BHD"]');
		return select.value === 'USD' && usd.selected && getComputedStyle(usd).display !== 'none' &&
			getComputedStyle(eur).display !== 'none' && getComputedStyle(bhd).display === 'none' &&
			document.querySelector('#catalog-create-name').value === 'Draft entry';
	})()`) {
		t.Fatal("currency search did not preserve the selected currency and draft while hiding nonmatches")
	}
	if eval[bool](t, tab, `document.querySelector('[data-catalog-dialog] button[type="submit"]:not([formnovalidate])').disabled`) {
		t.Fatal("create must remain available without JavaScript")
	}
	click(t, tab, `[data-catalog-dialog] button[type="submit"]:not([formnovalidate])`)
	eventually(t, tab, `document.body.textContent.includes("Draft entry") && !document.querySelector('[data-catalog-dialog][open]')`, "no-JavaScript create did not persist the entry")
}
