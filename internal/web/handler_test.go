package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/items"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/portability"
)

func TestMFAOptionsRenderAsJSON(t *testing.T) {
	handler, err := NewHandlerWithServices(nil, nil, nil, nil, config.HTTP{}, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	data := pageData{
		Locale: "en", Catalog: catalog,
		MFAOptions: json.RawMessage(`{"publicKey":{"challenge":"abc"}}`),
	}
	if err := handler.template.ExecuteTemplate(&body, "mfa.html", data); err != nil {
		t.Fatal(err)
	}
	start := strings.Index(body.String(), `<script id="mfa-options" type="application/json">`) + len(`<script id="mfa-options" type="application/json">`)
	end := strings.Index(body.String()[start:], `</script>`)
	if start < len(`<script id="mfa-options" type="application/json">`) || end < 0 {
		t.Fatalf("MFA options script missing: %s", body.String())
	}
	var options struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal([]byte(body.String()[start:start+end]), &options); err != nil {
		t.Fatalf("MFA options are not a JSON object: %v; body=%s", err, body.String())
	}
	if options.PublicKey.Challenge != "abc" {
		t.Fatalf("challenge = %q", options.PublicKey.Challenge)
	}
}

func TestCompleteMFASessionPreservesReturnTo(t *testing.T) {
	handler := &Handler{}
	request := httptest.NewRequest(http.MethodGet, "/mfa", nil)
	request.AddCookie(&http.Cookie{Name: "mfa_return_to", Value: "/invitations/example"})
	response := httptest.NewRecorder()
	returnTo := handler.completeMFASession(response, request, identity.NewSession{Secret: "session", CSRFSecret: "csrf"})
	if returnTo != "/invitations/example" {
		t.Fatalf("return_to = %q", returnTo)
	}
	if cookie := responseCookie(response, "mfa_return_to"); cookie.MaxAge != -1 {
		t.Fatalf("mfa_return_to cookie max age = %d", cookie.MaxAge)
	}
}

func TestWebLoginItemsHTMXAndCSRF(t *testing.T) {
	handler, auth, itemService, workspaceID, _ := testWeb(t, 10)

	loginPage := request(handler, http.MethodGet, "/login", nil, nil, nil)
	if loginPage.Code != http.StatusOK || !strings.Contains(loginPage.Body.String(), `<label for="email">`) {
		t.Fatalf("login page = %d, %s", loginPage.Code, loginPage.Body.String())
	}
	loginCSRF := responseCookie(loginPage, "login_csrf")
	invalidLogin := request(handler, http.MethodPost, "/login", url.Values{
		"email": {"owner@example.com"}, "password": {"long-enough-password"}, "csrf": {"wrong"},
	}, nil, nil)
	if invalidLogin.Code != http.StatusForbidden {
		t.Fatalf("invalid login CSRF = %d", invalidLogin.Code)
	}
	loggedIn := request(handler, http.MethodPost, "/login", url.Values{
		"email": {"owner@example.com"}, "password": {"long-enough-password"}, "csrf": {loginCSRF.Value},
	}, []*http.Cookie{loginCSRF}, nil)
	if loggedIn.Code != http.StatusSeeOther || loggedIn.Header().Get("Location") != "/" {
		t.Fatalf("login = %d, %s", loggedIn.Code, loggedIn.Body.String())
	}
	cookies := []*http.Cookie{responseCookie(loggedIn, "session"), responseCookie(loggedIn, "csrf")}
	home := request(handler, http.MethodGet, "/", nil, cookies, nil)
	if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), workspaceID) {
		t.Fatalf("home = %d, %s", home.Code, home.Body.String())
	}
	path := "/workspaces/" + workspaceID + "/items"
	page := request(handler, http.MethodGet, path, nil, cookies, nil)
	assertAccessiblePage(t, page.Body.String(), "New item title")
	if !strings.Contains(page.Body.String(), `hx-target="#item-list"`) {
		t.Fatal("item page lacks targeted HTMX enhancement")
	}
	withoutCSRF := request(handler, http.MethodPost, path, url.Values{
		"title": {"Blocked"}, "idempotency_key": {"blocked-key"},
	}, cookies, nil)
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF = %d", withoutCSRF.Code)
	}
	csrf := cookies[1].Value
	ordinaryInvalid := request(handler, http.MethodPost, path, url.Values{
		"title": {""}, "idempotency_key": {"ordinary-invalid-key"}, "csrf": {csrf},
	}, cookies, nil)
	if ordinaryInvalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ordinary validation response = %d", ordinaryInvalid.Code)
	}
	invalid := request(handler, http.MethodPost, path, url.Values{
		"title": {""}, "idempotency_key": {"invalid-key"}, "csrf": {csrf},
	}, cookies, map[string]string{"HX-Request": "true"})
	if invalid.Code != http.StatusOK || invalid.Header().Get("HX-Trigger") != "form-error" || !strings.Contains(invalid.Body.String(), `role="alert"`) {
		t.Fatalf("validation response = %d, %s", invalid.Code, invalid.Body.String())
	}
	created := request(handler, http.MethodPost, path, url.Values{
		"title": {"<script>alert(1)</script>"}, "idempotency_key": {"create-key"}, "csrf": {csrf},
	}, cookies, map[string]string{"HX-Request": "true"})
	if created.Code != http.StatusOK || strings.Contains(created.Body.String(), "<script>alert") || strings.Contains(created.Body.String(), "<!doctype") {
		t.Fatalf("HTMX create = %d, %s", created.Code, created.Body.String())
	}
	session, err := auth.AuthenticateSession(context.Background(), cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := auth.Authorize(context.Background(), session.UserID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	itemsPage, err := itemService.List(context.Background(), principal, workspaceID, items.ListInput{Sort: "-created_at", Limit: 10})
	if err != nil || len(itemsPage.Items) != 1 {
		t.Fatalf("items = %+v, %v", itemsPage, err)
	}
	item := itemsPage.Items[0]
	deleted := request(handler, http.MethodPost, path+"/"+item.ID, url.Values{
		"action": {"delete"}, "version": {"1"}, "csrf": {csrf},
	}, cookies, map[string]string{"HX-Request": "true"})
	if deleted.Code != http.StatusOK || strings.Contains(deleted.Body.String(), item.Title) {
		t.Fatalf("delete = %d, %s", deleted.Code, deleted.Body.String())
	}
}

func TestWebAccountNotificationsAndPasswordReset(t *testing.T) {
	mailer := &recordingMailer{}
	handler, auth, _, workspaceID, owner := testWeb(t, 10, mailer)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	account := request(handler, http.MethodGet, "/account", nil, cookies, nil)
	if account.Code != http.StatusOK || !strings.Contains(account.Body.String(), "Profile and preferences") {
		t.Fatalf("account page = %d, %s", account.Code, account.Body.String())
	}
	updated := request(handler, http.MethodPost, "/account/profile", url.Values{
		"csrf": {session.CSRFSecret}, "display_name": {"Owner"}, "locale": {"es"},
		"timezone": {"America/Bogota"}, "theme": {"dark"},
	}, cookies, nil)
	if updated.Code != http.StatusSeeOther || updated.Header().Get("Location") != "/account?saved=1" {
		t.Fatalf("profile update = %d, %q", updated.Code, updated.Header().Get("Location"))
	}
	if profile, err := auth.GetUserProfile(context.Background(), owner.UserID); err != nil || profile.DisplayName != "Owner" || profile.Timezone != "America/Bogota" || profile.Theme != "dark" {
		t.Fatalf("profile = %+v, %v", profile, err)
	}
	preference := request(handler, http.MethodPost, "/account/notifications", url.Values{
		"csrf": {session.CSRFSecret}, "notification_type": {"system"}, "enabled": {"false"},
	}, cookies, nil)
	if preference.Code != http.StatusSeeOther {
		t.Fatalf("notification preference = %d, %s", preference.Code, preference.Body.String())
	}

	notification, err := auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "Update", Body: "A workspace update",
	}, identity.AuditContext{})
	if err != nil || notification.ID == "" {
		t.Fatalf("notification = %+v, %v", notification, err)
	}
	notifications := request(handler, http.MethodGet, "/notifications", nil, cookies, nil)
	if notifications.Code != http.StatusOK || !strings.Contains(notifications.Body.String(), "A workspace update") {
		t.Fatalf("notifications page = %d, %s", notifications.Code, notifications.Body.String())
	}
	marked := request(handler, http.MethodPost, "/notifications/"+notification.ID, url.Values{
		"csrf": {session.CSRFSecret},
	}, cookies, nil)
	if marked.Code != http.StatusSeeOther {
		t.Fatalf("notification mark read = %d", marked.Code)
	}

	resetPage := request(handler, http.MethodGet, "/password-reset", nil, nil, nil)
	resetCSRF := responseCookie(resetPage, "reset_csrf")
	resetRequested := request(handler, http.MethodPost, "/password-reset", url.Values{
		"csrf": {resetCSRF.Value}, "email": {"owner@example.com"},
	}, []*http.Cookie{resetCSRF, {Name: "locale", Value: "es"}}, nil)
	if resetRequested.Code != http.StatusOK || !strings.Contains(resetRequested.Body.String(), "Si existe una cuenta") {
		t.Fatalf("reset request = %d, %s", resetRequested.Code, resetRequested.Body.String())
	}
	if mailer.subject != "Restablece tu contraseña de Dynamis Code" {
		t.Fatalf("reset subject = %q", mailer.subject)
	}
	resetLink := strings.TrimSpace(strings.TrimPrefix(mailer.body, "Restablece tu contraseña con este enlace: "))
	parsed, err := url.Parse(resetLink)
	if err != nil || parsed.Path == "" {
		t.Fatalf("reset link = %q, %v", resetLink, err)
	}
	resetTokenPage := request(handler, http.MethodGet, parsed.Path, nil, nil, nil)
	resetTokenCSRF := responseCookie(resetTokenPage, "reset_csrf")
	resetComplete := request(handler, http.MethodPost, parsed.Path, url.Values{
		"csrf": {resetTokenCSRF.Value}, "password": {"reset-owner-password"}, "password_confirmation": {"reset-owner-password"},
	}, []*http.Cookie{resetTokenCSRF}, nil)
	if resetComplete.Code != http.StatusOK || !strings.Contains(resetComplete.Body.String(), "Password reset") {
		t.Fatalf("reset complete = %d, %s", resetComplete.Code, resetComplete.Body.String())
	}
	if _, err := auth.AuthenticateLocal(context.Background(), "owner@example.com", "reset-owner-password"); err != nil {
		t.Fatal(err)
	}
}

