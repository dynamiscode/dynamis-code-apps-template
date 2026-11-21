//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// fillByExactLabel sets the control whose label text is exactly name.
func fillByExactLabel(t *testing.T, tab context.Context, name, value string) {
	t.Helper()
	eventually(t, tab, fmt.Sprintf(`(() => { const label = [...document.querySelectorAll("label")].find((l) => l.textContent.trim() === %s); return Boolean(label && (label.htmlFor ? document.getElementById(label.htmlFor) : label.querySelector("input, select, textarea"))); })()`, jsString(name)), "no control labelled exactly "+name)
	eval[any](t, tab, fmt.Sprintf(`(() => { const label = [...document.querySelectorAll("label")].find((l) => l.textContent.trim() === %s); const control = label.htmlFor ? document.getElementById(label.htmlFor) : label.querySelector("input, select, textarea"); control.value = %s; control.dispatchEvent(new Event("input", { bubbles: true })); })()`, jsString(name), jsString(value)))
}

// boxSize returns the rendered width and height of the first element matching selector.
func boxSize(t *testing.T, tab context.Context, selector string) (float64, float64) {
	t.Helper()
	box := eval[struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}](t, tab, fmt.Sprintf(`(() => { const r = document.querySelector(%s)?.getBoundingClientRect(); return { width: r ? r.width : 0, height: r ? r.height : 0 }; })()`, jsString(selector)))
	return box.Width, box.Height
}

// expectMinimumTarget checks that a control is at least 24 CSS pixels in both directions.
func expectMinimumTarget(t *testing.T, tab context.Context, selector string) {
	t.Helper()
	width, height := boxSize(t, tab, selector)
	if width < 24 || height < 24 {
		t.Fatalf("%s is %vx%v, want at least 24x24", selector, width, height)
	}
}

// expectNoCriticalOrSeriousAxe runs axe with the WCAG 2.2 A/AA tags and fails on critical or serious results.
func expectNoCriticalOrSeriousAxe(t *testing.T, tab context.Context) {
	t.Helper()
	eval[any](t, tab, axeSource)
	type violation struct {
		ID     string `json:"id"`
		Impact string `json:"impact"`
	}
	res := eval[struct {
		Error      string      `json:"error"`
		Violations []violation `json:"violations"`
	}](t, tab, `new Promise((done) => {
		axe.run(document, { runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] } })
			.then(done).catch((error) => done({ error: error.message }));
	})`, awaitPromise)
	if res.Error != "" {
		t.Fatalf("axe: %s", res.Error)
	}
	var ids []string
	for _, item := range res.Violations {
		if item.Impact == "critical" || item.Impact == "serious" {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) != 0 {
		t.Fatalf("critical or serious axe violations: %v", ids)
	}
}

// responseStatus loads a full URL and returns the document's HTTP status.
func responseStatus(t *testing.T, tab context.Context, url string) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(tab, navigationWait)
	defer cancel()
	response, err := chromedp.Run(ctx, chromedp.NavigateResponse(url))
	if err != nil || response == nil {
		t.Fatalf("navigate %s: %v", url, err)
	}
	return response.Status
}

// itemRowExpr finds the Items row whose title input carries the title attribute. Playwright matched
// rows by the attribute, so an unsaved edit keeps the row's identity.
func itemRowExpr(title string) string {
	return fmt.Sprintf(`[...document.querySelectorAll('#item-list tr[data-item-row]')].find((r) => r.querySelector('input[name="title"]')?.getAttribute("value") === %s)`, jsString(title))
}

// openItems signs the owner in and opens the Items page of the first workspace.
func openItems(t *testing.T, tab context.Context) string {
	t.Helper()
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	click(t, tab, ".cards a")
	waitVisible(t, tab, "[data-workspace-home]")
	click(t, tab, ".workspace-cards a[href$='/items']")
	waitVisible(t, tab, "#item-list")
	return eval[string](t, tab, "location.pathname")
}

// createItem adds an item through the create form and waits for its row.
func createItem(t *testing.T, tab context.Context, title string) {
	t.Helper()
	fillByLabel(t, tab, "New item title", title)
	clickByText(t, tab, "Add item")
	eventually(t, tab, fmt.Sprintf("Boolean(%s)", itemRowExpr(title)), "item "+title+" not listed")
}

