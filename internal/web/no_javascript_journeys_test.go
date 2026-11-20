package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// These tests port the Playwright blocks that run with javaScriptEnabled:false.
// They assert the same server-visible outcomes: status codes, redirects,
// headers, cookies, and rendered HTML. Focus and in-browser checks stay on
// Playwright.

func noJSSession(t *testing.T, auth *identity.Service, userID identity.UserID) ([]*http.Cookie, string) {
	t.Helper()
	session, err := auth.CreateSession(context.Background(), userID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}, session.CSRFSecret
}

// requiredNamedInput reports whether an input with the given name carries the
// native required attribute, in any attribute order.
func requiredNamedInput(body, name string) bool {
	for _, tag := range regexp.MustCompile(`<input[^>]*>`).FindAllString(body, -1) {
		if strings.Contains(tag, `name="`+name+`"`) && regexp.MustCompile(`\srequired[\s>/]`).MatchString(tag) {
			return true
		}
	}
	return false
}

// snippetAround returns the text near the first occurrence of marker, for failure messages.
func snippetAround(body, marker string, width int) string {
	at := strings.Index(body, marker)
	if at < 0 {
		return "(marker absent)"
	}
	return body[max(0, at-width/2):min(len(body), at+width)]
}

func TestNoJavaScriptAccountProfileAndDeletionConfirmation(t *testing.T) {
	handler, auth, _, _, owner := testWeb(t, 10)
	cookies, csrf := noJSSession(t, auth, owner.UserID)

	page := request(handler, http.MethodGet, "/account", nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `type="checkbox" name="confirm_delete"`) || !strings.Contains(page.Body.String(), `value="yes"`) {
		t.Fatalf("account page lacks native deletion confirmation: %d %s", page.Code, page.Body.String())
	}

	saved := request(handler, http.MethodPost, "/account/profile", url.Values{
		"csrf": {csrf}, "display_name": {"No JavaScript profile"}, "locale": {""},
		"timezone": {"America/Bogota"}, "theme": {"light"},
	}, cookies, nil)
	if saved.Code != http.StatusSeeOther || saved.Header().Get("Location") != "/account?saved=1" {
		t.Fatalf("profile save = %d, location %q", saved.Code, saved.Header().Get("Location"))
	}

	deleted := request(handler, http.MethodPost, "/account/delete", url.Values{"csrf": {csrf}, "password": {"long-enough-password"}}, cookies, nil)
	if deleted.Code != http.StatusUnprocessableEntity || !strings.Contains(deleted.Body.String(), "Confirm permanent account deletion to continue.") {
		t.Fatalf("account deletion without confirmation = %d: %s", deleted.Code, deleted.Body.String())
	}
}

func TestNoJavaScriptWorkspaceExportAndImportPreview(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	cookies, csrf := noJSSession(t, auth, owner.UserID)
	root := "/workspaces/" + workspaceID.String() + "/settings/import"

	export := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/export/download", nil, cookies, nil)
	if export.Code != http.StatusOK || export.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(export.Header().Get("Content-Type"), "application/json") ||
		!strings.Contains(export.Body.String(), "dynamis-code.workspace/v1") {
		t.Fatalf("workspace export = %d headers=%v", export.Code, export.Header())
	}

	privateTitle := "Do not echo private title"
	invalid := multipartRequest(t, handler, root, url.Values{"csrf": {csrf}, "action": {"preview"}}, "invalid.csv", "title,status\n"+privateTitle+",unknown\n", cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "Record 1") {
		t.Fatalf("invalid CSV preview = %d: %s", invalid.Code, invalid.Body.String())
	}
	if errorsAt := strings.Index(invalid.Body.String(), "import-preview-errors"); errorsAt < 0 ||
		strings.Contains(invalid.Body.String()[errorsAt:min(len(invalid.Body.String()), errorsAt+800)], privateTitle) {
		t.Fatalf("invalid CSV error panel is missing or echoes the submitted title")
	}

	title := "Browser import"
	preview := multipartRequest(t, handler, root, url.Values{"csrf": {csrf}, "action": {"preview"}}, "items.csv", "title,status\n"+title+",active\n", cookies, nil)
	if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), title) || !strings.Contains(preview.Body.String(), "import-confirm-form") {
		t.Fatalf("valid CSV preview = %d: %s", preview.Code, preview.Body.String())
	}
}

