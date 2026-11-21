//go:build browser

package browsertest

import (
	"testing"
)

// TestFixedContrastSurfacesAxeCleanInLight checks the surfaces that failed
// WCAG AA color-contrast under the light theme: header crumbs, token scope
// labels, and webhook event labels. Local headless Chrome defaults to the OS
// dark preference, so the light scheme is emulated explicitly.
func TestFixedContrastSurfacesAxeCleanInLight(t *testing.T) {
	tab := newTab(t, 1280, 900, "light")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	workspacePath := openWorkspaceSettings(t, tab, "/settings/general")

	navigate(t, tab, workspacePath+"/settings/general")
	waitVisible(t, tab, "main h1")
	expectAxeClean(t, tab)

	navigate(t, tab, workspacePath+"/settings/tokens")
	waitVisible(t, tab, "#token-name")
	expectAxeClean(t, tab)

	navigate(t, tab, workspacePath+"/settings/webhooks")
	waitVisible(t, tab, "main h1")
	expectAxeClean(t, tab)
}