// TestOIDCLoginAndCallbackFailure ports the configured OIDC login and callback failure checks.
func TestOIDCLoginAndCallbackFailure(t *testing.T) {
	for _, viewport := range []string{"desktop", "mobile"} {
		for _, theme := range []string{"light", "dark"} {
			for _, locale := range []string{"en", "es"} {
				width, height := int64(1280), int64(900)
				if viewport == "mobile" {
					width, height = 390, 844
				}
				t.Run(fmt.Sprintf("%s-%s-%s", viewport, theme, locale), func(t *testing.T) {
					tab := newTab(t, width, height, theme)
					setTheme(t, tab, theme)
					navigate(t, tab, "/language?locale="+locale+"&return_to=%2Flogin")
					oidcNavigate(t, tab, "/language?locale="+locale+"&return_to=%2Flogin")
					oidcNavigate(t, tab, "/login")
					expectAttribute(t, tab, "html", "lang", locale)
					expectTheme(t, tab, theme)
					eval[any](t, tab, `(() => { const style = document.createElement("style"); style.textContent = "html { font-size: 12px; }"; document.head.append(style); })()`)
					expectMinimumTarget(t, tab, `a[href$="password-reset"]`)
					providerName := "Sign in with Company SSO"
					if locale == "es" {
						providerName = "Iniciar sesión con Company SSO"
					}
					provider := fmt.Sprintf(`[...document.querySelectorAll("a")].find((a) => a.textContent.trim() === %s)`, jsString(providerName))
					if got := eval[string](t, tab, provider+`?.getAttribute("href") ?? ""`); got != "/auth/oidc/company" {
						t.Fatalf("provider href %q", got)
					}
					press(t, tab, "\t")
					expectFocused(t, tab, ".skip-link")
					for i := 0; i < 6; i++ {
						press(t, tab, "\t")
					}
					eventually(t, tab, fmt.Sprintf("document.activeElement === %s", provider), "provider link did not receive keyboard focus")
					expectNoCriticalOrSeriousAxe(t, tab)
					expectNoOverflow(t, tab, width)

					oidcNavigate(t, tab, "/auth/oidc/company/callback?error=access_denied")
					expectAttribute(t, tab, "html", "lang", locale)
					expectTheme(t, tab, theme)
					pattern := regexp.MustCompile(`(?i)could not be completed`)
					if locale == "es" {
						pattern = regexp.MustCompile(`(?i)no se pudo completar`)
					}
					expectText(t, tab, `[role="alert"]`, pattern)
					eventually(t, tab, `document.activeElement?.getAttribute("role") === "alert"`, "callback alert did not receive focus")
					expectMinimumTarget(t, tab, `a[href$="password-reset"]`)
					expectNoCriticalOrSeriousAxe(t, tab)
					expectNoOverflow(t, tab, width)
				})
			}
		}
	}
}

// oidcNavigate loads a path on the OIDC fixture origin in the same tab.
func oidcNavigate(t *testing.T, tab context.Context, path string) {
	t.Helper()
	run(t, tab, chromedp.Navigate(oidcURL+path))
}

// TestOIDCCompletesVerifiedCallback ports the verified OIDC callback, session and denial test.
func TestOIDCCompletesVerifiedCallback(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	oidcNavigate(t, tab, "/login")
	fillByLabel(t, tab, "Email", "oidc-fixture@example.com")
	fillByLabel(t, tab, "Password", "oidc-fixture-password-123")
	clickByText(t, tab, "Sign in")
	eventually(t, tab, fmt.Sprintf("location.href === %s", jsString(oidcURL+"/")), "password sign-in did not reach the home page")
	workspacePath := eval[string](t, tab, `document.querySelector(".cards a")?.getAttribute("href") ?? ""`)
	if !regexp.MustCompile("^/workspaces/").MatchString(workspacePath) {
		t.Fatalf("first workspace link %q", workspacePath)
	}
	call(t, tab, network.ClearBrowserCookies, cdp.Empty{})
	oidcNavigate(t, tab, "/login")
	clickByText(t, tab, "Sign in with Company SSO")
	eventually(t, tab, fmt.Sprintf("location.href === %s", jsString(oidcURL+"/")), "SSO callback did not reach the home page")
	if !hasCookie(t, tab, "session") {
		t.Fatal("no session cookie after SSO")
	}
	waitVisible(t, tab, "#main")
	expectNoCriticalOrSeriousAxe(t, tab)
	if status := responseStatus(t, tab, oidcURL+workspacePath); status != 404 {
		t.Fatalf("unrelated workspace status %d, want 404", status)
	}
}

// hasCookie reports whether the tab holds a cookie with the given name for the OIDC origin.
func hasCookie(t *testing.T, tab context.Context, name string) bool {
	t.Helper()
	cookies := call(t, tab, network.GetCookies, network.GetCookiesParams{URLs: []string{oidcURL}})
	for _, cookie := range cookies.Cookies {
		if cookie.Name == name {
			return true
		}
	}
	return false
}

