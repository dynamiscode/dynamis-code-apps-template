//go:build browser

package browsertest

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

const memberFixturePassword = "members-fixture-password-123"

// signInAs signs an account in on the tab and waits for the home page.
func signInAs(t *testing.T, tab context.Context, email, password string) {
	t.Helper()
	navigate(t, tab, "/login")
	fill(t, tab, "#email", email)
	fill(t, tab, "#password", password)
	click(t, tab, `button[type="submit"]`)
	eventually(t, tab, fmt.Sprintf("location.href === %s", jsString(baseURL+"/")), "sign-in did not reach the home page")
}

// openWorkspaceSettings opens the first workspace and the given settings path under it.
func openWorkspaceSettings(t *testing.T, tab context.Context, suffix string) string {
	t.Helper()
	click(t, tab, `.cards a[href^="/workspaces/"]`)
	waitVisible(t, tab, "[data-workspace-home]")
	workspacePath := regexp.MustCompile(`^(/workspaces/[^/]+)`).FindString(eval[string](t, tab, "location.pathname"))
	navigate(t, tab, workspacePath+suffix)
	return workspacePath
}

// selectByLabel chooses an option of the select labelled name.
func selectByLabel(t *testing.T, tab context.Context, name, value string) {
	t.Helper()
	eventually(t, tab, fmt.Sprintf("(() => { return Boolean(%s); })()", labelledControl(name)), "no select labelled "+name)
	eval[any](t, tab, fmt.Sprintf(`(() => { const control = %s; control.value = %s; control.dispatchEvent(new Event("change", { bubbles: true })); })()`, labelledControl(name), jsString(value)))
}

// checkByLabel ticks the checkbox labelled name.
func checkByLabel(t *testing.T, tab context.Context, name string) {
	t.Helper()
	eval[any](t, tab, fmt.Sprintf(`(() => { const control = %s; control.checked = true; control.dispatchEvent(new Event("change", { bubbles: true })); })()`, labelledControl(name)))
}

// postStatus posts a form from the page with fetch and returns the HTTP status.
func postStatus(t *testing.T, tab context.Context, path string, fields map[string]string) int {
	t.Helper()
	values := make([]string, 0, len(fields))
	for key, value := range fields {
		values = append(values, fmt.Sprintf("[%s, %s]", jsString(key), jsString(value)))
	}
	status := eval[int](t, tab, fmt.Sprintf(`(async () => {
		const body = new URLSearchParams([%s]);
		return (await fetch(%s, { method: "POST", body, redirect: "manual", credentials: "same-origin" })).status;
	})()`, strings.Join(values, ","), jsString(path)), awaitPromise)
	return status
}

// inputValue reads an input's value on the page, for CSRF and member IDs.
func inputValue(t *testing.T, tab context.Context, selector string) string {
	t.Helper()
	return eval[string](t, tab, fmt.Sprintf("document.querySelector(%s)?.value ?? \"\"", jsString(selector)))
}

// acceptInvitation opens an invitation link in a new browser, sets the password and registers.
func acceptInvitation(t *testing.T, link string) context.Context {
	t.Helper()
	member := newTab(t, 1280, 900, "")
	run(t, member, chromedp.Navigate(link))
	fill(t, member, "#invite-password", memberFixturePassword)
	click(t, member, `button[name="action"][value="register"]`)
	eventually(t, member, `document.querySelector("form[action='/logout'], button[type=submit]") !== null`, "member registration did not render")
	return member
}

// inviteMember creates an invitation as the owner and accepts it in a separate browser.
func inviteMember(t *testing.T, owner context.Context, workspacePath, email, role string) context.Context {
	t.Helper()
	navigate(t, owner, workspacePath+"/settings/invitations")
	fillByLabel(t, owner, "Email", email)
	selectByLabel(t, owner, "Role", role)
	click(t, owner, `button[name="action"][value="create"]`)
	waitVisible(t, owner, `input[aria-label="Copy this invitation link"]`)
	link := eval[string](t, owner, `(() => { const input = document.querySelector('input[aria-label="Copy this invitation link"]'); const value = input.value; input.value = ""; return value; })()`)
	navigate(t, owner, workspacePath+"/settings/members")
	return acceptInvitation(t, baseURLJoin(link))
}