func TestNoJavaScriptInvitationSignInAcceptsForExistingAccount(t *testing.T) {
	handler, auth, _, firstWorkspace, owner := testWeb(t, 10)
	ctx := context.Background()
	ownerPrincipal, err := auth.Authorize(ctx, owner.UserID, firstWorkspace, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	// The member account already exists: it joins the first workspace by invitation.
	first, err := auth.CreateInvitation(ctx, ownerPrincipal, "member@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.CreateInvitedLocalUser(ctx, first.Secret, "member-long-password", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	// A second workspace invites the existing account through the no-JS flow.
	second, err := auth.CreateWorkspace(ctx, ownerPrincipal, identity.WorkspaceCreateInput{Name: "Invitation handoff"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	secondPrincipal, err := auth.Authorize(ctx, owner.UserID, second, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, secondPrincipal, "member@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}

	path := "/invitations/" + invitation.Secret
	page := request(handler, http.MethodGet, path, nil, nil, nil)
	invitationCSRF := responseCookie(page, "invitation_csrf").Value
	started := request(handler, http.MethodPost, path+"/sign-in", url.Values{"csrf": {invitationCSRF}}, []*http.Cookie{{Name: "invitation_csrf", Value: invitationCSRF}}, nil)
	if started.Code != http.StatusSeeOther || started.Header().Get("Location") != invitationLoginLocation {
		t.Fatalf("invitation sign-in start = %d, location %q", started.Code, started.Header().Get("Location"))
	}
	handoff := []*http.Cookie{responseCookie(started, "invitation_handoff"), responseCookie(started, "invitation_handoff_binding")}

	login := request(handler, http.MethodGet, invitationLoginLocation, nil, handoff, nil)
	loginCSRF := responseCookie(login, "login_csrf")
	if login.Code != http.StatusOK || loginCSRF.Value == "" {
		t.Fatalf("invitation login page = %d", login.Code)
	}
	loggedIn := request(handler, http.MethodPost, "/login", url.Values{
		"email": {"member@example.com"}, "password": {"member-long-password"},
		"csrf": {loginCSRF.Value}, "return_to": {"/invitations/continue"},
	}, append([]*http.Cookie{loginCSRF}, handoff...), nil)
	if loggedIn.Code != http.StatusSeeOther || loggedIn.Header().Get("Location") != "/invitations/continue" {
		t.Fatalf("invitation login = %d, location %q", loggedIn.Code, loggedIn.Header().Get("Location"))
	}
	memberCookies := []*http.Cookie{responseCookie(loggedIn, "session"), responseCookie(loggedIn, "csrf")}
	continued := request(handler, http.MethodGet, "/invitations/continue", nil, append(append([]*http.Cookie(nil), memberCookies...), handoff...), nil)
	if continued.Code != http.StatusOK || !strings.Contains(continued.Body.String(), "Accept invitation") {
		t.Fatalf("invitation continue page = %d: %s", continued.Code, continued.Body.String())
	}
	accepted := request(handler, http.MethodPost, "/invitations/continue", url.Values{"csrf": {memberCookies[1].Value}}, append(append([]*http.Cookie(nil), memberCookies...), handoff...), nil)
	if accepted.Code != http.StatusSeeOther || accepted.Header().Get("Location") != "/workspaces/"+second.String()+"/items" {
		t.Fatalf("invitation accept = %d, location %q", accepted.Code, accepted.Header().Get("Location"))
	}
}

func TestNoJavaScriptWorkspaceSelectionValidationAndCreate(t *testing.T) {
	handler, auth, _, _, owner := testWeb(t, 10)
	cookies, csrf := noJSSession(t, auth, owner.UserID)
	spanish := append(append([]*http.Cookie(nil), cookies...), &http.Cookie{Name: "locale", Value: "es"})

	blank := request(handler, http.MethodPost, "/workspaces", url.Values{"csrf": {csrf}, "name": {"   "}}, spanish, nil)
	if blank.Code != http.StatusUnprocessableEntity || !strings.Contains(blank.Body.String(), `role="alert"`) {
		t.Fatalf("blank workspace name = %d: %s", blank.Code, blank.Body.String())
	}
	created := request(handler, http.MethodPost, "/workspaces", url.Values{"csrf": {csrf}, "name": {"Espacio de prueba"}}, spanish, nil)
	if created.Code != http.StatusSeeOther {
		t.Fatalf("workspace create = %d: %s", created.Code, created.Body.String())
	}
	home := request(handler, http.MethodGet, "/", nil, spanish, nil)
	if !strings.Contains(home.Body.String(), "Espacio de prueba") || !hasHTMLRootLang(home.Body.String(), "es") {
		t.Fatalf("Spanish workspace list missing new workspace: %s", home.Body.String())
	}
}

func TestNoJavaScriptItemCreateAndEditUseOrdinaryForms(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	cookies, csrf := noJSSession(t, auth, owner.UserID)
	path := "/workspaces/" + workspaceID.String() + "/items"

	created := request(handler, http.MethodPost, path, url.Values{
		"csrf": {csrf}, "title": {"No JavaScript item"}, "idempotency_key": {"no-js-create"},
	}, cookies, nil)
	if created.Code != http.StatusSeeOther && created.Code != http.StatusOK {
		t.Fatalf("ordinary item create = %d: %s", created.Code, created.Body.String())
	}
	list := request(handler, http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(list.Body.String(), `value="No JavaScript item"`) {
		t.Fatalf("created item missing from list: %s", list.Body.String())
	}
	var edit []string
	for _, candidate := range regexp.MustCompile(`="(/workspaces/[^"/]+/items/[^"/]+)"`).FindAllStringSubmatch(list.Body.String(), -1) {
		if !strings.HasSuffix(candidate[1], "/items/events") {
			edit = candidate
			break
		}
	}
	version := regexp.MustCompile(`name="version"[^>]*value="([0-9]+)"`).FindStringSubmatch(list.Body.String())
	if len(edit) != 2 || len(version) != 2 {
		t.Fatalf("item edit form missing action or version; version markup: %s", snippetAround(list.Body.String(), "version", 400))
	}
	saved := request(handler, http.MethodPost, edit[1], url.Values{
		"csrf": {csrf}, "action": {"update"}, "title": {"No JavaScript edit"}, "status": {"active"}, "version": {version[1]},
	}, cookies, nil)
	if saved.Code != http.StatusSeeOther && saved.Code != http.StatusOK {
		t.Fatalf("ordinary item edit to %s = %d: %s", edit[1], saved.Code, snippetAround(saved.Body.String(), "alert", 300))
	}
	after := request(handler, http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(after.Body.String(), `value="No JavaScript edit"`) {
		t.Fatalf("edited item missing from list: %s", after.Body.String())
	}
}

func TestNoJavaScriptSettingsFormsRenderAndValidate(t *testing.T) {
	handler, _, auth, _, workspaceID, owner := emailWebTest(t)
	cookies, csrf := noJSSession(t, auth, owner.UserID)
	base := "/workspaces/" + workspaceID.String() + "/settings"

	general := request(handler, http.MethodGet, base+"/general", nil, cookies, nil)
	if general.Code != http.StatusOK || !strings.Contains(general.Body.String(), `name="csrf"`) {
		t.Fatalf("general settings page = %d", general.Code)
	}
	saved := request(handler, http.MethodPost, base+"/general", url.Values{"csrf": {csrf}, "name": {"Example"}, "locale": {"en"}}, cookies, nil)
	if saved.Code != http.StatusSeeOther || !strings.Contains(saved.Header().Get("Location"), "saved=1") {
		t.Fatalf("general settings save = %d, location %q", saved.Code, saved.Header().Get("Location"))
	}

	email := request(handler, http.MethodGet, base+"/email", nil, cookies, nil)
	for _, provider := range []string{"smtp", "resend", "postmark"} {
		if !strings.Contains(email.Body.String(), `data-show-when="email-provider=`+provider+`"`) {
			t.Fatalf("email settings missing %s fieldset; show-when markup: %s", provider, snippetAround(email.Body.String(), "show-when", 300))
		}
	}
	invalid := request(handler, http.MethodPost, base+"/email", url.Values{
		"csrf": {csrf}, "provider": {"smtp"}, "sender_address": {"invalid@example.com"}, "host": {""}, "port": {"587"},
	}, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "The email settings are invalid.") {
		t.Fatalf("invalid email settings = %d: %s", invalid.Code, invalid.Body.String())
	}

	tokens := request(handler, http.MethodGet, base+"/tokens", nil, cookies, nil)
	if tokens.Code != http.StatusOK || !strings.Contains(tokens.Body.String(), `name="name"`) || !strings.Contains(tokens.Body.String(), `name="csrf"`) {
		t.Fatalf("tokens page lacks ordinary form: %d", tokens.Code)
	}

	provisioning := request(handler, http.MethodGet, base+"/provisioning", nil, cookies, nil)
	if provisioning.Code != http.StatusOK || strings.Count(provisioning.Body.String(), `action="`+base+`/provisioning" method="post"`) != 2 {
		t.Fatalf("provisioning page should render two ordinary POST forms: %d", provisioning.Code)
	}
}

func TestNoJavaScriptWebhookDeleteNeedsNativeConfirmation(t *testing.T) {
	handler, _, auth, _, workspaceID, owner, _ := webhookWebTest(t)
	cookies, csrf := noJSSession(t, auth, owner.UserID)
	base := "/workspaces/" + workspaceID.String() + "/settings/webhooks"

	created := request(handler, http.MethodPost, base, url.Values{
		"action": {"create"}, "csrf": {csrf}, "name": {"No-JS fixture"},
		"url": {"https://hooks.example.test/no-js"}, "events": {"item.created"},
	}, cookies, nil)
	if created.Code != http.StatusOK {
		t.Fatalf("webhook create = %d: %s", created.Code, created.Body.String())
	}
	page := request(handler, http.MethodGet, base, nil, cookies, nil)
	historyPath := regexp.MustCompile(`href="(` + regexp.QuoteMeta(base) + `/[^"]+/deliveries)"`).FindStringSubmatch(page.Body.String())
	if len(historyPath) != 2 {
		t.Fatalf("webhook history link missing: %s", page.Body.String())
	}
	history := request(handler, http.MethodGet, historyPath[1], nil, cookies, nil)
	if history.Header().Get("Cache-Control") != "no-store" || !strings.Contains(history.Body.String(), "No deliveries yet.") {
		t.Fatalf("empty delivery history = headers %v body %s", history.Header(), history.Body.String())
	}
	if !requiredNamedInput(page.Body.String(), "confirm_delete") {
		t.Fatalf("webhook delete lacks required native confirmation: %s", snippetAround(page.Body.String(), "confirm_delete", 300))
	}
	webhookPath := strings.TrimSuffix(historyPath[1], "/deliveries")
	unconfirmed := request(handler, http.MethodPost, webhookPath, url.Values{"action": {"delete"}, "csrf": {csrf}}, cookies, nil)
	if unconfirmed.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unconfirmed delete = %d", unconfirmed.Code)
	}
	deleted := request(handler, http.MethodPost, webhookPath, url.Values{"action": {"delete"}, "csrf": {csrf}, "confirm_delete": {"yes"}}, cookies, nil)
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != base {
		t.Fatalf("confirmed delete = %d, location %q", deleted.Code, deleted.Header().Get("Location"))
	}
}

func TestNoJavaScriptNotificationsEmptyStateAndMarkRead(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	cookies, csrf := noJSSession(t, auth, owner.UserID)
	spanish := append(append([]*http.Cookie(nil), cookies...), &http.Cookie{Name: "locale", Value: "es"})

	empty := request(handler, http.MethodGet, "/notifications", nil, spanish, nil)
	if !strings.Contains(empty.Body.String(), "No hay notificaciones.") || !hasHTMLRootLang(empty.Body.String(), "es") {
		t.Fatalf("Spanish empty notifications = %s", empty.Body.String())
	}

	existing, err := auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "Playwright notification", Body: "Fixture body",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	marked := request(handler, http.MethodPost, "/notifications/"+existing.ID.String(), url.Values{"csrf": {csrf}}, cookies, nil)
	if marked.Code != http.StatusSeeOther {
		t.Fatalf("mark read without JavaScript = %d: %s", marked.Code, marked.Body.String())
	}
	list := request(handler, http.MethodGet, "/notifications", nil, cookies, nil)
	if strings.Contains(list.Body.String(), "Mark as read") {
		t.Fatalf("read notification still offers Mark as read: %s", list.Body.String())
	}
}

func TestNoJavaScriptMembersEmptyAndFilterStates(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	cookies, _ := noJSSession(t, auth, owner.UserID)
	path := "/workspaces/" + workspaceID.String() + "/settings/members"

	filtered := request(handler, http.MethodGet, path+"?email=nobody%40example.com", nil, cookies, nil)
	if filtered.Code != http.StatusOK || !strings.Contains(filtered.Body.String(), "No members match these filters.") {
		t.Fatalf("members filter with no matches = %d: %s", filtered.Code, filtered.Body.String())
	}
	if strings.Contains(filtered.Body.String(), "owner@example.com") {
		t.Fatalf("members filter leaked unrelated owner row: %s", filtered.Body.String())
	}
}