func PrincipalSystem() identity.Principal {
	return identity.Principal{AuthMethod: "system"}
}

func TestWebLocalePrecedenceAndWorkspaceFallback(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	spanish := requestFrom(handler, http.MethodGet, "/", nil, cookies, map[string]string{"Accept-Language": "es-MX,es;q=0.9"}, "example.com", "192.0.2.1:1234")
	if spanish.Header().Get("Content-Language") != "es" || !strings.Contains(spanish.Body.String(), `<html lang="es">`) || !strings.Contains(spanish.Body.String(), "Espacios de trabajo") {
		t.Fatalf("Spanish home = %d, %s", spanish.Code, spanish.Body.String())
	}

	if err := auth.SetUserLocale(context.Background(), owner.UserID, "en", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	userPreference := requestFrom(handler, http.MethodGet, "/", nil, append(cookies, &http.Cookie{Name: "locale", Value: "es"}), map[string]string{"Accept-Language": "es"}, "example.com", "192.0.2.1:1234")
	if userPreference.Header().Get("Content-Language") != "en" || !strings.Contains(userPreference.Body.String(), `<html lang="en">`) {
		t.Fatalf("user preference precedence = %d, %s", userPreference.Code, userPreference.Body.String())
	}
	if err := auth.SetUserLocale(context.Background(), owner.UserID, "", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}

	workspace := requestFrom(handler, http.MethodGet, "/workspaces/"+workspaceID, nil, cookies, map[string]string{"Accept-Language": "en"}, "example.com", "192.0.2.1:1234")
	if workspace.Header().Get("Content-Language") != "en" {
		t.Fatalf("workspace default locale = %q", workspace.Header().Get("Content-Language"))
	}
	ownerPrincipal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdateWorkspaceLocale(context.Background(), ownerPrincipal, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	workspace = requestFrom(handler, http.MethodGet, "/workspaces/"+workspaceID, nil, cookies, map[string]string{"Accept-Language": "en"}, "example.com", "192.0.2.1:1234")
	if workspace.Header().Get("Content-Language") != "es" || !strings.Contains(workspace.Body.String(), `<html lang="es">`) {
		t.Fatalf("workspace fallback = %q, %s", workspace.Header().Get("Content-Language"), workspace.Body.String())
	}
}

type recordingMailer struct {
	subject string
	body    string
}

func (m *recordingMailer) Send(_ context.Context, _ string, subject, body string) error {
	m.subject, m.body = subject, body
	return nil
}

func TestInvitationEmailUsesWorkspaceLocale(t *testing.T) {
	mailer := &recordingMailer{}
	handler, auth, _, workspaceID, owner := testWeb(t, 10, mailer)
	ownerPrincipal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdateWorkspaceLocale(context.Background(), ownerPrincipal, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	request(handler, http.MethodPost, "/workspaces/"+workspaceID+"/settings/invitations", url.Values{
		"action": {"create"}, "csrf": {session.CSRFSecret}, "email": {"invite@example.com"}, "role": {"member"},
	}, []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}, nil)
	if mailer.subject != "Has sido invitado a Dynamis Code" || !strings.HasPrefix(mailer.body, "Abre este enlace de invitación:") {
		t.Fatalf("invitation email = subject %q body %q", mailer.subject, mailer.body)
	}
}

func TestWebLanguageSettingsRoutes(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	language := request(handler, http.MethodGet, "/language?locale=es&return_to=/", nil, cookies, nil)
	if language.Code != http.StatusSeeOther || language.Header().Get("Location") != "/" || responseCookie(language, "locale").Value != "es" {
		t.Fatalf("language route = %d, location %q, cookie %q", language.Code, language.Header().Get("Location"), responseCookie(language, "locale").Value)
	}
	if invalid := request(handler, http.MethodGet, "/language?locale=fr&return_to=https://example.com", nil, cookies, nil); invalid.Header().Get("Location") != "/" || responseCookie(invalid, "locale").Value != "" {
		t.Fatalf("invalid language route = %d, location %q", invalid.Code, invalid.Header().Get("Location"))
	}

	settings := request(handler, http.MethodGet, "/settings/language", nil, cookies, nil)
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), "Automatic") {
		t.Fatalf("language settings = %d, %s", settings.Code, settings.Body.String())
	}
	saved := request(handler, http.MethodPost, "/settings/language", url.Values{
		"csrf": {session.CSRFSecret}, "locale": {"es"},
	}, cookies, nil)
	if saved.Code != http.StatusSeeOther || saved.Header().Get("Location") != "/settings/language?saved=1" {
		t.Fatalf("save language = %d, %q", saved.Code, saved.Header().Get("Location"))
	}
	if locale, err := auth.GetUserLocale(context.Background(), owner.UserID); err != nil || locale != "es" {
		t.Fatalf("saved user locale = %q, %v", locale, err)
	}
	reset := request(handler, http.MethodPost, "/settings/language", url.Values{
		"csrf": {session.CSRFSecret}, "locale": {""},
	}, cookies, nil)
	if reset.Code != http.StatusSeeOther || responseCookie(reset, "locale").MaxAge >= 0 {
		t.Fatalf("reset language = %d, cookie %+v", reset.Code, responseCookie(reset, "locale"))
	}
	if locale, err := auth.GetUserLocale(context.Background(), owner.UserID); err != nil || locale != "" {
		t.Fatalf("reset user locale = %q, %v", locale, err)
	}

	general := request(handler, http.MethodPost, "/workspaces/"+workspaceID+"/settings/general", url.Values{
		"csrf": {session.CSRFSecret}, "locale": {"es"},
	}, cookies, nil)
	if general.Code != http.StatusSeeOther || general.Header().Get("Location") != "/workspaces/"+workspaceID+"/settings/general?saved=1" {
		t.Fatalf("workspace language = %d, %q", general.Code, general.Header().Get("Location"))
	}
	if locale, err := auth.GetWorkspaceLocale(context.Background(), workspaceID); err != nil || locale != "es" {
		t.Fatalf("saved workspace locale = %q, %v", locale, err)
	}
}