func baseURLJoin(link string) string {
	if strings.HasPrefix(link, "http") {
		return link
	}
	return baseURL + link
}

// shimWebMCP installs the test shim used when the browser has no native document.modelContext.
const shimWebMCP = `(() => {
	if (document.modelContext) return;
	Object.defineProperty(document, "__membersWebMCPTestShim", { value: true });
	const tools = [];
	Object.defineProperty(document, "modelContext", { value: {
		registerTool(tool) { tools.push(tool); },
		getTools() { return Promise.resolve(tools); },
		executeTool(tool, input) { return Promise.resolve(tool.execute(JSON.parse(input))); },
	} });
})();`

// TestMembersLayouts ports the members route layout checks. Screenshot comparisons are
// deferred to the baseline recapture.
func TestMembersLayouts(t *testing.T) {
	for _, c := range []struct {
		name  string
		width int64
		theme string
	}{
		{"desktop light", 1280, "light"}, {"desktop dark", 1280, "dark"},
		{"mobile light", 390, "light"}, {"mobile dark", 390, "dark"},
	} {
		t.Run(c.name, func(t *testing.T) {
			tab := newTab(t, c.width, 900, c.theme)
			signInAs(t, tab, fixtureOwnerEmail, fixturePassword)
			openWorkspaceSettings(t, tab, "/settings/members")
			waitVisible(t, tab, "#member-list")
			if !regexp.MustCompile("Members").MatchString(eval[string](t, tab, `[...document.querySelectorAll("h1")].map((h) => h.textContent).join(" ")`)) {
				t.Fatal("members heading missing")
			}
			if c.width < 608 {
				waitVisible(t, tab, ".list-view-cards")
			} else {
				waitVisible(t, tab, ".list-view-table")
			}
			expectAttribute(t, tab, "html", "data-theme", "system")
			if scheme := eval[string](t, tab, `getComputedStyle(document.documentElement).colorScheme`); scheme != c.theme {
				t.Fatalf("color-scheme %q, want %q", scheme, c.theme)
			}
			expectNoOverflow(t, tab, c.width)
		})
	}
}

