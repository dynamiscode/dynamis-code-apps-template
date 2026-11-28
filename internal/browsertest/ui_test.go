//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"regexp"
	"strings"
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
func expectMinimumTarget(t *testing.T, tab context.Context, selector string) {
	t.Helper()
	visible := eval[bool](t, tab, fmt.Sprintf(`(() => {
		const element = document.querySelector(%s);
		if (!element) return false;
		const rect = element.getBoundingClientRect();
		return rect.width >= 24 && rect.height >= 24;
	})()`, jsString(selector)))
	if !visible {
		t.Fatalf("%s is not a usable 24-by-24 CSS-pixel target", selector)
	}
}

// expectNoCriticalOrSeriousAxe runs axe with the WCAG 2.2 A/AA tags and fails on critical or serious results.
func expectNoCriticalOrSeriousAxe(t *testing.T, tab context.Context) {
	t.Helper()
	eval[any](t, tab, axeSource)
	type node struct {
		Target []string `json:"target"`
		HTML   string   `json:"html"`
	}
	type violation struct {
		ID     string `json:"id"`
		Impact string `json:"impact"`
		Help   string `json:"help"`
		Nodes  []node `json:"nodes"`
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
	var details strings.Builder
	for _, item := range res.Violations {
		if item.Impact == "critical" || item.Impact == "serious" {
			ids = append(ids, item.ID)
			fmt.Fprintf(&details, "\n%s (%s): %s", item.ID, item.Impact, item.Help)
			for _, n := range item.Nodes {
				fmt.Fprintf(&details, "\n  %s %s", strings.Join(n.Target, ", "), n.HTML)
			}
		}
	}
	if len(ids) != 0 {
		t.Fatalf("critical or serious axe violations:%s", details.String())
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
	eventually(t, tab, `location.pathname === "/" || /^\/workspaces\/[^/]+$/.test(location.pathname)`, "password sign-in did not reach a workspace")
	workspacePath := eval[string](t, tab, `location.pathname === "/" ? (document.querySelector(".cards a")?.getAttribute("href") ?? "") : location.pathname`)
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

func TestWorkspaceHomePassesAxe(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	if eval[bool](t, tab, `location.pathname === "/"`) {
		click(t, tab, ".cards a")
	}
	waitVisible(t, tab, "[data-workspace-home]")
	expectNoCriticalOrSeriousAxe(t, tab)
}
