//go:build browser

package browsertest

import (
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestWorkspaceTitleInSidebarAndMobileNavigation(t *testing.T) {
	for _, layout := range []struct {
		name          string
		width, height int64
	}{
		{"desktop", 1280, 900},
		{"mobile", 390, 844},
	} {
		t.Run(layout.name, func(t *testing.T) {
			tab := newTab(t, layout.width, layout.height, "")
			signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
			workspace := openWorkspaceSettings(t, tab, "/clients")
			visible := `el => { if (!el) return false; const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return s.visibility === "visible" && s.display !== "none" && r.width > 0 && r.height > 0 && r.right > 0 && r.left < innerWidth; }`
			headerTitle := `document.querySelector('header a[href="' + ` + jsString(workspace) + ` + '"]')`
			switcher := `document.querySelector('.drawer-side details > summary')`
			if layout.name == "mobile" {
				eventually(t, tab, fmt.Sprintf(`(%s)(%s) && !(%s)(%s)`, visible, headerTitle, visible, switcher), "collapsed navigation must show the workspace title in the header")
				click(t, tab, `header button[aria-label="Open navigation"]`)
			}
			eventually(t, tab, fmt.Sprintf(`(%s)(%s) && !(%s)(%s)`, visible, switcher, visible, headerTitle), "open navigation must show only the sidebar workspace switcher")
			if got := eval[int](t, tab, `(document.querySelector('.drawer-side').innerText.match(/UI Baselines/g) || []).length`); got != 1 {
				t.Fatalf("visible workspace name count = %d, want one switcher label", got)
			}
			// Native details supports keyboard operation as well as pointer activation.
			run(t, tab, chromedp.Focus(`.drawer-side details > summary`))
			press(t, tab, "\r")
			eventually(t, tab, `document.querySelector('.drawer-side details')?.open === true`, "workspace switcher did not open from the keyboard")
			eventually(t, tab, fmt.Sprintf(`document.querySelector('.drawer-side details a[aria-current="page"]')?.getAttribute('href') === %s`, jsString(workspace)), "current workspace home is missing from the switcher")
			click(t, tab, `.drawer-side details a[aria-current="page"]`)
			eventually(t, tab, fmt.Sprintf(`location.pathname === %s`, jsString(workspace)), "workspace switcher did not navigate home")
		})
	}
}

func TestHeaderLanguageDropdownSwitchesAndReturnsToPage(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureMemberEmail, fixtureMemberPass)
	workspace := openWorkspaceSettings(t, tab, "/clients")
	navigate(t, tab, "/settings/language")
	original := eval[struct {
		Locale string `json:"locale"`
		CSRF   string `json:"csrf"`
	}](t, tab, `({locale: document.querySelector('#language').value, csrf: document.querySelector('form[action="/settings/language"] [name="csrf"]').value})`)
	t.Cleanup(func() {
		eval[any](t, tab, fmt.Sprintf(`(async () => { await fetch("/settings/language", {method: "POST", body: new URLSearchParams({csrf: %s, locale: %s}), credentials: "same-origin"}); })()`, jsString(original.CSRF), jsString(original.Locale)), awaitPromise)
	})

	page := workspace + "/clients"
	navigate(t, tab, page)
	language := `header details:has(form[action="/settings/language"])`
	theme := `header details:has(form[action="/account/theme"])`
	eventually(t, tab, fmt.Sprintf(`!!document.querySelector(%s) && !!document.querySelector(%s)`, jsString(language), jsString(theme)), "language and theme controls are missing")
	click(t, tab, language+` > summary`)
	click(t, tab, language+` button[name="locale"][value="es"]`)
	eventually(t, tab, fmt.Sprintf(`location.pathname === %s && document.documentElement.lang === "es"`, jsString(page)), "language change did not return to the page in Spanish")
	eventually(t, tab, `document.querySelector('header details:has(form[action="/settings/language"]) > summary')?.textContent.includes("Idioma")`, "language dropdown did not translate")
}