// TestMembersActionsAndPermissions ports the members action, confirmation, permission,
// WebMCP-prepared role change and Spanish test.
func TestMembersActionsAndPermissions(t *testing.T) {
	owner := newTab(t, 1280, 900, "")
	// The shim must exist before the first document loads, as addInitScript did.
	call(t, owner, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: shimWebMCP})
	call(t, owner, emulation.SetFocusEmulationEnabled, emulation.SetFocusEmulationEnabledParams{Enabled: true})
	signInAs(t, owner, fixtureOwnerEmail, fixturePassword)
	workspacePath := openWorkspaceSettings(t, owner, "/settings/members")
	membersPath := workspacePath + "/settings/members"

	removed := inviteMember(t, owner, workspacePath, "removed@example.com", "member")
	inviteMember(t, owner, workspacePath, "transferred@example.com", "member")
	navigate(t, owner, membersPath)
	waitVisible(t, owner, "#member-list")
	rowExists := func(email string) string {
		return fmt.Sprintf(`[...document.querySelectorAll(".list-view-table tbody tr")].some((r) => r.textContent.includes(%s))`, jsString(email))
	}
	eventually(t, owner, rowExists("removed@example.com"), "removed member row missing")
	eventually(t, owner, rowExists("transferred@example.com"), "transferred member row missing")
	// Each non-owner member renders each action form twice (table and cards). The count follows the rows, since other tests share the database.
	rows := eval[int](t, owner, `document.querySelectorAll(".list-view-table tbody tr").length`)
	if rows < 2 {
		t.Fatalf("members table has %d rows", rows)
	}
	for _, action := range []string{"member-role-update", "member-remove", "ownership-transfer"} {
		expectCount(t, owner, fmt.Sprintf(`form[data-webmcp-actions="%s"][data-webmcp-id]`, action), 2*(rows-1))
	}

	run(t, owner, chromedp.EmulateViewport(390, 844))
	waitVisible(t, owner, ".list-view-cards")
	expectAxeClean(t, owner)

	run(t, owner, chromedp.EmulateViewport(1280, 900))
	roleMemberID := eval[string](t, owner, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes("removed@example.com"));
		return row.querySelector('form[data-webmcp-actions="member-role-update"] input[name="member_id"]').value;
	})()`))
	consequence := eval[string](t, owner, `(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes("removed@example.com"));
		return row.querySelector('input[name="confirm_remove"]').getAttribute("aria-describedby") ?? "";
	})()`)
	if consequence == "" {
		t.Fatal("remove confirmation has no consequence description")
	}
	eventually(t, owner, fmt.Sprintf(`(() => { const el = document.getElementById(%s); return Boolean(el) && el.getClientRects().length > 0; })()`, jsString(consequence)), "remove consequence not visible")

	prepared := eval[struct {
		Result    map[string]any `json:"result"`
		Selected  string         `json:"selected"`
		Focused   bool           `json:"focused"`
		Submitted bool           `json:"submitted"`
	}](t, owner, fmt.Sprintf(`(async () => {
		const context = document.modelContext;
		const tools = await context.getTools();
		const tool = tools.find((candidate) => candidate.name === "member-role-update-v1");
		let submitted = false;
		const form = [...document.querySelectorAll('form[data-webmcp-actions="member-role-update"]')]
			.find((candidate) => candidate.getClientRects().length > 0 && candidate.dataset.webmcpId === %s);
		form.addEventListener("submit", () => { submitted = true; }, { once: true });
		const result = await context.executeTool(tool, JSON.stringify({ memberId: %s, role: "viewer" }));
		return { result, selected: form.querySelector('select[name="role"]').value, focused: document.activeElement === form.querySelector('button[value="role"]'), submitted };
	})()`, jsString(roleMemberID), jsString(roleMemberID)), awaitPromise)
	if prepared.Result["tool"] != "member-role-update-v1" || prepared.Result["status"] != "ready-for-user-submission" {
		t.Fatalf("WebMCP prepared result %v", prepared.Result)
	}
	if prepared.Selected != "viewer" || !prepared.Focused || prepared.Submitted {
		t.Fatalf("WebMCP preparation %+v", prepared)
	}

	csrf := eval[string](t, owner, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes("removed@example.com"));
		return row.querySelector('form[data-webmcp-actions="member-role-update"] input[name="csrf"]').value;
	})()`))
	if status := postStatus(t, owner, membersPath, map[string]string{"csrf": csrf, "member_id": roleMemberID, "action": "role", "role": "owner"}); status != 422 {
		t.Fatalf("invalid role POST status %d, want 422", status)
	}

	eval[any](t, owner, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes("removed@example.com"));
		row.querySelector('select[name="role"]').focus();
	})()`))
	press(t, owner, "\t")
	eventually(t, owner, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes("removed@example.com"));
		return document.activeElement === row.querySelector('form[data-webmcp-actions="member-role-update"] button[value="role"]') || document.activeElement?.textContent?.trim() === "Change role";
	})()`), "Change role did not receive keyboard focus")
	selectByLabelInRow(t, owner, "removed@example.com", "viewer")
	clickInRow(t, owner, "removed@example.com", "Change role")
	eventually(t, owner, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes("removed@example.com"));
		return Boolean(row) && row.textContent.includes("Viewer");
	})()`), "role change did not show Viewer")

	if status := navigateStatus(t, removed, membersPath); status != 200 {
		t.Fatalf("viewer members page status %d, want 200", status)
	}
	expectCount(t, removed, `form[data-webmcp-actions="member-role-update"]`, 0)
	expectCount(t, removed, `form[data-webmcp-actions="member-remove"]`, 0)
	removedCSRF := inputValue(t, removed, `form.list-view-column input[name=csrf]`)
	for _, fields := range []map[string]string{
		{"csrf": removedCSRF, "member_id": roleMemberID, "action": "role", "role": "member"},
		{"csrf": removedCSRF, "member_id": roleMemberID, "action": "remove", "confirm_remove": "yes"},
	} {
		if status := postStatus(t, removed, membersPath, fields); status != 403 {
			t.Fatalf("viewer action %s status %d, want 403", fields["action"], status)
		}
	}
	if status := postStatus(t, owner, membersPath, map[string]string{"member_id": roleMemberID, "action": "remove", "confirm_remove": "yes"}); status != 403 {
		t.Fatalf("POST without CSRF status %d, want 403", status)
	}

	navigate(t, owner, "/language?locale=es&return_to="+url.QueryEscape(membersPath))
	eventually(t, owner, `[...document.querySelectorAll("body *")].some((el) => el.childElementCount === 0 && el.textContent.trim() === "Esto quita su acceso al espacio de trabajo y deja de compartir sus recursos con esta persona.")`, "Spanish removal consequence not shown")

	noJS := newTab(t, 1280, 900, "")
	call(t, noJS, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true})
	signInAs(t, noJS, fixtureOwnerEmail, fixturePassword)
	openWorkspaceSettings(t, noJS, "/settings/members")
	selectByLabelInRow(t, noJS, "removed@example.com", "member")
	clickInRow(t, noJS, "removed@example.com", "Change role")
	eventually(t, noJS, rowTextCheck("removed@example.com", "Member"), "no-JS role change did not show Member")

	clickInRow(t, noJS, "removed@example.com", "Remove")
	eventually(t, noJS, rowTextCheck("removed@example.com", ""), "no-JS remove without confirmation removed the row")
	checkInRow(t, noJS, "removed@example.com", "Remove this member?")
	clickInRow(t, noJS, "removed@example.com", "Remove")
	eventually(t, noJS, fmt.Sprintf(`![...document.querySelectorAll(".list-view-table tbody tr")].some((r) => r.textContent.includes(%s))`, jsString("removed@example.com")), "no-JS remove did not remove the row")

	clickInRow(t, noJS, "transferred@example.com", "Transfer ownership")
	checkInRow(t, noJS, "transferred@example.com", "Transfer ownership to this member?")
	clickInRow(t, noJS, "transferred@example.com", "Transfer ownership")
	eventually(t, noJS, `/settings\/members/.test(location.pathname)`, "transfer did not stay on members settings")

	if status := navigateStatus(t, removed, membersPath); status != 404 {
		t.Fatalf("removed member members page status %d, want 404", status)
	}
}

// navigateStatus loads a path and returns the document's HTTP status.
func navigateStatus(t *testing.T, tab context.Context, path string) int64 {
	t.Helper()
	ctx, cancel := context.WithTimeout(tab, actionWait)
	defer cancel()
	response, err := chromedp.Run(ctx, chromedp.NavigateResponse(baseURL+path))
	if err != nil || response == nil {
		t.Fatalf("navigate %s: %v", path, err)
	}
	return response.Status
}

func rowTextCheck(email, text string) string {
	return fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes(%s));
		return Boolean(row) && row.textContent.includes(%s);
	})()`, jsString(email), jsString(text))
}