// TestOIDCUnconfiguredProviderIsUnavailable ports the unconfigured provider check.
func TestOIDCUnconfiguredProviderIsUnavailable(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	if status := responseStatus(t, tab, oidcURL+"/auth/oidc/missing"); status != 404 {
		t.Fatalf("unconfigured provider status %d, want 404", status)
	}
}

// TestSetupAccessibleAcrossLayouts ports the initial setup accessibility checks.
func TestSetupAccessibleAcrossLayouts(t *testing.T) {
	for _, viewport := range []string{"desktop", "mobile"} {
		for _, theme := range []string{"light", "dark"} {
			for _, locale := range []string{"en", "es"} {
				width, height := int64(1280), int64(900)
				if viewport == "mobile" {
					width, height = 390, 844
				}
				t.Run(fmt.Sprintf("%s-%s-%s", viewport, theme, locale), func(t *testing.T) {
					tab := newTab(t, width, height, theme)
					setTheme(t, tab, theme)
					run(t, tab, chromedp.Navigate(setupURL+"/language?locale="+locale+"&return_to=%2Fsetup"))
					run(t, tab, chromedp.Navigate(setupURL+"/setup"))
					expectAttribute(t, tab, "html", "lang", locale)
					expectTheme(t, tab, theme)
					waitVisible(t, tab, `form[action="/setup"]`)
					expectNoCriticalOrSeriousAxe(t, tab)
					expectNoOverflow(t, tab, width)
				})
			}
		}
	}
}

// TestSetupWizardCreatesOwnerThenDisables ports the setup validation and first-owner test.
func TestSetupWizardCreatesOwnerThenDisables(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	run(t, tab, chromedp.Navigate(setupURL+"/setup"))
	clickByText(t, tab, "Next")
	expectFocused(t, tab, "#email")
	fillByLabel(t, tab, "Email", "setup-owner@example.com")
	fillByExactLabel(t, tab, "Password", "setup-owner-password-123")
	fillByLabel(t, tab, "Confirm password", "setup-owner-password-123")
	clickByText(t, tab, "Next")
	waitVisible(t, tab, "#workspace")
	fill(t, tab, "#workspace", "Setup smoke workspace")
	click(t, tab, `form[action="/setup"] button[type="submit"]`)
	eventually(t, tab, fmt.Sprintf("location.href === %s", jsString(setupURL+"/login")), "setup did not reach login")
	waitVisible(t, tab, `form[action="/login"]`)
	if status := responseStatus(t, tab, setupURL+"/setup"); status != 404 {
		t.Fatalf("setup after completion status %d, want 404", status)
	}
}

// TestLoginSystemThemeFollowsPreference ports the login system-theme test.
func TestLoginSystemThemeFollowsPreference(t *testing.T) {
	for _, scheme := range []string{"light", "dark"} {
		t.Run(scheme, func(t *testing.T) {
			tab := newTab(t, 1280, 900, scheme)
			navigate(t, tab, "/login")
			expectTheme(t, tab, "system")
			want := "#f3f6fa"
			if scheme == "dark" {
				want = "#111826"
			}
			if canvas := eval[string](t, tab, `getComputedStyle(document.documentElement).getPropertyValue("--canvas").trim()`); canvas != want {
				t.Fatalf("canvas %q, want %q", canvas, want)
			}
			expectNoCriticalOrSeriousAxe(t, tab)
		})
	}
}

// TestInvalidLoginPreservesFormAndAlerts ports the invalid login test.
func TestInvalidLoginPreservesFormAndAlerts(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	navigate(t, tab, "/login")
	fillByLabel(t, tab, "Email", "missing-user@example.com")
	fillByLabel(t, tab, "Password", "invalid-login-password-123")
	clickByText(t, tab, "Sign in")
	expectText(t, tab, `[role="alert"]`, regexp.MustCompile(regexp.QuoteMeta("The email or password is invalid.")))
	waitVisible(t, tab, `form[action="/login"]`)
	if eval[string](t, tab, `document.querySelector('input[name="csrf"]')?.value ?? ""`) == "" {
		t.Fatal("csrf is empty after failed login")
	}
	expectNoCriticalOrSeriousAxe(t, tab)
}