func TestBrowserPagesRenderSpanishDocuments(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	ownerPrincipal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdateWorkspaceLocale(context.Background(), ownerPrincipal, "es", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}, {Name: "locale", Value: "es"}}
	paths := []string{
		"/", "/workspaces/" + workspaceID, "/workspaces/" + workspaceID + "/items",
		"/workspaces/" + workspaceID + "/settings/general", "/workspaces/" + workspaceID + "/settings/members",
		"/workspaces/" + workspaceID + "/settings/invitations", "/workspaces/" + workspaceID + "/settings/tokens",
		"/workspaces/" + workspaceID + "/settings/export", "/settings/language", "/sessions", "/security",
	}
	for _, path := range paths {
		response := requestFrom(handler, http.MethodGet, path, nil, cookies, map[string]string{"Accept-Language": "en"}, "example.com", "192.0.2.1:1234")
		body := response.Body.String()
		if response.Code != http.StatusOK || response.Header().Get("Content-Language") != "es" || !strings.Contains(body, `<html lang="es">`) || strings.Contains(body, "common.") {
			t.Errorf("%s = %d language=%q body=%s", path, response.Code, response.Header().Get("Content-Language"), body)
		}
	}
	invitation, err := auth.CreateInvitation(context.Background(), ownerPrincipal, "recipient@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	invitationResponse := requestFrom(handler, http.MethodGet, "/invitations/"+invitation.Secret, nil, nil, map[string]string{"Accept-Language": "en"}, "example.com", "192.0.2.1:1234")
	if invitationResponse.Header().Get("Content-Language") != "es" || !strings.Contains(invitationResponse.Body.String(), `<html lang="es">`) {
		t.Fatalf("invitation locale = %q, %s", invitationResponse.Header().Get("Content-Language"), invitationResponse.Body.String())
	}
}