func selectByLabelInRow(t *testing.T, tab context.Context, email, value string) {
	t.Helper()
	eval[any](t, tab, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes(%s));
		const select = row.querySelector('select[name="role"]');
		select.value = %s;
		select.dispatchEvent(new Event("change", { bubbles: true }));
	})()`, jsString(email), jsString(value)))
}

func clickInRow(t *testing.T, tab context.Context, email, name string) {
	t.Helper()
	eval[any](t, tab, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes(%s));
		const control = [...row.querySelectorAll("button, a, summary")].find((el) => el.textContent.trim().includes(%s));
		control.click();
	})()`, jsString(email), jsString(name)))
	settle(t, tab)
}

func checkInRow(t *testing.T, tab context.Context, email, label string) {
	t.Helper()
	eval[any](t, tab, fmt.Sprintf(`(() => {
		const row = [...document.querySelectorAll(".list-view-table tbody tr")].find((r) => r.textContent.includes(%s));
		const control = [...row.querySelectorAll("label")].find((l) => l.textContent.includes(%s));
		const input = control.querySelector("input") ?? document.getElementById(control.htmlFor);
		input.checked = true;
		input.dispatchEvent(new Event("change", { bubbles: true }));
	})()`, jsString(email), jsString(label)))
}

func expectCount(t *testing.T, tab context.Context, selector string, want int) {
	t.Helper()
	if got := eval[int](t, tab, fmt.Sprintf("document.querySelectorAll(%s).length", jsString(selector))); got != want {
		t.Fatalf("%s count = %d, want %d", selector, got, want)
	}
}
