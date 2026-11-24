//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"testing"

	"github.com/chromedp/cdproto/emulation"
)

func TestSignInKeyboardFocus(t *testing.T) {
	tab := newTab(t, 390, 844, "light")
	navigate(t, tab, "/login")
	waitVisible(t, tab, "#email")
	for _, selector := range []string{`.skip-link`, "#email", "#password", `#password + .reveal`, `button[type=submit]`, `a[href="/password-reset"]`} {
		assertTabFocus(t, tab, selector)
		if selector == `a[href="/password-reset"]` {
			capturePublicIntake(t, tab, "mobile-login-keyboard-recovery.png")
		}
	}
}

func assertTabFocus(t *testing.T, tab context.Context, selector string) {
	t.Helper()
	press(t, tab, "\t")
	got := eval[string](t, tab, `document.activeElement?.outerHTML ?? ""`)
	if !eval[bool](t, tab, fmt.Sprintf(`document.activeElement?.matches(%s)`, jsString(selector))) {
		t.Fatalf("Tab focused %s, want %s", got, selector)
	}
	visible := eval[bool](t, tab, `(() => { const s = getComputedStyle(document.activeElement); return (s.outlineStyle !== 'none' && parseFloat(s.outlineWidth) > 0) || s.boxShadow !== 'none'; })()`)
	if !visible {
		t.Fatalf("focused %s has no outline or box shadow", selector)
	}
	t.Logf("Tab focus %s: visible outline or shadow", selector)
}

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