func TestSSEScopeReconnectHeartbeatAndLimits(t *testing.T) {
	handler, auth, itemService, workspaceID, owner := testWeb(t, 1)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	streamRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/workspaces/"+workspaceID+"/items/events", nil)
	addCookies(streamRequest, cookies)
	response, err := server.Client().Do(streamRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	initial := readUntil(t, reader, "event: resync")
	if !strings.Contains(initial, "id: 0") {
		t.Fatalf("initial resync = %q", initial)
	}
	secondRequest, _ := http.NewRequest(http.MethodGet, server.URL+"/workspaces/"+workspaceID+"/items/events", nil)
	addCookies(secondRequest, cookies)
	second, err := server.Client().Do(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests || second.Header.Get("Retry-After") == "" {
		t.Fatalf("second stream = %d", second.StatusCode)
	}
	if heartbeat := readUntil(t, reader, ": heartbeat"); !strings.Contains(heartbeat, ": heartbeat") {
		t.Fatalf("heartbeat = %q", heartbeat)
	}
	principal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	principal.AuthMethod = "test"
	created, err := itemService.Create(context.Background(), principal, workspaceID, "Secret title", "sse-create-key", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	event := readUntil(t, reader, "event: item.changed") + readUntil(t, reader, "\n\n")
	if !strings.Contains(event, created.Item.ID) || strings.Contains(event, "Secret title") || !strings.Contains(event, `"schemaVersion":1`) {
		t.Fatalf("change event = %q", event)
	}
	eventID := sseID(event)
	if eventID == "" {
		t.Fatal("change event ID missing")
	}
	cancel()
	response.Body.Close()
	time.Sleep(30 * time.Millisecond)
	complete := items.Complete
	if _, err := itemService.Update(
		context.Background(), principal, workspaceID, created.Item.ID, 1,
		items.UpdateInput{Status: &complete}, identity.AuditContext{},
	); err != nil {
		t.Fatal(err)
	}
	reconnectCtx, reconnectCancel := context.WithTimeout(context.Background(), time.Second)
	defer reconnectCancel()
	reconnect, _ := http.NewRequestWithContext(
		reconnectCtx, http.MethodGet,
		server.URL+"/workspaces/"+workspaceID+"/items/events", nil,
	)
	reconnect.Header.Set("Last-Event-ID", eventID)
	addCookies(reconnect, cookies)
	reconnected, err := server.Client().Do(reconnect)
	if err != nil {
		t.Fatal(err)
	}
	reconnectReader := bufio.NewReader(reconnected.Body)
	replayed := readUntil(t, reconnectReader, "event: item.changed") + readUntil(t, reconnectReader, "\n\n")
	if !strings.Contains(replayed, `"action":"updated"`) {
		t.Fatalf("reconnect replay = %q", replayed)
	}
	reconnected.Body.Close()
	reconnectCancel()
	time.Sleep(30 * time.Millisecond)

	wrong := request(handler, http.MethodGet, "/workspaces/00000000000000000000000000000000/items/events", nil, cookies, nil)
	if wrong.Code != http.StatusForbidden {
		t.Fatalf("wrong-workspace stream = %d", wrong.Code)
	}
}

func TestNotificationSSEIsScopedAndRedactedToRecipient(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 1)
	_, err := auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "Existing", Body: "Existing body",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	streamRequest, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/notifications/events", nil)
	addCookies(streamRequest, []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}})
	response, err := server.Client().Do(streamRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("notification stream = %d", response.StatusCode)
	}
	reader := bufio.NewReader(response.Body)
	newNotification, err := auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "Visible", Body: "Private body",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	event := readUntil(t, reader, "event: notification.created") + readUntil(t, reader, "\n\n")
	if strings.Contains(event, "Existing body") || !strings.Contains(event, "Private body") || !strings.Contains(event, "Visible") ||
		strings.Contains(event, "UserID") || strings.Contains(event, "WorkspaceID") || strings.Contains(event, "ReadAt") ||
		!strings.Contains(event, `"type":"workspace"`) || !strings.Contains(event, `"createdAt":"`) {
		t.Fatalf("notification event = %q", event)
	}
	cancel()
	response.Body.Close()
	time.Sleep(30 * time.Millisecond)
	resyncCtx, resyncCancel := context.WithTimeout(context.Background(), time.Second)
	defer resyncCancel()
	resyncRequest, _ := http.NewRequestWithContext(resyncCtx, http.MethodGet, server.URL+"/notifications/events", nil)
	resyncRequest.Header.Set("Last-Event-ID", "missing")
	addCookies(resyncRequest, []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}})
	resyncResponse, err := server.Client().Do(resyncRequest)
	if err != nil {
		t.Fatal(err)
	}
	resyncReader := bufio.NewReader(resyncResponse.Body)
	resync := readUntil(t, resyncReader, "\n\n")
	resyncResponse.Body.Close()
	if !strings.Contains(resync, "id: "+newNotification.ID) || !strings.Contains(resync, "event: resync") {
		t.Fatalf("notification resync = %q", resync)
	}
	wrong := request(handler, http.MethodGet, "/notifications", nil, []*http.Cookie{{Name: "session", Value: "missing"}}, nil)
	if wrong.Code != http.StatusSeeOther {
		t.Fatalf("unauthenticated notification page = %d", wrong.Code)
	}
}

