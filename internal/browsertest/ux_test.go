//go:build browser

package browsertest

import (
	"fmt"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// TestFormUXPatterns ports scripts/ux-patterns.mjs. It walks the setup wizard, login,
// drafts, email settings, tokens, the account menu, and clients on a fresh server. The
// "Find and sort" disclosure check is removed: the disclosure was removed in #533 by design.
func TestFormUXPatterns(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	base := uxURL
	const email = "ux@example.com"
	const password = "ux-password-1234!"
	check := func(ok bool, message string) {
		t.Helper()
		if !ok {
			t.Fatalf("not ok - %s", message)
		}
	}
	js := func(expr string) bool { return eval[bool](t, tab, expr) }
	typeInto := func(selector, text string) {
		t.Helper()
		eval[any](t, tab, fmt.Sprintf(`(() => { const el = document.querySelector(%s); el.value = ""; el.dispatchEvent(new Event("input", { bubbles: true })); })()`, jsString(selector)))
		run(t, tab, chromedp.SendKeys(selector, text))
	}
	click := func(selector string) { t.Helper(); run(t, tab, chromedp.Click(selector)) }
	enter := func(selector string) { t.Helper(); run(t, tab, chromedp.Focus(selector), chromedp.KeyEvent("\r")) }

	// form-wizard: setup is two steps; Next is blocked while step one is invalid.
	run(t, tab, chromedp.Navigate(base+"/setup"))
	check(js(`document.querySelectorAll("fieldset[data-step]")[1].hidden`), "form-wizard: step two starts hidden")
	check(js(`document.querySelector('form[data-wizard] button[type="submit"]').hidden`), "form-wizard: submit hidden before the last step")
	click(".wizard-nav button:not(.btn-ghost)")
	check(js(`document.querySelectorAll("fieldset[data-step]")[1].hidden`), "form-wizard: Next is blocked while step one is invalid")
	check(js(`document.querySelector("#email").getAttribute("aria-invalid") === "true"`), "form-field-input: the invalid field is marked")
	typeInto("#email", "  "+email+"  ")
	typeInto("#password", "short")
	check(js(`document.querySelector(".strength meter").value === 0`), "form-field-input: a short password scores 0")
	typeInto("#password", password)
	check(js(`document.querySelector(".strength meter").value >= 1`), "form-field-input: a valid password scores above 0")
	click("#password + .reveal")
	check(js(`document.querySelector("#password").type === "text"`), "form-field-input: reveal shows the password")
	click("#password + .reveal")
	typeInto("#password-confirmation", "different-password-1")
	click(".wizard-nav button:not(.btn-ghost)")
	check(js(`document.querySelector("#password-confirmation").getAttribute("aria-invalid") === "true"`), "form-field-input: mismatched confirmation is invalid")
	typeInto("#password-confirmation", password)
	click(".wizard-nav button:not(.btn-ghost)")
	check(js(`!document.querySelectorAll("fieldset[data-step]")[1].hidden`), "form-wizard: Next moves to step two once step one is valid")
	check(js(`document.activeElement?.tagName === "LEGEND"`), "form-wizard: focus moves to the new step")
	check(js(`document.querySelector("#email").value === "ux@example.com"`), "form-field-input: the email was trimmed")
	typeInto("#workspace", "UX")
	click(`form[data-wizard] button[type="submit"]`)
	eventually(t, tab, fmt.Sprintf(`!location.pathname.endsWith("/setup")`), "setup did not finish")
	check(true, "form-wizard: the owner account is created from the last step")

	// form-field-input: a full-page error takes focus.
	call(t, tab, network.ClearBrowserCookies, cdp.Empty{})
	run(t, tab, chromedp.Navigate(base+"/login"))
	typeInto("#email", email)
	typeInto("#password", "wrong-password-1234")
	click(`form[action="/login"] button[type="submit"]`)
	eventually(t, tab, `document.activeElement?.matches('.error[role="alert"]')`, "login error did not take focus")
	typeInto("#password", password)
	click(`form[action="/login"] button[type="submit"]`)
	eventually(t, tab, `location.pathname === "/" || /^\/workspaces\/[^/]+$/.test(location.pathname)`, "login did not reach home")
	workspace := eval[string](t, tab, `location.pathname === "/" ? new URL(document.querySelector(".cards a").getAttribute("href"), location.href).pathname : location.pathname`)

	// form-drafts: an unsent invitation survives leaving the page.
	run(t, tab, chromedp.Navigate(base+workspace+"/settings/invitations"))
	typeInto("#invite-email", "draft@example.com")
	run(t, tab, chromedp.Navigate(base+workspace))
	run(t, tab, chromedp.Navigate(base+workspace+"/settings/invitations"))
	check(js(`document.querySelector("#invite-email").value === "draft@example.com"`), "form-drafts: the draft is restored")
	check(js(`Boolean(document.querySelector(".draft-notice"))`), "form-drafts: the restore is announced")
	click(".draft-notice button")
	check(js(`document.querySelector("#invite-email").value === ""`), "form-drafts: Discard clears the draft")

	// form-drafts: the unsaved-changes warning fires only when dirty.
	run(t, tab, chromedp.Navigate(base+"/account"))
	prompt := func() bool {
		return eval[bool](t, tab, `(() => { const event = new Event("beforeunload", { cancelable: true }); window.dispatchEvent(event); return event.defaultPrevented; })()`)
	}
	check(!prompt(), "form-drafts: no warning on a clean form")
	typeInto("#display-name", "Ux")
	check(prompt(), "form-drafts: a warning on unsaved changes")
	check(js(`document.querySelector("#account-timezone").value !== ""`), "form-field-input: timezone has a smart default")
	// The unsaved-changes prompt is a real beforeunload dialog; accept it so the journey can continue.
	acceptDialogs(tab)

	// form-wizard: progressive disclosure on email settings.
	run(t, tab, chromedp.Navigate(base+workspace+"/settings/email"))
	shown := func() string {
		return eval[string](t, tab, `JSON.stringify([...document.querySelectorAll("[data-show-when]")].filter((s) => !s.hidden).map((s) => s.dataset.showWhen))`)
	}
	check(shown() == `["email-provider=smtp"]`, "form-wizard: only the SMTP section shows for SMTP")
	eval[any](t, tab, `(() => { const select = document.querySelector("#email-provider"); select.value = "resend"; select.dispatchEvent(new Event("change", { bubbles: true })); })()`)
	check(shown() == `["email-provider=resend"]`, "form-wizard: switching provider swaps the section")

	// async-feedback: the audit table loads after the page.
	run(t, tab, chromedp.Navigate(base+workspace+"/settings/audit"))
	eventually(t, tab, `!document.querySelector("[data-lazy-src]") && document.querySelector("#audit-table") !== null`, "lazy audit table")
	check(true, "async-feedback: the audit table loads lazily")

	// form-field-input: smart default token expiry; keyboard: Escape closes a menu.
	run(t, tab, chromedp.Navigate(base+workspace+"/settings/tokens"))
	check(js(`/^\d{4}-\d{2}-\d{2}$/.test(document.querySelector("#token-expiry").value)`), "form-field-input: token expiry has a default")
	click("header .dropdown-end summary")
	run(t, tab, chromedp.KeyEvent("\x1b"))
	check(js(`!document.querySelector("header .dropdown-end").open`), "async-feedback: Escape closes the account menu")

	// clients: a blank name is stopped with a summary; a client is created; a choice field gets an option.
	run(t, tab, chromedp.Navigate(base+workspace+"/clients"))
	waitVisible(t, tab, "#client-list")
	click(`a[href$="/clients/new"]`)
	typeInto("#client-display-name", "   ")
	enter("#client-display-name")
	check(js(`document.activeElement?.classList.contains("error-summary")`), "clients: a blank name takes the error summary focus")
	typeInto("#client-display-name", "Ada Lovelace")
	typeInto("#client-email", "ada@example.com")
	click("form.create button[type=submit]")
	waitVisible(t, tab, "#client-detail")
	check(js(`document.querySelector("#client-detail #client-display-name").value === "Ada Lovelace"`), "clients: creating a client opens its detail page")
	clientPath := eval[string](t, tab, `location.pathname`)

	run(t, tab, chromedp.Navigate(base+workspace+"/client-fields"))
	click(".client-create > summary")
	typeInto("#client-field-label", "Tier")
	eval[any](t, tab, `document.querySelector("#client-field-type").value = "single_choice"`)
	click(".client-create form button[type=submit]")
	waitVisible(t, tab, ".client-fields")
	typeInto(`input[id^="option-label-"]`, "Gold")
	click("form.option-create button[type=submit]")
	waitVisible(t, tab, ".client-field-option-list")

	run(t, tab, chromedp.Navigate(base+clientPath))
	waitVisible(t, tab, "#client-values")
	eval[any](t, tab, `(() => { const select = document.querySelector('select[id^="value-"]'); select.value = [...select.options].find((o) => o.textContent === "Gold").value; })()`)
	click("form.client-value button[type=submit]")
	eventually(t, tab, `document.querySelector("form.client-value-clear") !== null`, "saved value did not reload")
	check(js(`document.querySelector('select[id^="value-"]').selectedOptions[0].textContent === "Gold"`), "clients: a choice value is entered and saved")
	click("form.client-value-clear button")
	eventually(t, tab, `!document.querySelector("form.client-value-clear")`, "cleared value did not reload")
	check(js(`document.querySelector('select[id^="value-"]').selectedOptions[0].disabled`), "clients: Clear removes the saved value")
}
