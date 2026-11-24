//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestPublicIntakeLongConsentReflow(t *testing.T) {
	owner := newTab(t, 1280, 900, "light")
	signIn(t, owner)
	if eval[bool](t, owner, `location.pathname === "/"`) {
		click(t, owner, ".cards a")
	}
	workspace := eval[string](t, owner, `location.pathname`)
	navigate(t, owner, workspace+"/engagement-stages")
	fill(t, owner, "#stage-name", "Discovery")
	click(t, owner, `form:has(#stage-name) button[type=submit]`)
	navigate(t, owner, workspace+"/intake")
	consent := strings.Repeat("LongConsent", 130)
	fill(t, owner, "#intake-consent", consent)
	click(t, owner, `button[name=action][value=create]`)
	waitVisible(t, owner, "#intake-link")
	link := eval[string](t, owner, `new URL(document.querySelector('#intake-link').value, location.href).href`)

	for _, tc := range []struct {
		name, theme   string
		width, height int64
	}{{"mobile-dark", "dark", 390, 844}, {"desktop-light", "light", 1440, 900}} {
		t.Run(tc.name, func(t *testing.T) {
			tab := newTab(t, tc.width, tc.height, tc.theme)
			setTheme(t, tab, tc.theme)
			run(t, tab, chromedp.Navigate(link))
			waitVisible(t, tab, `input[name=consent]`)
			width := eval[int](t, tab, `document.documentElement.scrollWidth`)
			t.Logf("document width %d; viewport %d", width, tc.width)
			if width > int(tc.width) {
				t.Fatalf("document scroll width = %d, viewport = %d", width, tc.width)
			}
			if got := eval[int](t, tab, `document.querySelector('input[name=consent] + span').textContent.length`); got != len(consent) {
				t.Fatalf("consent length = %d, want %d", got, len(consent))
			}
			expectAxeClean(t, tab)
			run(t, tab, chromedp.Focus(`input[name=consent]`))
			if !eval[bool](t, tab, `document.activeElement === document.querySelector('input[name=consent]')`) {
				t.Fatal("consent checkbox is not keyboard focusable")
			}
			capturePublicIntake(t, tab, tc.name+"-form.png")
			fill(t, tab, "#name", "Synthetic Visitor")
			click(t, tab, `input[name=consent]`)
			click(t, tab, `button[type=submit]`)
			waitVisible(t, tab, `[role=alert]`)
			if !eval[bool](t, tab, `document.querySelector('[role=alert]').textContent.includes('phone number or email address')`) {
				t.Fatal("missing contact validation error")
			}
			expectAxeClean(t, tab)
			if got := eval[int](t, tab, `document.documentElement.scrollWidth`); got > int(tc.width) {
				t.Fatalf("error document scroll width = %d, viewport = %d", got, tc.width)
			}
			capturePublicIntake(t, tab, tc.name+"-error.png")
		})
	}
}

func capturePublicIntake(t *testing.T, tab context.Context, name string) {
	t.Helper()
	dir := os.Getenv("PUBLIC_INTAKE_EVIDENCE_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	png, err := chromedp.Run(tab, chromedp.CaptureScreenshot())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), png, 0644); err != nil {
		t.Fatal(fmt.Errorf("write screenshot: %w", err))
	}
}