// TestLoginPageAccessibleAcrossLayouts ports the login page accessibility matrix.
func TestLoginPageAccessibleAcrossLayouts(t *testing.T) {
	for _, viewport := range []string{"desktop", "mobile"} {
		for _, theme := range []string{"light", "dark"} {
			for _, locale := range []string{"en", "es"} {
				width, height := int64(1280), int64(900)
				if viewport == "mobile" {
					width, height = 390, 844
				}
				t.Run(fmt.Sprintf("%s-%s-%s", viewport, theme, locale), func(t *testing.T) {
					tab := newTab(t, width, height, theme)
					setTheme(t, tab, theme)
					navigate(t, tab, "/language?locale="+locale+"&return_to=%2Flogin")
					navigate(t, tab, "/login")
					expectAttribute(t, tab, "html", "lang", locale)
					expectTheme(t, tab, theme)
					waitVisible(t, tab, `form[action="/login"]`)
					press(t, tab, "\t")
					expectFocused(t, tab, ".skip-link")
					expectNoOverflow(t, tab, width)
				})
			}
		}
	}
}

// TestItemsCreateEditAndDeleteFirstTask ports the Items create, edit preservation, validation and delete test.
func TestItemsCreateEditAndDeleteFirstTask(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	openItems(t, tab)
	createItem(t, tab, "Playwright first item")
	createItem(t, tab, "Playwright second item")

	fillItemTitle(t, tab, "Playwright second item", "Playwright updated item")
	fillItemTitle(t, tab, "Playwright first item", "Unsaved Playwright item")
	clickInItemRow(t, tab, "Playwright second item", "Save Playwright second item")
	eventually(t, tab, fmt.Sprintf("Boolean(%s)", itemRowExpr("Playwright updated item")), "saved item not listed as updated")
	// The preserved row is still identified by its saved title attribute; its live draft is the unsaved value.
	eventually(t, tab, fmt.Sprintf(`(() => { const row = %s; return Boolean(row) && row.querySelector('input[name="title"]').value === "Unsaved Playwright item" && row.querySelector('form[data-item-edit]')?.dataset.dirty === "true"; })()`, itemRowExpr("Playwright first item")), "unsaved item lost its dirty draft")

	invalid := ""
	for i := 0; i < 201; i++ {
		invalid += "x"
	}
	eval[any](t, tab, fmt.Sprintf(`(() => { const row = %s; const input = row.querySelector('input[name="title"]'); input.removeAttribute("maxlength"); })()`, itemRowExpr("Playwright updated item")))
	// A failed save keeps the row's saved title attribute, so the row is still found by its last saved title.
	fillItemTitle(t, tab, "Playwright updated item", invalid)
	clickInItemRow(t, tab, "Playwright updated item", "Save Playwright updated item")
	expectText(t, tab, `[role="alert"]`, regexp.MustCompile("item values are invalid"))
	eventually(t, tab, fmt.Sprintf(`(() => { const row = %s; return Boolean(row) && row.querySelector('input[name="title"]').value === %s; })()`, itemRowExpr("Playwright updated item"), jsString(invalid)), "invalid title was not preserved")

	eval[any](t, tab, fmt.Sprintf(`(() => { const row = %s; row.querySelector('input[name="version"]').value = "0"; })()`, itemRowExpr("Playwright updated item")))
	acceptDialogs(tab)
	clickInItemRow(t, tab, "Playwright updated item", "Delete permanently Playwright updated item")
	expectText(t, tab, `[role="alert"]`, regexp.MustCompile("item version is invalid"))
	eventually(t, tab, fmt.Sprintf("Boolean(%s)", itemRowExpr("Playwright updated item")), "item disappeared after an invalid delete")
}

// itemRowWithInputExpr finds the Items row whose title input holds the value.
func itemRowWithInputExpr(title string) string {
	return itemRowExpr(title)
}

// fillItemTitle sets the title input of the row currently holding oldTitle.
func fillItemTitle(t *testing.T, tab context.Context, oldTitle, newTitle string) {
	t.Helper()
	eval[any](t, tab, fmt.Sprintf(`(() => { const row = %s; const input = row.querySelector('input[name="title"]'); input.value = %s; input.dispatchEvent(new Event("input", { bubbles: true })); })()`, itemRowExpr(oldTitle), jsString(newTitle)))
}

// clickInItemRow clicks the control named name inside the row holding title.
func clickInItemRow(t *testing.T, tab context.Context, title, name string) {
	t.Helper()
	// Row actions sit in row menus. Open the menus around the control, as a user would, then click it.
	eval[any](t, tab, fmt.Sprintf(`(() => { const row = %s; const control = [...row.querySelectorAll("button, a")].find((b) => (b.getAttribute("aria-label") || b.textContent).trim() === %s || (b.getAttribute("aria-label") || b.textContent).includes(%s)); for (let menu = control.closest("details"); menu; menu = menu.parentElement?.closest("details")) menu.open = true; window.__bt = 1; control.click(); })()`, itemRowExpr(title), jsString(name), jsString(name)))
	settle(t, tab)
}

