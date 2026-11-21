//go:build browser

package browsertest

import (
	"fmt"
	"regexp"
	"testing"
)

// TestPasswordRecoveryStates ports scripts/playwright/password-reset.spec.mjs.
func TestPasswordRecoveryStates(t *testing.T) {
	layouts := []struct {
		name          string
		width, height int64
		theme, locale string
	}{}
	for _, viewport := range []string{"desktop", "mobile"} {
		for _, theme := range []string{"light", "dark"} {
			for _, locale := range []string{"en", "es"} {
				width, height := int64(1280), int64(900)
				if viewport == "mobile" {
					width, height = 390, 844
				}
				layouts = append(layouts, struct {
					name          string
					width, height int64
					theme, locale string
				}{fmt.Sprintf("%s-%s-%s", viewport, theme, locale), width, height, theme, locale})
			}
		}
	}
	for _, layout := range layouts {
		t.Run(layout.name, func(t *testing.T) {
			tab := newTab(t, layout.width, layout.height, layout.theme)
			setTheme(t, tab, layout.theme)
			navigate(t, tab, fmt.Sprintf("/language?locale=%s&return_to=%%2Fpassword-reset", layout.locale))
			navigate(t, tab, "/password-reset")
			expectTheme(t, tab, layout.theme)
			expectAttribute(t, tab, "html", "lang", layout.locale)
			waitVisible(t, tab, `form[action="/password-reset"]`)

			press(t, tab, "\t")
			expectFocused(t, tab, ".skip-link")
			press(t, tab, "\t")
			expectFocused(t, tab, "#reset-email")
			press(t, tab, "\t")
			expectFocused(t, tab, `form[action="/password-reset"] button[type="submit"]`)

			if layout.width < 608 {
				expectNoOverflow(t, tab, layout.width)
			}

			navigate(t, tab, "/password-reset/unavailable")
			fill(t, tab, "#reset-password", "first-valid-password-123")
			fill(t, tab, "#reset-password-confirmation", "different-valid-password-456")
			click(t, tab, `form[action="/password-reset/unavailable"] button[type="submit"]`)
			eventually(t, tab, `document.activeElement?.getAttribute("role") === "alert"`, "mismatch alert did not receive focus")
			expectText(t, tab, `[role="alert"]`, regexp.MustCompile(`(?i)passwords do not match|no coinciden`))
			waitVisible(t, tab, `form[action="/password-reset/unavailable"]`)
			if csrf := eval[string](t, tab, `document.querySelector('input[name="csrf"]')?.value ?? ""`); csrf == "" {
				t.Fatal("csrf input is empty")
			}

			fill(t, tab, "#reset-password-confirmation", "first-valid-password-123")
			click(t, tab, `form[action="/password-reset/unavailable"] button[type="submit"]`)
			eventually(t, tab, `document.activeElement?.getAttribute("role") === "alert"`, "unavailable alert did not receive focus")
			waitVisible(t, tab, `form[action="/password-reset"]`)
			expectText(t, tab, `[role="alert"]`, regexp.MustCompile(`(?i)invalid|válido`))
		})
	}
}
