//go:build browser

package browsertest

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// TestEmailVerificationFailure ports scripts/playwright/email-verification.spec.mjs.
func TestEmailVerificationFailure(t *testing.T) {
	copies := map[string]struct{ heading, message string }{
		"en": {"Email verification", "The email verification link is invalid or expired."},
		"es": {"Verificación de correo", "El enlace de verificación de correo no es válido o venció."},
	}
	for _, viewport := range []string{"desktop", "mobile"} {
		for _, theme := range []string{"light", "dark", "system"} {
			for _, locale := range []string{"en", "es"} {
				width, height := int64(1280), int64(900)
				if viewport == "mobile" {
					width, height = 390, 844
				}
				colorScheme := theme
				if theme == "system" {
					colorScheme = "dark"
				}
				name := fmt.Sprintf("%s-%s-%s", viewport, theme, locale)
				t.Run(name, func(t *testing.T) {
					tab := newTab(t, width, height, colorScheme)
					setTheme(t, tab, theme)
					navigate(t, tab, fmt.Sprintf("/language?locale=%s&return_to=%%2Flogin", locale))

					response, err := chromedp.Run[*network.Response](tab, chromedp.NavigateResponse(baseURL+"/account/verify-email/unavailable"))
					if err != nil {
						t.Fatalf("navigate: %v", err)
					}
					if response == nil || response.Status != 422 {
						t.Fatalf("status = %v, want 422", response)
					}
					expectAttribute(t, tab, "html", "lang", locale)
					expectAttribute(t, tab, "html", "data-theme", theme)
					if got := eval[string](t, tab, `document.querySelector("h1")?.textContent.trim() ?? ""`); got != copies[locale].heading {
						t.Fatalf("heading = %q, want %q", got, copies[locale].heading)
					}
					expectText(t, tab, `[role="alert"]`, regexp.MustCompile(regexp.QuoteMeta(copies[locale].message)))
					waitVisible(t, tab, `a[href="/login"]`)

					eventually(t, tab, `document.activeElement?.getAttribute("role") === "alert"`, "error alert did not receive focus")
					press(t, tab, "\t")
					expectFocused(t, tab, `a[href="/login"]`)
					expectNoOverflow(t, tab, width)
					expectAxeClean(t, tab)
				})
			}
		}
	}
}