func sseID(event string) string {
	for _, line := range strings.Split(event, "\n") {
		if strings.HasPrefix(line, "id: ") {
			return strings.TrimPrefix(line, "id: ")
		}
	}
	return ""
}

func TestCriticalPagesAccessibilityContract(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	for _, target := range []string{"/", "/workspaces/" + workspaceID, "/workspaces/" + workspaceID + "/items"} {
		response := request(handler, http.MethodGet, target, nil, cookies, nil)
		assertAccessiblePage(t, response.Body.String(), "")
	}
	login := request(handler, http.MethodGet, "/login", nil, nil, nil)
	assertAccessiblePage(t, login.Body.String(), "Email")
}

func TestTokensPageLabelsWorkspaceUpdateScope(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	principal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateAPIToken(context.Background(), principal, "workspace update", []identity.Permission{
		identity.WorkspaceUpdate,
	}, nil, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	response := request(handler, http.MethodGet, "/workspaces/"+workspaceID+"/settings/tokens", nil, []*http.Cookie{
		{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret},
	}, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Workspace update") {
		t.Fatalf("workspace:update token label = %d, %s", response.Code, response.Body.String())
	}
}

func TestWebMCPBrowserSurfaceContract(t *testing.T) {
	script, err := files.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(script)
	for _, tool := range []string{
		"workspace-create-v1", "item-create-v1", "item-update-v1", "item-delete-v1",
		"member-role-update-v1", "member-remove-v1", "ownership-transfer-v1",
		"invitation-revoke-v1", "token-revoke-v1", "session-revoke-v1", "workspace-export-v1",
	} {
		if !strings.Contains(source, `"`+tool+`"`) {
			t.Errorf("WebMCP tool %q is not registered", tool)
		}
	}
	if !strings.Contains(source, "document.modelContext") || !strings.Contains(source, "ready-for-user-submission") || strings.Contains(source, "requestSubmit") {
		t.Fatal("WebMCP must feature-detect and prepare without automatic form submission")
	}
	for _, forbidden := range []string{`name: "csrf"`, `name: "password"`, `name: "secret"`, `name: "oidc"`} {
		if strings.Contains(source, forbidden) {
			t.Errorf("WebMCP schema exposes forbidden field %q", forbidden)
		}
	}
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	pages := map[string]string{
		"/": "home", "/workspaces/" + workspaceID + "/items": "items",
		"/workspaces/" + workspaceID + "/settings/members":     "members",
		"/workspaces/" + workspaceID + "/settings/invitations": "invitations",
		"/workspaces/" + workspaceID + "/settings/tokens":      "tokens",
		"/workspaces/" + workspaceID + "/settings/export":      "export", "/sessions": "sessions",
	}
	for target, page := range pages {
		body := request(handler, http.MethodGet, target, nil, cookies, nil).Body.String()
		if !strings.Contains(body, `data-webmcp-page="`+page+`"`) || !strings.Contains(body, `/assets/app.js`) {
			t.Errorf("%s lacks WebMCP page marker or script", target)
		}
	}
	for _, target := range []string{"/login", "/setup", "/security", "/invitations/invalid"} {
		body := request(handler, http.MethodGet, target, nil, nil, nil).Body.String()
		if strings.Contains(body, "data-webmcp-page") || strings.Contains(body, "/assets/app.js") {
			t.Errorf("secret-bearing page %s exposes WebMCP", target)
		}
	}
}

func TestWebNavigationAndActionFeedback(t *testing.T) {
	handler, auth, itemService, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	workspaceRoot := "/workspaces/" + workspaceID
	workspaceHome := request(handler, http.MethodGet, workspaceRoot, nil, cookies, nil)
	if workspaceHome.Code != http.StatusOK || !strings.Contains(workspaceHome.Body.String(), `data-workspace-home`) || !strings.Contains(workspaceHome.Body.String(), `class="active" aria-current="page" href="`+workspaceRoot+`">Home</a>`) || !strings.Contains(workspaceHome.Body.String(), `href="`+workspaceRoot+`/items"`) || !strings.Contains(workspaceHome.Body.String(), `href="`+workspaceRoot+`/settings"`) {
		t.Fatalf("workspace home = %d, %s", workspaceHome.Code, workspaceHome.Body.String())
	}
	for _, page := range []struct {
		path        string
		active      string
		sidebarName string
	}{
		{workspaceRoot + "/items", `class="active" aria-current="page" href="` + workspaceRoot + "/items" + `"`, "Workspace navigation"},
		{workspaceRoot + "/settings/members", `class="sidebar-subitem active" aria-current="page" href="` + workspaceRoot + "/settings/members" + `"`, "Workspace settings"},
		{workspaceRoot + "/settings/invitations", `class="sidebar-subitem active" aria-current="page" href="` + workspaceRoot + "/settings/members" + `"`, "Workspace settings"},
		{workspaceRoot + "/settings/tokens", `class="sidebar-subitem active" aria-current="page" href="` + workspaceRoot + "/settings/tokens" + `"`, "Workspace settings"},
		{workspaceRoot + "/settings/export", `class="sidebar-subitem active" aria-current="page" href="` + workspaceRoot + "/settings/export" + `"`, "Workspace settings"},
	} {
		body := request(handler, http.MethodGet, page.path, nil, cookies, nil).Body.String()
		if !strings.Contains(body, `<nav class="sidebar-nav" aria-label="`+page.sidebarName+`">`) ||
			!strings.Contains(body, page.active) ||
			!strings.Contains(body, `class="workspace-switcher"`) ||
			!strings.Contains(body, `class="account-menu"`) {
			t.Errorf("%s lacks primary navigation or current-page state", page.path)
		}
	}
	itemsBody := request(handler, http.MethodGet, workspaceRoot+"/items", nil, cookies, nil).Body.String()
	if strings.Contains(itemsBody, `Members &amp; invitations`) || strings.Contains(itemsBody, `API tokens`) || strings.Contains(itemsBody, `>Export<`) || !strings.Contains(itemsBody, `<a class="sidebar-back" href="/">← Workspaces</a>`) {
		t.Error("items context has incorrect navigation")
	}
	settingsRoot := request(handler, http.MethodGet, workspaceRoot+"/settings", nil, cookies, nil)
	if settingsRoot.Code != http.StatusSeeOther || settingsRoot.Header().Get("Location") != workspaceRoot+"/settings/members" {
		t.Fatalf("settings root = %d, %s", settingsRoot.Code, settingsRoot.Header().Get("Location"))
	}
	for _, page := range []string{workspaceRoot + "/settings/members", workspaceRoot + "/settings/invitations", workspaceRoot + "/settings/tokens", workspaceRoot + "/settings/export"} {
		body := request(handler, http.MethodGet, page, nil, cookies, nil).Body.String()
		if strings.Contains(body, `>Items<`) || strings.Contains(body, `href="`+workspaceRoot+`/settings">Settings`) || strings.Contains(body, `<nav class="sidebar-nav" aria-label="Workspace navigation">`) || !strings.Contains(body, `Members &amp; invitations`) || !strings.Contains(body, `API tokens`) || !strings.Contains(body, `>Export<`) || !strings.Contains(body, `href="`+workspaceRoot+`">← Back to home`) {
			t.Errorf("%s lacks expanded settings navigation", page)
		}
	}
	if body := request(handler, http.MethodGet, workspaceRoot+"/settings/members", nil, cookies, nil).Body.String(); !strings.Contains(body, `Members &amp; invitations`) || !strings.Contains(body, `<nav class="subnav" aria-label="Members and invitations">`) {
		t.Error("members page lacks combined settings entry or local tabs")
	}
	if body := request(handler, http.MethodGet, workspaceRoot+"/settings/invitations", nil, cookies, nil).Body.String(); !strings.Contains(body, `Members &amp; invitations`) || !strings.Contains(body, `class="active" aria-current="page" href="`+workspaceRoot+`/settings/invitations"`) {
		t.Error("invitations page lacks combined settings entry or current tab")
	}
	principal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := itemService.Create(context.Background(), principal, workspaceID, "Navigation item", "navigation-key", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	body := request(handler, http.MethodGet, workspaceRoot+"/items", nil, cookies, nil).Body.String()
	for _, required := range []string{
		`aria-label="Save Navigation item"`,
		`data-confirm="Permanently delete this item? This action cannot be undone."`,
		`role="status" aria-live="polite" id="realtime-status">Live updates connect when supported; refresh otherwise.`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("items page lacks %q", required)
		}
	}
	source, err := files.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `button[data-confirm]`) {
		t.Error("browser script lacks destructive-action confirmation hook")
	}
}

