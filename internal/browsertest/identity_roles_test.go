//go:build browser

package browsertest

import (
	"testing"

	"github.com/chromedp/cdproto/emulation"
)

func TestIdentityRolesWithoutJavaScript(t *testing.T) {
	tab := newTab(t, 390, 844, "light")
	call(t, tab, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	navigate(t, tab, "/login")
	waitVisible(t, tab, "#email")
	expectNoOverflow(t, tab, 390)
	capturePublicIntake(t, tab, "mobile-nojs-login.png")
	fill(t, tab, "#email", fixtureOwnerEmail)
	fill(t, tab, "#password", "incorrect-password")
	click(t, tab, `button[type=submit]`)
	waitVisible(t, tab, `[role=alert]`)
	fill(t, tab, "#password", fixturePassword)
	click(t, tab, `button[type=submit]`)
	eventually(t, tab, `location.pathname === "/" || /^\/workspaces\/[^/]+$/.test(location.pathname)`, "sign-in did not complete")
	navigate(t, tab, "/account")
	waitVisible(t, tab, "[data-account-page]")
	expectNoOverflow(t, tab, 390)
	capturePublicIntake(t, tab, "mobile-nojs-account.png")
	click(t, tab, `a.account-shortcut[href="/sessions"]`)
	waitVisible(t, tab, ".session-card")
	expectNoOverflow(t, tab, 390)
}
