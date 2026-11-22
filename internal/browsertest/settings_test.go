//go:build browser

package browsertest

import (
	"fmt"
	"testing"

	"github.com/chromedp/chromedp"
)

// TestSettingsKeyboardReachableAndAxeClean ports the settings keyboard order and axe checks.
func TestSettingsKeyboardReachableAndAxeClean(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	workspacePath := openWorkspaceSettings(t, tab, "/settings/general")
	waitVisible(t, tab, "#workspace-locale")
	press(t, tab, "\t")
	expectFocused(t, tab, ".skip-link")
	eval[any](t, tab, `document.getElementById("workspace-locale").focus()`)
	expectFocused(t, tab, "#workspace-locale")

	navigate(t, tab, workspacePath+"/settings/email")
	waitVisible(t, tab, "#email-provider")
	press(t, tab, "\t")
	expectFocused(t, tab, ".skip-link")
	eval[any](t, tab, `document.getElementById("email-provider").focus()`)
	press(t, tab, "\t")
	expectFocused(t, tab, "#email-sender-name")

	navigate(t, tab, workspacePath+"/settings/tokens")
	waitVisible(t, tab, "#token-name")
	press(t, tab, "\t")
	expectFocused(t, tab, ".skip-link")
	eval[any](t, tab, `document.getElementById("token-name").focus()`)
	expectFocused(t, tab, "#token-name")

	navigate(t, tab, workspacePath+"/settings/provisioning")
	waitVisible(t, tab, "h1")
	press(t, tab, "\t")
	expectFocused(t, tab, ".skip-link")
	for _, section := range []string{"general", "email", "tokens", "provisioning"} {
		navigate(t, tab, workspacePath+"/settings/"+section)
		waitVisible(t, tab, "h1")
		expectAxeClean(t, tab)
	}
}

// TestSettingsRenderInSpanish ports the Spanish settings test.
func TestSettingsRenderInSpanish(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	workspacePath := openWorkspaceSettings(t, tab, "/settings/general")
	navigate(t, tab, "/language?locale=es&return_to="+urlQuery(workspacePath))
	cases := []struct {
		section, heading string
		csrfForms        int
	}{
		{"general", "Configuración general del espacio de trabajo", 1},
		{"email", "Entrega de correo", 1},
		{"tokens", "Tokens de API", 1},
		{"provisioning", "Aprovisionamiento SCIM", 2},
	}
	for _, c := range cases {
		navigate(t, tab, workspacePath+"/settings/"+c.section)
		expectAttribute(t, tab, "html", "lang", "es")
		eventually(t, tab, fmt.Sprintf(`[...document.querySelectorAll("h1, h2")].some((h) => h.textContent.trim() === %s)`, jsString(c.heading)), "Spanish heading missing for "+c.section)
		if got := eval[int](t, tab, fmt.Sprintf(`document.querySelectorAll('form[action$="/settings/%s"][method="post"] input[name="csrf"]').length`, c.section)); got < c.csrfForms {
			t.Fatalf("%s csrf forms = %d, want at least %d", c.section, got, c.csrfForms)
		}
	}
	navigate(t, tab, workspacePath+"/settings/email")
	expectCount(t, tab, `input[type="password"]`, 4)
	navigate(t, tab, workspacePath+"/settings/tokens")
	if eval[bool](t, tab, `document.body.textContent.includes("Copia este token ahora")`) {
		t.Fatal("one-time token copy shown on a plain load")
	}
}

// TestSettingsReflowAt320Pixels ports the token and provisioning reflow test.
func TestSettingsReflowAt320Pixels(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
	workspacePath := openWorkspaceSettings(t, tab, "/settings/general")
	run(t, tab, chromedp.EmulateViewport(320, 800))
	for _, section := range []string{"tokens", "provisioning"} {
		navigate(t, tab, workspacePath+"/settings/"+section)
		waitVisible(t, tab, "main h2")
		expectNoOverflow(t, tab, 320)
	}
}

// TestMemberSettingsPermissions ports the member permissions test: email is hidden and
// general settings are read-only for a member.
func TestMemberSettingsPermissions(t *testing.T) {
	tab := newTab(t, 1280, 900, "")
	signInAs(t, tab, fixtureMemberEmail, fixtureMemberPass)
	workspacePath := openWorkspaceSettings(t, tab, "/settings/general")
	waitVisible(t, tab, "#workspace-locale")
	expectCount(t, tab, `a[href$="/settings/email"]`, 0)
	if !eval[bool](t, tab, `document.getElementById("workspace-locale").disabled`) {
		t.Fatal("member can change the workspace locale")
	}
	expectCount(t, tab, `form[action$="/settings/general"] button[type="submit"]`, 0)
	if status := responseStatus(t, tab, baseURL+workspacePath+"/settings/email"); status != 403 {
		t.Fatalf("member email settings status %d, want 403", status)
	}
}