func TestWebBaselineManagementRoutes(t *testing.T) {
	handler, auth, _, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	for _, target := range []string{
		"/workspaces/" + workspaceID + "/settings/members",
		"/workspaces/" + workspaceID + "/settings/invitations",
		"/workspaces/" + workspaceID + "/settings/tokens",
		"/sessions", "/security", "/workspaces/" + workspaceID + "/settings/export",
	} {
		response := request(handler, http.MethodGet, target, nil, cookies, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("baseline route %s = %d, %s", target, response.Code, response.Body.String())
		}
	}
	exportPage := request(handler, http.MethodGet, "/workspaces/"+workspaceID+"/settings/export", nil, cookies, nil)
	if !strings.Contains(exportPage.Body.String(), "Download JSON") || !strings.Contains(exportPage.Body.String(), "/settings/export/download") {
		t.Fatal("browser export screen lacks download action")
	}
	exportDownload := request(handler, http.MethodGet, "/workspaces/"+workspaceID+"/settings/export/download", nil, cookies, nil)
	if exportDownload.Code != http.StatusOK || exportDownload.Header().Get("Content-Disposition") == "" || exportDownload.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("browser export download = %d, %s", exportDownload.Code, exportDownload.Header().Get("Content-Type"))
	}
	created := request(handler, http.MethodPost, "/workspaces", url.Values{
		"name": {"Created from browser"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/" {
		t.Fatalf("browser workspace creation = %d, %s", created.Code, created.Body.String())
	}
}

func TestWebSetupRequiresTokenAndBootstrapsFirstInstanceAdmin(t *testing.T) {
	handler, auth := testUnbootstrappedWeb(t, "setup-token")

	login := request(handler, http.MethodGet, "/login", nil, nil, nil)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/setup" {
		t.Fatalf("login redirect = %d, %q", login.Code, login.Header().Get("Location"))
	}
	page := request(handler, http.MethodGet, "/setup", nil, nil, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `label for="setup-token"`) {
		t.Fatalf("setup page = %d, %s", page.Code, page.Body.String())
	}
	assertAccessiblePage(t, page.Body.String(), "Setup token")
	csrf := responseCookie(page, "setup_csrf")
	wrong := request(handler, http.MethodPost, "/setup", url.Values{
		"setup_token": {"wrong-token"}, "email": {"owner@example.com"},
		"workspace": {"Example"}, "password": {"long-enough-password"},
		"password_confirmation": {"long-enough-password"}, "csrf": {csrf.Value},
	}, []*http.Cookie{csrf}, nil)
	if wrong.Code != http.StatusForbidden || strings.Contains(wrong.Body.String(), "wrong-token") {
		t.Fatalf("wrong setup token = %d, %s", wrong.Code, wrong.Body.String())
	}
	created := request(handler, http.MethodPost, "/setup", url.Values{
		"setup_token": {"setup-token"}, "email": {"Owner@Example.com"},
		"workspace": {"Example"}, "password": {"long-enough-password"},
		"password_confirmation": {"long-enough-password"}, "csrf": {csrf.Value},
	}, []*http.Cookie{csrf}, nil)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/login" {
		t.Fatalf("setup = %d, %s", created.Code, created.Body.String())
	}
	bootstrapped, err := auth.IsBootstrapped(context.Background())
	if err != nil || !bootstrapped {
		t.Fatalf("bootstrap state = %t, %v", bootstrapped, err)
	}
	userID, err := auth.AuthenticateLocal(context.Background(), "owner@example.com", "long-enough-password")
	if err != nil || !auth.IsInstanceAdmin(context.Background(), userID) {
		t.Fatalf("first instance admin = %q, %v", userID, err)
	}
	if disabled := request(handler, http.MethodGet, "/setup", nil, nil, nil); disabled.Code != http.StatusNotFound {
		t.Fatalf("disabled setup = %d", disabled.Code)
	}
}