// TestWorkspaceHomePassesAxe ports the workspace home axe check.
func TestWorkspaceHomePassesAxe(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	click(t, tab, ".cards a")
	waitVisible(t, tab, "[data-workspace-home]")
	expectNoCriticalOrSeriousAxe(t, tab)
}

// TestItemsSharingCreatesAndRevokesPublicLink ports the Items sharing test.
func TestItemsSharingCreatesAndRevokesPublicLink(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	itemsPath := openItems(t, tab)
	title := "Playwright shared item"
	createItem(t, tab, title)
	// One click, as Playwright made: a second create would add a second link.
	// Record the link as it appears: a realtime refresh can replace the list right after the create response.
	eval[any](t, tab, `(() => { window.__shareHref = ""; new MutationObserver(() => { const link = document.querySelector("#item-list a[href*='/share/']"); if (link && !window.__shareHref) window.__shareHref = link.href; }).observe(document.body, { subtree: true, childList: true }); })()`)
	eval[any](t, tab, fmt.Sprintf(`(() => { const row = %s; const create = row.querySelector("form.share button[value=create]"); for (let menu = create.closest("details"); menu; menu = menu.parentElement?.closest("details")) menu.open = true; window.__bt = 1; create.click(); })()`, itemRowExpr(title)))
	settle(t, tab)
	eventually(t, tab, `window.__shareHref !== ""`, "share link not created")
	shareURL := eval[string](t, tab, `window.__shareHref`)
	navigate(t, tab, shareURL[len(baseURL):])
	expectText(t, tab, "h1", regexp.MustCompile("Shared item"))
	waitVisible(t, tab, "main.public-share-card")
	expectText(t, tab, "main.public-share-card", regexp.MustCompile(regexp.QuoteMeta(title)))
	if eval[bool](t, tab, `document.querySelector("main.public-share-card").textContent.includes("UI Baselines")`) {
		t.Fatal("public share exposes the workspace name")
	}

	navigate(t, tab, itemsPath)
	waitVisible(t, tab, "#item-list")
	acceptDialogs(tab)
	eval[any](t, tab, fmt.Sprintf(`(() => { const row = %s; const revoke = row.querySelector('form button[value="revoke"]'); for (let menu = revoke.closest("details"); menu; menu = menu.parentElement?.closest("details")) menu.open = true; window.__bt = 1; revoke.click(); })()`, itemRowExpr(title)))
	settle(t, tab)
	navigate(t, tab, shareURL[len(baseURL):])
	if status := responseStatus(t, tab, shareURL); status != 404 {
		t.Fatalf("revoked share status %d, want 404", status)
	}
	eventually(t, tab, `[...document.querySelectorAll("h1, h2, h3")].some((h) => /unavailable|no disponible/i.test(h.textContent))`, "revoked share has no unavailable heading")
}

// TestItemsSearchHistoryNavigation ports the Items query history test. The search field
// replaces the removed sort and filter disclosure; history behaves the same.
func TestItemsSearchHistoryNavigation(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	basePath := openItems(t, tab)
	eval[any](t, tab, `(() => { const input = document.querySelector('input[placeholder="Search titles"]'); input.value = "history marker"; input.dispatchEvent(new Event("input", { bubbles: true })); input.dispatchEvent(new Event("change", { bubbles: true })); })()`)
	eventually(t, tab, `location.search.includes("q=history+marker") || location.search.includes("q=history%20marker")`, "search did not put the query in the URL")
	eventually(t, tab, `document.querySelector('input[placeholder="Search titles"]').value === "history marker"`, "search field did not keep the query")

	run(t, tab, chromedp.NavigateBack())
	eventually(t, tab, fmt.Sprintf(`location.pathname === %s && location.search === ""`, jsString(basePath)), "back did not return to the Items page")
	eventually(t, tab, `document.querySelector('input[placeholder="Search titles"]').value === ""`, "back kept the query in the search field")
	run(t, tab, chromedp.NavigateForward())
	eventually(t, tab, `location.search.includes("q=history+marker") || location.search.includes("q=history%20marker")`, "forward did not restore the query")
	eventually(t, tab, `document.querySelector('input[placeholder="Search titles"]').value === "history marker"`, "forward did not restore the search field")
}