func TestWebLocalSetupWorksWithoutToken(t *testing.T) {
	handler, auth := testUnbootstrappedWeb(t, "")

	login := localRequest(handler, http.MethodGet, "/login", nil, nil, nil)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/setup" {
		t.Fatalf("local login redirect = %d, %q", login.Code, login.Header().Get("Location"))
	}
	page := localRequest(handler, http.MethodGet, "/setup", nil, nil, nil)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), `name="setup_token"`) {
		t.Fatalf("local setup page = %d, %s", page.Code, page.Body.String())
	}
	assertAccessiblePage(t, page.Body.String(), "Email")
	csrf := responseCookie(page, "setup_csrf")
	created := localRequest(handler, http.MethodPost, "/setup", url.Values{
		"email": {"Owner@Example.com"}, "workspace": {"Example"},
		"password":              {"long-enough-password"},
		"password_confirmation": {"long-enough-password"}, "csrf": {csrf.Value},
	}, []*http.Cookie{csrf}, nil)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/login" {
		t.Fatalf("local setup = %d, %s", created.Code, created.Body.String())
	}
	userID, err := auth.AuthenticateLocal(context.Background(), "owner@example.com", "long-enough-password")
	if err != nil || !auth.IsInstanceAdmin(context.Background(), userID) {
		t.Fatalf("local first instance admin = %q, %v", userID, err)
	}
	if disabled := localRequest(handler, http.MethodGet, "/setup", nil, nil, nil); disabled.Code != http.StatusNotFound {
		t.Fatalf("local setup after bootstrap = %d", disabled.Code)
	}
}

func TestWebSetupRequiresConfigurationRemotely(t *testing.T) {
	handler, _ := testUnbootstrappedWeb(t, "")
	response := requestFrom(handler, http.MethodGet, "/setup", nil, nil, map[string]string{
		"X-Forwarded-For": "127.0.0.1",
	}, "localhost:8080", "192.0.2.1:1234")
	if response.Code != http.StatusServiceUnavailable ||
		!strings.Contains(response.Body.String(), "BOOTSTRAP_SETUP_TOKEN") {
		t.Fatalf("remote setup = %d, %s", response.Code, response.Body.String())
	}
}

func TestWebSetupRejectsCSRFAndPasswordMismatch(t *testing.T) {
	handler, _ := testUnbootstrappedWeb(t, "setup-token")
	page := request(handler, http.MethodGet, "/setup", nil, nil, nil)
	csrf := responseCookie(page, "setup_csrf")
	invalidCSRF := request(handler, http.MethodPost, "/setup", url.Values{
		"setup_token": {"setup-token"}, "csrf": {"wrong"},
	}, []*http.Cookie{csrf}, nil)
	if invalidCSRF.Code != http.StatusForbidden {
		t.Fatalf("invalid setup CSRF = %d", invalidCSRF.Code)
	}
	mismatch := request(handler, http.MethodPost, "/setup", url.Values{
		"setup_token": {"setup-token"}, "email": {"owner@example.com"},
		"workspace": {"Example"}, "password": {"long-enough-password"},
		"password_confirmation": {"different-password"}, "csrf": {csrf.Value},
	}, []*http.Cookie{csrf}, nil)
	if mismatch.Code != http.StatusUnprocessableEntity || !strings.Contains(mismatch.Body.String(), "do not match") {
		t.Fatalf("password mismatch = %d, %s", mismatch.Code, mismatch.Body.String())
	}
}

func testWeb(t *testing.T, maximumStreams int, mailers ...appmail.Sender) (http.Handler, *identity.Service, *items.Service, string, identity.BootstrapResult) {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	cfg.HTTP.SSEPollInterval = 10 * time.Millisecond
	cfg.HTTP.SSEHeartbeat = 20 * time.Millisecond
	cfg.HTTP.SSEMaxLifetime = time.Second
	cfg.HTTP.SSEMaxConnections = maximumStreams
	cfg.HTTP.SSEMaxPerUser = maximumStreams
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	var mailer appmail.Sender
	if len(mailers) > 0 {
		mailer = mailers[0]
	}
	webHandler, err := NewHandlerWithServices(auth, itemService,
		portability.NewService(db, cfg.Database.Driver, auth, cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes, cfg.Data.ImportMaxRecords, cfg.Data.ImportMaxBytes, itemService),
		nil, cfg.HTTP, "", "", mailer)
	if err != nil {
		t.Fatal(err)
	}
	return webHandler.Routes(), auth, itemService, owner.WorkspaceID, owner
}

func testUnbootstrappedWeb(t *testing.T, setupToken string) (http.Handler, *identity.Service) {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	db, err := database.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := database.Migrate(ctx, db, cfg.Database.Driver); err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(db, cfg.Database.Driver)
	if err != nil {
		t.Fatal(err)
	}
	itemService := items.NewService(db, cfg.Database.Driver, auth, cfg.Data.ItemsMaxPerWorkspace)
	webHandler, err := NewHandler(auth, itemService, cfg.HTTP, setupToken)
	if err != nil {
		t.Fatal(err)
	}
	return webHandler.Routes(), auth
}

func request(handler http.Handler, method, target string, form url.Values, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	return requestFrom(handler, method, target, form, cookies, headers, "example.com", "192.0.2.1:1234")
}

func localRequest(handler http.Handler, method, target string, form url.Values, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	return requestFrom(handler, method, target, form, cookies, headers, "localhost:8080", "127.0.0.1:1234")
}

func requestFrom(handler http.Handler, method, target string, form url.Values, cookies []*http.Cookie, headers map[string]string, host, remoteAddr string) *httptest.ResponseRecorder {
	var body io.Reader
	if form != nil {
		body = bytes.NewBufferString(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	req.Host = host
	req.RemoteAddr = remoteAddr
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	addCookies(req, cookies)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func addCookies(request *http.Request, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
}

func responseCookie(response *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return &http.Cookie{Name: name}
}

func readUntil(t *testing.T, reader *bufio.Reader, wanted string) string {
	t.Helper()
	var result strings.Builder
	for !strings.Contains(result.String(), wanted) {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream before %q: %v (%q)", wanted, err, result.String())
		}
		result.WriteString(line)
	}
	return result.String()
}

func assertAccessiblePage(t *testing.T, body string, label string) {
	t.Helper()
	for _, required := range []string{`<!doctype html>`, `<html lang="en">`, `<meta name="viewport"`, `<main`, `<h1`} {
		if !strings.Contains(body, required) {
			t.Errorf("page lacks %s", required)
		}
	}
	if label != "" && !strings.Contains(body, label) {
		t.Errorf("page lacks label %q", label)
	}
}
