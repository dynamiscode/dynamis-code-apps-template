package web

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
	"example.com/dynamis-code/apps-template/internal/portability"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRenderCallSitesUseTemplSwitchCases(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	switchCases := renderSwitchCases(t)
	fset := token.NewFileSet()
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 3 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "render" {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if !ok || receiver.Name != "h" {
				return true
			}
			literal, ok := call.Args[2].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Errorf("%s: h.render view name must be a string literal; dispatch dynamically by explicit literal calls", fset.Position(call.Pos()))
				return true
			}
			name, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Errorf("%s: invalid render view literal: %v", fset.Position(literal.Pos()), err)
				return true
			}
			if _, ok := switchCases[name]; !ok {
				t.Errorf("%s: h.render view %q is not handled by render switch", fset.Position(literal.Pos()), name)
			}
			return true
		})
	}
}

func renderSwitchCases(t *testing.T) map[string]struct{} {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "handler.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	cases := make(map[string]struct{})
	for _, declaration := range file.Decls {
		method, ok := declaration.(*ast.FuncDecl)
		if !ok || method.Name.Name != "render" || method.Recv == nil || len(method.Recv.List) != 1 {
			continue
		}
		receiver, ok := method.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		typeName, ok := receiver.X.(*ast.Ident)
		if !ok || typeName.Name != "Handler" {
			continue
		}
		ast.Inspect(method.Body, func(node ast.Node) bool {
			statement, ok := node.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			selector, ok := statement.Tag.(*ast.Ident)
			if !ok || selector.Name != "name" {
				return true
			}
			for _, clause := range statement.Body.List {
				caseClause := clause.(*ast.CaseClause)
				for _, expression := range caseClause.List {
					if literal, ok := expression.(*ast.BasicLit); ok && literal.Kind == token.STRING {
						name, err := strconv.Unquote(literal.Value)
						if err == nil {
							cases[name] = struct{}{}
						}
					}
				}
			}
			return false
		})
		return cases
	}
	t.Fatal("Handler.render method not found")
	return cases
}

func TestMFAOptionsRenderAsJSON(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	data := viewmodel.PageData{
		Locale: "en", Catalog: catalog,
		MFAOptions: json.RawMessage(`{"publicKey":{"challenge":"abc"},"label":"</script><script>alert(1)</script>"}`),
	}
	if err := authMFA(data).Render(context.Background(), &body); err != nil {
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
		Label string `json:"label"`
	}
	if err := json.Unmarshal([]byte(body.String()[start:start+end]), &options); err != nil {
		t.Fatalf("MFA options are not a JSON object: %v; body=%s", err, body.String())
	}
	if options.PublicKey.Challenge != "abc" {
		t.Fatalf("challenge = %q", options.PublicKey.Challenge)
	}
	if options.Label != `</script><script>alert(1)</script>` || strings.Contains(body.String(), `</script><script>alert(1)</script>`) {
		t.Fatal("MFA options did not remain safely encoded JSON")
	}
}

func TestEmailVerificationRouteSuccessUsesAuthLayout(t *testing.T) {
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
	t.Cleanup(func() { _ = db.Close() })
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
	if _, err := db.ExecContext(ctx, "UPDATE users SET email_verified_at = NULL WHERE id = ?", owner.UserID); err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandlerWithServices(auth, nil, nil, cfg.HTTP, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	unconfigured := request(handler.Routes(), http.MethodPost, "/account/verify-email", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if unconfigured.Code != http.StatusServiceUnavailable || !strings.Contains(unconfigured.Body.String(), "Email delivery is not configured.") || strings.Contains(unconfigured.Body.String(), "verify-email/") {
		t.Fatalf("unconfigured verification delivery = %d, %s", unconfigured.Code, unconfigured.Body.String())
	}
	failedMailerHandler, err := NewHandlerWithServices(auth, nil, nil, cfg.HTTP, "", "", failingMailer{})
	if err != nil {
		t.Fatal(err)
	}
	failedDelivery := request(failedMailerHandler.Routes(), http.MethodPost, "/account/verify-email", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if failedDelivery.Code != http.StatusServiceUnavailable || !strings.Contains(failedDelivery.Body.String(), "Verification email could not be delivered.") || strings.Contains(failedDelivery.Body.String(), "verify-email/") {
		t.Fatalf("failed verification delivery = %d, %s", failedDelivery.Code, failedDelivery.Body.String())
	}
	mailer := &recordingMailer{}
	mailHandler, err := NewHandlerWithServices(auth, nil, nil, cfg.HTTP, "", "", mailer)
	if err != nil {
		t.Fatal(err)
	}
	requested := request(mailHandler.Routes(), http.MethodPost, "/account/verify-email", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if requested.Code != http.StatusSeeOther || requested.Header().Get("Location") != "/account?saved=1" {
		t.Fatalf("verification email request = %d, %q", requested.Code, requested.Header().Get("Location"))
	}
	link := strings.TrimPrefix(mailer.body, "Verify your email with this link: ")
	parsedLink, err := url.Parse(link)
	if err != nil || parsedLink.Path == "" {
		t.Fatalf("verification email link invalid: %q", link)
	}
	response := request(mailHandler.Routes(), http.MethodGet, parsedLink.Path, nil, nil, map[string]string{"Accept-Language": "es"})
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, "Correo verificado.") ||
		!strings.Contains(body, `href="/account" class="btn btn-primary"`) {
		t.Fatalf("verification success route status = %d; localized success or account link missing", response.Code)
	}
	if strings.Contains(body, parsedLink.Path[strings.LastIndex(parsedLink.Path, "/")+1:]) || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("verification success page exposed its token or was cacheable")
	}
}

type failingMailer struct{}

func (failingMailer) Send(context.Context, string, string, string) error {
	return errors.New("mail delivery failed")
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

func TestMFAPolicyRedirectStartsBrowserChallenge(t *testing.T) {
	handler, auth, workspaceID, owner := testWebWithMFA(t, 10, true)
	ctx := context.Background()
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := auth.BeginTOTPEnrollment(ctx, owner.UserID, session.ID, "long-enough-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CompleteTOTPEnrollment(ctx, session.ID, enrollment.Challenge, webTOTPCode(enrollment.Secret, time.Now()), identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}

	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	response := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/members", nil, cookies, nil)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/mfa" {
		t.Fatalf("MFA redirect = %d, %s", response.Code, response.Body.String())
	}
	if challenge := responseCookie(response, "mfa_challenge"); challenge.Value == "" {
		t.Fatal("MFA challenge cookie missing")
	}
	if returnTo := responseCookie(response, "mfa_return_to"); returnTo.Value != "/workspaces/"+workspaceID.String()+"/settings/members" {
		t.Fatalf("MFA return path = %q", returnTo.Value)
	}
}

func webTOTPCode(secret string, now time.Time) string {
	decoded, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	counter := uint64(now.Unix() / 30)
	var message [8]byte
	for index := len(message) - 1; index >= 0; index-- {
		message[index] = byte(counter)
		counter >>= 8
	}
	mac := hmac.New(sha1.New, decoded)
	_, _ = mac.Write(message[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	value := (uint32(sum[offset])&127)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1000000)
}

func TestWebAccountNotificationsAndPasswordReset(t *testing.T) {
	mailer := &recordingMailer{}
	handler, auth, workspaceID, owner := testWeb(t, 10, mailer)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	account := request(handler, http.MethodGet, "/account", nil, cookies, nil)
	if account.Code != http.StatusOK || !strings.Contains(account.Body.String(), "Profile and preferences") {
		t.Fatalf("account page status = %d; missing profile heading", account.Code)
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
	invalidProfile := request(handler, http.MethodPost, "/account/profile", url.Values{
		"csrf": {session.CSRFSecret}, "display_name": {"Owner"}, "locale": {"es"},
		"timezone": {"Not/A_Timezone"}, "theme": {"dark"},
	}, cookies, nil)
	if invalidProfile.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidProfile.Body.String(), "La configuración de cuenta no es válida.") {
		t.Fatalf("invalid profile = %d, %s", invalidProfile.Code, invalidProfile.Body.String())
	}
	if profile, err := auth.GetUserProfile(context.Background(), owner.UserID); err != nil || profile.Timezone != "America/Bogota" {
		t.Fatalf("invalid profile changed saved timezone = %+v, %v", profile, err)
	}
	passwordMismatch := request(handler, http.MethodPost, "/account/password", url.Values{
		"csrf": {session.CSRFSecret}, "new_password": {"a-new-password-1234"}, "password_confirmation": {"different-password-1234"},
	}, cookies, nil)
	if passwordMismatch.Code != http.StatusUnprocessableEntity || !strings.Contains(passwordMismatch.Body.String(), "Las nuevas contraseñas no coinciden.") || !strings.Contains(passwordMismatch.Body.String(), `form action="/account/password"`) {
		t.Fatalf("account password mismatch status = %d; missing translated error or form", passwordMismatch.Code)
	}
	preference := request(handler, http.MethodPost, "/account/notifications", url.Values{
		"csrf": {session.CSRFSecret}, "notification_type": {"system"}, "enabled": {"false"},
	}, cookies, nil)
	if preference.Code != http.StatusSeeOther {
		t.Fatalf("notification preference status = %d", preference.Code)
	}
	preferences, err := auth.GetNotificationPreferences(context.Background(), owner.UserID, 0)
	if err != nil || len(preferences) != 1 || preferences[0].NotificationType != "system" || preferences[0].Enabled {
		t.Fatalf("saved notification preferences = %+v, %v", preferences, err)
	}

	notification, err := auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "Update", Body: "A workspace update",
	}, identity.AuditContext{})
	if err != nil || notification.ID == 0 {
		t.Fatalf("notification creation failed; id present = %t", notification.ID != 0)
	}
	notifications := request(handler, http.MethodGet, "/notifications", nil, cookies, nil)
	notificationsBody := notifications.Body.String()
	if notifications.Code != http.StatusOK || !strings.Contains(notificationsBody, "A workspace update") {
		t.Fatalf("notifications page status = %d; missing notification", notifications.Code)
	}
	for _, marker := range []string{`class="notification-center"`, `aria-live="polite"`, `<strong data-unread-count>1</strong>`, "No leídas", "Dynamis Code"} {
		if !strings.Contains(notificationsBody, marker) {
			t.Errorf("notifications page missing marker %q", marker)
		}
	}
	marked := request(handler, http.MethodPost, "/notifications/"+notification.ID.String(), url.Values{
		"csrf": {session.CSRFSecret},
	}, cookies, nil)
	if marked.Code != http.StatusSeeOther {
		t.Fatalf("notification mark read = %d", marked.Code)
	}
	readPage := request(handler, http.MethodGet, "/notifications", nil, cookies, nil)
	if readPage.Code != http.StatusOK || !strings.Contains(readPage.Body.String(), `<strong data-unread-count>0</strong>`) ||
		!strings.Contains(readPage.Body.String(), "leída") {
		t.Fatalf("read notification page status = %d; missing read state", readPage.Code)
	}

	resetPage := request(handler, http.MethodGet, "/password-reset", nil, nil, nil)
	if resetPage.Code != http.StatusOK || !strings.Contains(resetPage.Body.String(), `form action="/password-reset"`) ||
		!strings.Contains(resetPage.Body.String(), `class="brand auth-brand"`) || !strings.Contains(resetPage.Body.String(), `class="input w-full"`) {
		t.Fatalf("password reset page status = %d; missing form or auth layout marker", resetPage.Code)
	}
	expiredReset := request(handler, http.MethodPost, "/password-reset", url.Values{"email": {"owner@example.com"}}, nil, nil)
	if expiredReset.Code != http.StatusForbidden || !strings.Contains(expiredReset.Body.String(), `tabindex="-1"`) ||
		!strings.Contains(expiredReset.Body.String(), `form action="/password-reset"`) {
		t.Fatalf("expired reset request status = %d; missing focused alert or recovery form", expiredReset.Code)
	}
	resetCSRF := responseCookie(resetPage, "reset_csrf")
	resetRequested := request(handler, http.MethodPost, "/password-reset", url.Values{
		"csrf": {resetCSRF.Value}, "email": {"owner@example.com"},
	}, []*http.Cookie{resetCSRF, {Name: "locale", Value: "es"}}, nil)
	if resetRequested.Code != http.StatusOK || !strings.Contains(resetRequested.Body.String(), "Si existe una cuenta") {
		t.Fatalf("reset request status = %d; missing generic response", resetRequested.Code)
	}
	if mailer.subject != "Restablece tu contraseña de Dynamis Code" {
		t.Fatalf("reset subject = %q", mailer.subject)
	}
	resetLink := strings.TrimSpace(strings.TrimPrefix(mailer.body, "Restablece tu contraseña con este enlace: "))
	parsed, err := url.Parse(resetLink)
	if err != nil || parsed.Path == "" {
		t.Fatal("reset link invalid")
	}
	resetTokenPage := request(handler, http.MethodGet, parsed.Path, nil, nil, nil)
	resetTokenCSRF := responseCookie(resetTokenPage, "reset_csrf")
	resetMismatch := request(handler, http.MethodPost, parsed.Path, url.Values{
		"csrf": {resetTokenCSRF.Value}, "password": {"reset-owner-password"}, "password_confirmation": {"different-password"},
	}, []*http.Cookie{resetTokenCSRF, {Name: "locale", Value: "es"}}, nil)
	if resetMismatch.Code != http.StatusUnprocessableEntity || !strings.Contains(resetMismatch.Body.String(), "Las contraseñas no coinciden.") {
		t.Fatalf("reset mismatch status = %d; missing translated error", resetMismatch.Code)
	}
	for _, marker := range []string{`class="alert alert-error error"`, `tabindex="-1"`, `class="input w-full"`, `<html lang="es">`} {
		if !strings.Contains(resetMismatch.Body.String(), marker) {
			t.Fatalf("reset mismatch view missing marker %q", marker)
		}
	}
	if !strings.Contains(resetMismatch.Body.String(), `name="csrf" value="`+resetTokenCSRF.Value+`"`) {
		t.Fatal("reset mismatch response omitted retained CSRF field")
	}
	invalidResetPage := request(handler, http.MethodGet, "/password-reset/invalid-secret", nil, nil, nil)
	invalidResetCSRF := responseCookie(invalidResetPage, "reset_csrf")
	invalidReset := request(handler, http.MethodPost, "/password-reset/invalid-secret", url.Values{
		"csrf": {invalidResetCSRF.Value}, "password": {"reset-owner-password"}, "password_confirmation": {"reset-owner-password"},
	}, []*http.Cookie{invalidResetCSRF, {Name: "locale", Value: "es"}}, nil)
	if invalidReset.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidReset.Body.String(), "El enlace de restablecimiento no es válido o venció.") || !strings.Contains(invalidReset.Body.String(), `form action="/password-reset"`) ||
		!strings.Contains(invalidReset.Body.String(), `tabindex="-1"`) || responseCookie(invalidReset, "reset_csrf").Value == "" {
		t.Fatalf("invalid reset link status = %d; missing recovery form, focused alert, or fresh CSRF cookie", invalidReset.Code)
	}
	resetComplete := request(handler, http.MethodPost, parsed.Path, url.Values{
		"csrf": {resetTokenCSRF.Value}, "password": {"reset-owner-password"}, "password_confirmation": {"reset-owner-password"},
	}, []*http.Cookie{resetTokenCSRF}, nil)
	if resetComplete.Code != http.StatusOK || !strings.Contains(resetComplete.Body.String(), "Password reset") {
		t.Fatalf("reset completion status = %d; missing completion message", resetComplete.Code)
	}
	if _, err := auth.AuthenticateLocal(context.Background(), "owner@example.com", "reset-owner-password"); err != nil {
		t.Fatal(err)
	}
}

func TestAccountSecurityPagesUseSharedBrandAndRequireDeletionConfirmation(t *testing.T) {
	handler, auth, _, owner := testWeb(t, 10)
	if err := auth.UpdateUserProfile(context.Background(), owner.UserID, identity.ProfileUpdateInput{
		Locale: "es", Timezone: "America/Bogota", Theme: "dark",
	}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	for _, path := range []string{"/account", "/settings/language", "/sessions", "/security"} {
		response := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
		body := response.Body.String()
		if response.Code != http.StatusOK || !hasHTMLRootLang(body, "es") || !strings.Contains(body, `data-theme="dark"`) {
			t.Errorf("%s = %d with Spanish/dark preferences: %s", path, response.Code, body)
		}
		if !strings.Contains(body, "Dynamis Code") || !strings.Contains(body, `class="account-page`) {
			t.Errorf("%s does not use the branded account layout: %s", path, body)
		}
		if path == "/account" && (!strings.Contains(body, `type="checkbox" name="confirm_delete"`) || !strings.Contains(body, `value="yes"`) || !strings.Contains(body, "Entiendo que eliminar la cuenta es permanente")) {
			t.Errorf("account page lacks the Spanish no-JavaScript deletion confirmation: %s", body)
		}
		if path == "/account" && (!strings.Contains(body, `action="/account/password"`) || strings.Contains(body, `name="new_password" data-persist`)) {
			t.Errorf("account page lacks password form or persists password fields: %s", body)
		}
		if path == "/settings/language" && (!strings.Contains(body, `<div class="account-page" data-account-page>`) || !strings.Contains(body, `aria-label="Idioma"`)) {
			t.Errorf("language page lacks templ account layout or labelled language form: %s", body)
		}
	}

	account := request(handler, http.MethodPost, "/account/delete", url.Values{
		"csrf": {session.CSRFSecret}, "password": {"long-enough-password"},
	}, cookies, nil)
	if account.Code != http.StatusUnprocessableEntity || !strings.Contains(account.Body.String(), "Confirma la eliminación permanente de la cuenta para continuar.") {
		t.Fatalf("account deletion without confirmation = %d, %s", account.Code, account.Body.String())
	}
	if !strings.Contains(account.Body.String(), `form action="/account/password"`) {
		t.Fatalf("account error page lost its password settings form: %s", account.Body.String())
	}
	confirmed := request(handler, http.MethodPost, "/account/delete", url.Values{
		"csrf": {session.CSRFSecret}, "password": {"long-enough-password"}, "confirm_delete": {"yes"},
	}, cookies, nil)
	if confirmed.Code != http.StatusUnprocessableEntity || !strings.Contains(confirmed.Body.String(), "Transfiere la propiedad de los espacios de trabajo antes de eliminar esta cuenta.") {
		t.Fatalf("confirmed owner deletion = %d, %s", confirmed.Code, confirmed.Body.String())
	}
	if _, err := auth.AuthenticateLocal(context.Background(), "owner@example.com", "long-enough-password"); err != nil {
		t.Fatalf("account was removed without explicit confirmation: %v", err)
	}

	wrongPassword := request(handler, http.MethodPost, "/account/password", url.Values{
		"csrf": {session.CSRFSecret}, "current_password": {"wrong-password"},
		"new_password": {"changed-owner-password"}, "password_confirmation": {"changed-owner-password"},
	}, cookies, nil)
	if wrongPassword.Code != http.StatusUnprocessableEntity || !strings.Contains(wrongPassword.Body.String(), "Revisa tu contraseña actual") {
		t.Fatalf("password change with wrong current password = %d, %s", wrongPassword.Code, wrongPassword.Body.String())
	}
	changed := request(handler, http.MethodPost, "/account/password", url.Values{
		"csrf": {session.CSRFSecret}, "current_password": {"long-enough-password"},
		"new_password": {"changed-owner-password"}, "password_confirmation": {"changed-owner-password"},
	}, cookies, nil)
	if changed.Code != http.StatusSeeOther || changed.Header().Get("Location") != "/account?saved=1" {
		t.Fatalf("password change = %d, %q", changed.Code, changed.Header().Get("Location"))
	}
	if _, err := auth.AuthenticateLocal(context.Background(), "owner@example.com", "changed-owner-password"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err := auth.AuthenticateSession(context.Background(), session.Secret); !errors.Is(err, identity.ErrInvalidSession) {
		t.Fatalf("password change did not revoke the old session: %v", err)
	}

	principal, err := auth.Authorize(context.Background(), owner.UserID, owner.WorkspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(context.Background(), principal, "member@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	memberID, _, err := auth.CreateInvitedLocalUser(context.Background(), invitation.Secret, "member-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	memberSession, err := auth.CreateSession(context.Background(), memberID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	deleted := request(handler, http.MethodPost, "/account/delete", url.Values{
		"csrf": {memberSession.CSRFSecret}, "password": {"member-long-password"}, "confirm_delete": {"yes"},
	}, []*http.Cookie{{Name: "session", Value: memberSession.Secret}, {Name: "csrf", Value: memberSession.CSRFSecret}}, nil)
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != "/login" {
		t.Fatalf("confirmed member deletion = %d, %q", deleted.Code, deleted.Header().Get("Location"))
	}
	if _, err := auth.AuthenticateLocal(context.Background(), "member@example.com", "member-long-password"); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("deleted account still authenticates: %v", err)
	}
}

func TestSessionsListAndRevocation(t *testing.T) {
	handler, auth, _, owner := testWeb(t, 10)
	current, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: current.Secret}, {Name: "csrf", Value: current.CSRFSecret}}
	page := request(handler, http.MethodGet, "/sessions", nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "This session") || !strings.Contains(page.Body.String(), `<html lang="en">`) || !strings.Contains(page.Body.String(), "/assets/components.css") {
		t.Fatalf("session list = %d, %s", page.Code, page.Body.String())
	}
	if strings.Count(page.Body.String(), `action="/sessions/`) != 2 || !strings.Contains(page.Body.String(), `action="/sessions/`+current.ID+`"`) || strings.Contains(page.Body.String(), current.Secret) {
		t.Fatalf("session list action count or session secret exposure is wrong: %s", page.Body.String())
	}
	csrfFailure := request(handler, http.MethodPost, "/sessions/"+other.ID, url.Values{"csrf": {"invalid"}}, cookies, nil)
	if csrfFailure.Code != http.StatusForbidden {
		t.Fatalf("invalid revoke CSRF = %d", csrfFailure.Code)
	}
	revoked := request(handler, http.MethodPost, "/sessions/"+other.ID, url.Values{"csrf": {current.CSRFSecret}}, cookies, nil)
	if revoked.Code != http.StatusSeeOther || revoked.Header().Get("Location") != "/sessions?revoked=1" {
		t.Fatalf("revoke response = %d, %q", revoked.Code, revoked.Header().Get("Location"))
	}
	feedback := request(handler, http.MethodGet, revoked.Header().Get("Location"), nil, cookies, nil)
	if feedback.Code != http.StatusOK || !strings.Contains(feedback.Body.String(), "Session revoked.") || !strings.Contains(feedback.Body.String(), "Revoked") {
		t.Fatalf("revoke feedback = %d, %s", feedback.Code, feedback.Body.String())
	}
	ownerPrincipal, err := auth.Authorize(context.Background(), owner.UserID, owner.WorkspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(context.Background(), ownerPrincipal, "other@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	foreignUserID, _, err := auth.CreateInvitedLocalUser(context.Background(), invitation.Secret, "other-owner-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := auth.CreateSession(context.Background(), foreignUserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := request(handler, http.MethodPost, "/sessions/"+foreign.ID, url.Values{"csrf": {current.CSRFSecret}}, cookies, nil)
	if unauthorized.Code != http.StatusConflict {
		t.Fatalf("cross-user revoke = %d, %s", unauthorized.Code, unauthorized.Body.String())
	}
	if _, err := auth.AuthenticateSession(context.Background(), foreign.Secret); err != nil {
		t.Fatalf("cross-user session was revoked: %v", err)
	}
	currentRevoke := request(handler, http.MethodPost, "/sessions/"+current.ID, url.Values{"csrf": {current.CSRFSecret}}, cookies, nil)
	if currentRevoke.Code != http.StatusSeeOther || currentRevoke.Header().Get("Location") != "/sessions?revoked=1" {
		t.Fatalf("current session revoke response = %d, %q", currentRevoke.Code, currentRevoke.Header().Get("Location"))
	}
	if _, err := auth.AuthenticateSession(context.Background(), current.Secret); !errors.Is(err, identity.ErrInvalidSession) {
		t.Fatalf("current session remains valid after revocation: %v", err)
	}
	redirected := request(handler, http.MethodGet, currentRevoke.Header().Get("Location"), nil, cookies, nil)
	if redirected.Code != http.StatusSeeOther || redirected.Header().Get("Location") != "/login" {
		t.Fatalf("revoked session follow-up = %d, %q", redirected.Code, redirected.Header().Get("Location"))
	}
}

func TestSessionsPageExpiredAndSpanishEmptyStates(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Locale: "es", Theme: "dark", Catalog: catalog, Title: "Sessions"}
	data.Sessions = []identity.Session{{
		ID: "expired-session", AuthMethod: "local",
		CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour),
	}}
	var expired bytes.Buffer
	if err := sessionsPage(data, "", time.Now()).Render(context.Background(), &expired); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(expired.String(), "Vencida") || strings.Contains(expired.String(), `action="/sessions/expired-session"`) {
		t.Fatalf("expired session status or action is wrong: %s", expired.String())
	}
	data.Sessions[0].ExpiresAt = time.Now().Add(time.Hour)
	var active bytes.Buffer
	if err := sessionsPage(data, data.Sessions[0].ID, time.Now()).Render(context.Background(), &active); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(active.String(), "Activa") || !strings.Contains(active.String(), `action="/sessions/expired-session"`) {
		t.Fatalf("current active session lacks its revocation form: %s", active.String())
	}
	revokedAt := time.Now()
	data.Sessions[0].RevokedAt = &revokedAt
	var revoked bytes.Buffer
	if err := sessionsPage(data, "", time.Now()).Render(context.Background(), &revoked); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(revoked.String(), "Revocada") || strings.Contains(revoked.String(), `action="/sessions/expired-session"`) {
		t.Fatalf("revoked session status or action is wrong: %s", revoked.String())
	}
	data.Sessions = nil
	var empty bytes.Buffer
	if err := sessionsPage(data, "", time.Now()).Render(context.Background(), &empty); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty.String(), "No hay sesiones de navegador disponibles.") || !strings.Contains(empty.String(), `data-theme="dark"`) {
		t.Fatalf("Spanish empty session state = %s", empty.String())
	}
}

func TestSecurityPageUsesTemplAndRetainsCredentialBoundaries(t *testing.T) {
	handler, auth, _, owner := testWebWithMFA(t, 10, true)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	page := request(handler, http.MethodGet, "/security", nil, cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, `id="security-heading"`) || !strings.Contains(body, `href="/assets/components.css"`) || !strings.Contains(body, `action="/security/totp/start"`) {
		t.Fatalf("security page = %d, %s", page.Code, body)
	}
	if strings.Contains(body, session.Secret) {
		t.Fatalf("security page exposed template source or session secret: %s", body)
	}

	badPassword := request(handler, http.MethodPost, "/security/totp/start", url.Values{
		"csrf": {session.CSRFSecret}, "password": {"wrong-password"},
	}, cookies, nil)
	if badPassword.Code != http.StatusUnauthorized || !strings.Contains(badPassword.Body.String(), "Fresh authentication failed.") || !strings.Contains(badPassword.Body.String(), `id="security-heading"`) {
		t.Fatalf("reauthentication failure = %d, %s", badPassword.Code, badPassword.Body.String())
	}

	enrollment := request(handler, http.MethodPost, "/security/totp/start", url.Values{
		"csrf": {session.CSRFSecret}, "password": {"long-enough-password"},
	}, cookies, nil)
	if enrollment.Code != http.StatusOK || !strings.Contains(enrollment.Body.String(), `id="security-heading"`) || !strings.Contains(enrollment.Body.String(), "Scan or enter this one-time authenticator setup value") || strings.Contains(enrollment.Body.String(), session.Secret) {
		t.Fatalf("TOTP enrollment response = %d, %s", enrollment.Code, enrollment.Body.String())
	}
}

func PrincipalSystem() identity.Principal {
	return identity.Principal{AuthMethod: "system"}
}

func TestWebLocalePrecedenceAndWorkspaceFallback(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}

	// The picker (no workspace context) resolves the Accept-Language request;
	// a single membership redirects now, so this test seeds a second one.
	second := request(handler, http.MethodPost, "/workspaces", url.Values{"csrf": {session.CSRFSecret}, "name": {"Second"}}, cookies, nil)
	if second.Code != http.StatusSeeOther {
		t.Fatalf("seed second workspace = %d", second.Code)
	}
	spanish := requestFrom(handler, http.MethodGet, "/", nil, cookies, map[string]string{"Accept-Language": "es-MX,es;q=0.9"}, "example.com", "192.0.2.1:1234")
	if spanish.Header().Get("Content-Language") != "es" || !hasHTMLRootLang(spanish.Body.String(), "es") || !strings.Contains(spanish.Body.String(), "Espacios de trabajo") {
		t.Fatalf("Spanish home = %d, %s", spanish.Code, spanish.Body.String())
	}

	if err := auth.SetUserLocale(context.Background(), owner.UserID, "en", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	userPreference := requestFrom(handler, http.MethodGet, "/", nil, append(cookies, &http.Cookie{Name: "locale", Value: "es"}), map[string]string{"Accept-Language": "es"}, "example.com", "192.0.2.1:1234")
	if userPreference.Header().Get("Content-Language") != "en" || !hasHTMLRootLang(userPreference.Body.String(), "en") {
		t.Fatalf("user preference precedence = %d, %s", userPreference.Code, userPreference.Body.String())
	}
	if err := auth.SetUserLocale(context.Background(), owner.UserID, "", identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}

	workspace := requestFrom(handler, http.MethodGet, "/workspaces/"+workspaceID.String(), nil, cookies, map[string]string{"Accept-Language": "en"}, "example.com", "192.0.2.1:1234")
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
	workspace = requestFrom(handler, http.MethodGet, "/workspaces/"+workspaceID.String(), nil, cookies, map[string]string{"Accept-Language": "en"}, "example.com", "192.0.2.1:1234")
	if workspace.Header().Get("Content-Language") != "es" || !hasHTMLRootLang(workspace.Body.String(), "es") {
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
	handler, auth, workspaceID, owner := testWeb(t, 10, mailer)
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
	request(handler, http.MethodPost, "/workspaces/"+workspaceID.String()+"/settings/invitations", url.Values{
		"action": {"create"}, "csrf": {session.CSRFSecret}, "email": {"invite@example.com"}, "role": {"member"},
	}, []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}, nil)
	if mailer.subject != "Has sido invitado a Dynamis Code" || !strings.HasPrefix(mailer.body, "Abre este enlace de invitación:") {
		t.Fatalf("invitation email = subject %q body %q", mailer.subject, mailer.body)
	}
}

func TestWebLanguageSettingsRoutes(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
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
	if err := auth.UpdateUserProfile(context.Background(), owner.UserID, identity.ProfileUpdateInput{
		Locale: "es", Timezone: "America/Bogota", Theme: "dark",
	}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	invalidLocale := request(handler, http.MethodPost, "/settings/language", url.Values{
		"csrf": {session.CSRFSecret}, "locale": {"fr"},
	}, cookies, nil)
	if invalidLocale.Code != http.StatusUnprocessableEntity || !hasHTMLRootLang(invalidLocale.Body.String(), "es") ||
		!strings.Contains(invalidLocale.Body.String(), `data-theme="dark"`) ||
		!strings.Contains(invalidLocale.Body.String(), "El idioma seleccionado no es válido.") ||
		!strings.Contains(invalidLocale.Body.String(), `selected value="es"`) {
		t.Fatalf("invalid language form lost localized account context = %d, %s", invalidLocale.Code, invalidLocale.Body.String())
	}
	if locale, err := auth.GetUserLocale(context.Background(), owner.UserID); err != nil || locale != "es" {
		t.Fatalf("invalid language changed saved locale = %q, %v", locale, err)
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

	general := request(handler, http.MethodPost, "/workspaces/"+workspaceID.String()+"/settings/general", url.Values{
		"csrf": {session.CSRFSecret}, "locale": {"es"},
	}, cookies, nil)
	if general.Code != http.StatusSeeOther || general.Header().Get("Location") != "/workspaces/"+workspaceID.String()+"/settings/general?saved=1" {
		t.Fatalf("workspace language = %d, %q", general.Code, general.Header().Get("Location"))
	}
	if locale, err := auth.GetWorkspaceLocale(context.Background(), workspaceID); err != nil || locale != "es" {
		t.Fatalf("saved workspace locale = %q, %v", locale, err)
	}
}

func TestWebRedirectRejectsExternalTarget(t *testing.T) {
	for _, target := range []string{"https://example.com", "//example.com", "/\\example.com", "/%5cexample.com"} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		(&Handler{}).redirect(response, request, target)

		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
			t.Errorf("redirect %q = %d, location %q", target, response.Code, response.Header().Get("Location"))
		}
	}
}

func TestBrowserPagesRenderSpanishDocuments(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
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
		"/workspaces/" + workspaceID.String(),
		"/workspaces/" + workspaceID.String() + "/settings/general", "/workspaces/" + workspaceID.String() + "/settings/members",
		"/workspaces/" + workspaceID.String() + "/settings/invitations", "/workspaces/" + workspaceID.String() + "/settings/tokens",
		"/workspaces/" + workspaceID.String() + "/settings/provisioning", "/workspaces/" + workspaceID.String() + "/settings/export",
		"/workspaces/" + workspaceID.String() + "/settings/audit",
		"/settings/language", "/sessions", "/security",
	}
	for _, path := range paths {
		response := requestFrom(handler, http.MethodGet, path+"?all=1", nil, cookies, map[string]string{"Accept-Language": "en"}, "example.com", "192.0.2.1:1234")
		body := response.Body.String()
		if response.Code != http.StatusOK || response.Header().Get("Content-Language") != "es" || !hasHTMLRootLang(body, "es") || strings.Contains(body, "common.") {
			t.Errorf("%s = %d language=%q body=%s", path, response.Code, response.Header().Get("Content-Language"), body)
		}
	}
	invitation, err := auth.CreateInvitation(context.Background(), ownerPrincipal, "recipient@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	invitationResponse := requestFrom(handler, http.MethodGet, "/invitations/"+invitation.Secret, nil, nil, map[string]string{"Accept-Language": "en"}, "example.com", "192.0.2.1:1234")
	if invitationResponse.Header().Get("Content-Language") != "es" || !hasHTMLRootLang(invitationResponse.Body.String(), "es") {
		t.Fatalf("invitation locale = %q; Spanish root lang present=%v", invitationResponse.Header().Get("Content-Language"), hasHTMLRootLang(invitationResponse.Body.String(), "es"))
	}
}

func TestNotificationSSEIsScopedAndRedactedToRecipient(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 1)
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
	if !strings.Contains(resync, "id: "+newNotification.ID.String()) || !strings.Contains(resync, "event: resync") {
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
	handler, auth, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	for _, target := range []string{"/workspaces/" + workspaceID.String(), "/workspaces/" + workspaceID.String() + "/settings/audit"} {
		response := request(handler, http.MethodGet, target, nil, cookies, nil)
		assertAccessiblePage(t, response.Body.String(), "")
	}
	login := request(handler, http.MethodGet, "/login", nil, nil, nil)
	assertAccessiblePage(t, login.Body.String(), "Email")
}

func TestWebAuditHistoryIsScopedRedactedAndNotWebMCP(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
	ctx := context.Background()
	if err := auth.RecordAudit(ctx, identity.AuditEvent{
		EventType:     "test.audit.visible",
		ActorUserID:   owner.UserID,
		AuthMethod:    "session",
		WorkspaceID:   workspaceID,
		TargetType:    "workspace",
		TargetKey:     987654321,
		Action:        "audit.visible",
		Outcome:       "success",
		RequestID:     "request-secret",
		SourceAddress: "198.51.100.9",
		Metadata:      `{"invite_secret":"invite-secret","signed_url":"https://example.test/signed?token=signed-secret"}`,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	auditPath := "/workspaces/" + workspaceID.String() + "/settings/audit"
	shell := request(handler, http.MethodGet, auditPath, nil, cookies, nil)
	if shell.Code != http.StatusOK {
		t.Fatalf("lazy audit shell status = %d", shell.Code)
	}
	for _, marker := range []string{`data-lazy-src="` + auditPath + `?part=table"`, `href="` + auditPath + `?all=1"`} {
		if !strings.Contains(shell.Body.String(), marker) {
			t.Errorf("lazy audit shell missing marker %q", marker)
		}
	}
	if strings.Contains(shell.Body.String(), "test.audit.visible") {
		t.Error("lazy audit shell rendered history before fragment request")
	}
	fragment := request(handler, http.MethodGet, auditPath+"?part=table", nil, cookies, map[string]string{"HX-Request": "true"})
	if fragment.Code != http.StatusOK {
		t.Fatalf("audit fragment status = %d", fragment.Code)
	}
	for _, marker := range []string{
		`<div id="audit-table">`, "test.audit.visible", `tabindex="0"`, `role="region"`, `aria-label="Audit history"`,
	} {
		if !strings.Contains(fragment.Body.String(), marker) {
			t.Errorf("audit fragment missing marker %q", marker)
		}
	}
	if strings.Contains(fragment.Body.String(), "198.51.100.9") {
		t.Error("audit fragment exposed source address")
	}
	page := request(handler, http.MethodGet, auditPath+"?all=1", nil, cookies, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("audit page status/cache = %d, %q", page.Code, page.Header().Get("Cache-Control"))
	}
	for _, visible := range []string{"Audit history", "test.audit.visible", "audit.visible"} {
		if !strings.Contains(body, visible) {
			t.Errorf("audit page missing marker %q", visible)
		}
	}
	for _, secret := range []string{"request-secret", "invite-secret", "example.test", "signed-secret", "198.51.100.9"} {
		if strings.Contains(body, secret) {
			t.Errorf("audit page exposed forbidden marker %q", secret)
		}
	}
	if strings.Contains(body, "/assets/app.js") {
		t.Error("audit page exposes WebMCP actions")
	}
	assertAccessiblePage(t, body, "Audit history")
	for _, marker := range []string{`tabindex="0"`, `role="region"`, `aria-label="Audit history"`} {
		if !strings.Contains(body, marker) {
			t.Errorf("audit table missing accessibility marker %q", marker)
		}
	}
	expired := request(handler, http.MethodGet, auditPath+"?part=table", nil, []*http.Cookie{{Name: "session", Value: "expired"}}, map[string]string{"HX-Request": "true"})
	if expired.Code != http.StatusSeeOther || expired.Header().Get("Location") != "/login" {
		t.Fatalf("expired audit fragment = %d, location=%q", expired.Code, expired.Header().Get("Location"))
	}

	invitations, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, invitations, "viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "viewer-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerSession, err := auth.CreateSession(ctx, viewerID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	viewerResponse := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/audit", nil, []*http.Cookie{
		{Name: "session", Value: viewerSession.Secret}, {Name: "csrf", Value: viewerSession.CSRFSecret},
	}, nil)
	if viewerResponse.Code != http.StatusForbidden {
		t.Fatalf("viewer audit page = %d, want %d", viewerResponse.Code, http.StatusForbidden)
	}
	viewerSettings := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/members", nil, []*http.Cookie{
		{Name: "session", Value: viewerSession.Secret}, {Name: "csrf", Value: viewerSession.CSRFSecret},
	}, nil)
	viewerAuditLink := strings.Contains(viewerSettings.Body.String(), "/settings/audit")
	if viewerSettings.Code != http.StatusOK || viewerAuditLink {
		t.Fatalf("viewer settings navigation: status=%d audit_link=%t", viewerSettings.Code, viewerAuditLink)
	}

	otherWorkspace, err := auth.CreateWorkspace(ctx, identity.Principal{
		UserID: viewerID, AuthMethod: "session",
	}, identity.WorkspaceCreateInput{Name: "Viewer workspace"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	crossWorkspace := request(handler, http.MethodGet, "/workspaces/"+otherWorkspace.String()+"/settings/audit", nil, cookies, nil)
	if crossWorkspace.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace audit page = %d, want %d", crossWorkspace.Code, http.StatusNotFound)
	}
}

func TestAuditHistoryUsesProfileLocaleAndThemes(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/settings/audit?all=1"
	for _, theme := range []string{"light", "dark", "system"} {
		if err := auth.UpdateUserProfile(context.Background(), owner.UserID, identity.ProfileUpdateInput{
			Locale: "es", Timezone: "America/Bogota", Theme: theme,
		}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
		response := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
		body := response.Body.String()
		spanish := hasHTMLRootLang(body, "es")
		localized := response.Header().Get("Content-Language") == "es"
		themed := strings.Contains(body, `data-theme="`+theme+`"`)
		if theme == "system" {
			themed = !strings.Contains(body, `data-theme=`)
		}
		if response.Code != http.StatusOK || !spanish || !localized || !themed {
			t.Errorf("audit document theme=%q: status=%d spanish=%t localized=%t themed=%t",
				theme, response.Code, spanish, localized, themed)
		}
	}
}

func TestAuditTableRendersEmptyStateInBothLocales(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en", "es"} {
		data := viewmodel.PageData{Locale: locale, Catalog: catalog}
		var output strings.Builder
		if err := auditTable(data).Render(context.Background(), &output); err != nil {
			t.Fatalf("render empty audit table for locale %q: %v", locale, err)
		}
		html := output.String()
		container := strings.Contains(html, `<div id="audit-table">`)
		emptyState := strings.Contains(html, data.T("audit.none"))
		if !container || !emptyState {
			t.Errorf("empty audit table locale=%q: container=%t empty_state=%t", locale, container, emptyState)
		}
	}
}

func TestTokensPageLabelsWorkspaceUpdateScope(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
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
	response := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/tokens", nil, []*http.Cookie{
		{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret},
	}, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Workspace update") || !strings.Contains(response.Body.String(), "Create limited credentials for integrations") {
		t.Fatalf("workspace:update token label = %d, %s", response.Code, response.Body.String())
	}
}

func TestWebSCIMProvisioningSettingsLifecycle(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspaceID.String() + "/settings/provisioning"

	page := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(page.Body.String(), "http://example.com/scim/v2/"+workspaceID.String()) ||
		strings.Contains(page.Body.String(), "/assets/app.js") || strings.Contains(page.Body.String(), "data-webmcp-page") {
		t.Fatalf("SCIM provisioning page = %d, headers=%v, body=%s", page.Code, page.Header(), page.Body.String())
	}

	withoutCSRF := request(handler, http.MethodPost, path, url.Values{"action": {"create"}}, cookies, nil)
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("SCIM missing CSRF = %d", withoutCSRF.Code)
	}
	invalidAction := request(handler, http.MethodPost, path, url.Values{
		"action": {"invalid"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if invalidAction.Code != http.StatusUnprocessableEntity || invalidAction.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(invalidAction.Body.String(), "The requested SCIM action is invalid.") {
		t.Fatalf("SCIM invalid-action state = %d, cache-control=%q, alert present=%v", invalidAction.Code, invalidAction.Header().Get("Cache-Control"), strings.Contains(invalidAction.Body.String(), `role="alert"`))
	}
	created := request(handler, http.MethodPost, path, url.Values{
		"action": {"create"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	secret := extractSCIMSecret(t, created.Body.String())
	if created.Code != http.StatusOK || created.Header().Get("Cache-Control") != "no-store" ||
		strings.Count(created.Body.String(), secret) != 1 || !strings.Contains(created.Body.String(), "Copy this credential now") || !strings.Contains(created.Body.String(), "Create a replacement to rotate") {
		t.Fatalf("SCIM create = %d, cache-control=%q, one-time secret markup valid=%v", created.Code, created.Header().Get("Cache-Control"), strings.Contains(created.Body.String(), "Copy this credential now"))
	}
	if _, err := auth.AuthenticateSCIMToken(context.Background(), secret, workspaceID); err != nil {
		t.Fatalf("created SCIM credential = %v", err)
	}
	scimPrincipal, err := auth.AuthenticateSCIMToken(context.Background(), secret, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	scimUser, err := auth.CreateSCIMUser(context.Background(), scimPrincipal, identity.SCIMUserInput{
		UserName: "provisioned@example.com",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	adminSession, err := auth.CreateSession(context.Background(), scimUser.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	adminCookies := []*http.Cookie{{Name: "session", Value: adminSession.Secret}, {Name: "csrf", Value: adminSession.CSRFSecret}}
	if adminPage := request(handler, http.MethodGet, path+"?all=1", nil, adminCookies, nil); adminPage.Code != http.StatusForbidden {
		t.Fatalf("SCIM member page = %d", adminPage.Code)
	}
	ownerPrincipal, err := auth.Authorize(context.Background(), owner.UserID, workspaceID, identity.MembersManage)
	if err != nil {
		t.Fatal(err)
	}
	members, err := auth.ListMembers(context.Background(), ownerPrincipal)
	if err != nil {
		t.Fatal(err)
	}
	var memberID identity.MemberID
	for _, member := range members {
		if member.UserID == scimUser.UserID {
			memberID = member.ID
		}
	}
	if memberID == 0 {
		t.Fatal("scim user is not a workspace member")
	}
	if err := auth.ChangeMemberRole(context.Background(), ownerPrincipal, memberID, identity.Admin, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	if adminPage := request(handler, http.MethodGet, path+"?all=1", nil, adminCookies, nil); adminPage.Code != http.StatusOK {
		t.Fatalf("SCIM admin page = %d", adminPage.Code)
	}
	adminCreated := request(handler, http.MethodPost, path, url.Values{
		"action": {"create"}, "csrf": {adminSession.CSRFSecret},
	}, adminCookies, nil)
	adminSecret := extractSCIMSecret(t, adminCreated.Body.String())
	if adminCreated.Code != http.StatusOK || strings.Count(adminCreated.Body.String(), adminSecret) != 1 {
		t.Fatalf("SCIM admin create = %d, one-time secret rendered once=%v", adminCreated.Code, strings.Count(adminCreated.Body.String(), adminSecret) == 1)
	}
	if _, err := auth.AuthenticateSCIMToken(context.Background(), secret, workspaceID); !errors.Is(err, identity.ErrInvalidToken) {
		t.Fatalf("owner SCIM credential after admin rotation = %v", err)
	}
	if _, err := auth.AuthenticateSCIMToken(context.Background(), adminSecret, workspaceID); err != nil {
		t.Fatalf("admin-created SCIM credential = %v", err)
	}
	adminRevoked := request(handler, http.MethodPost, path, url.Values{
		"action": {"revoke"}, "csrf": {adminSession.CSRFSecret},
	}, adminCookies, nil)
	if adminRevoked.Code != http.StatusSeeOther || adminRevoked.Header().Get("Location") != path {
		t.Fatalf("SCIM admin revoke = %d, %q", adminRevoked.Code, adminRevoked.Header().Get("Location"))
	}
	if _, err := auth.AuthenticateSCIMToken(context.Background(), adminSecret, workspaceID); !errors.Is(err, identity.ErrInvalidToken) {
		t.Fatalf("admin-revoked SCIM credential = %v", err)
	}

	page = request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
	if strings.Contains(page.Body.String(), secret) || strings.Contains(page.Body.String(), adminSecret) || strings.Contains(page.Body.String(), "SCIMTokenSecret") {
		t.Fatal("SCIM GET exposed a one-time secret")
	}
	rotated := request(handler, http.MethodPost, path, url.Values{
		"action": {"create"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	rotatedSecret := extractSCIMSecret(t, rotated.Body.String())
	if rotated.Code != http.StatusOK || rotatedSecret == secret || strings.Count(rotated.Body.String(), rotatedSecret) != 1 || strings.Contains(rotated.Body.String(), secret) {
		t.Fatalf("SCIM rotation = %d, one-time secret rendered once=%v, old secret absent=%v", rotated.Code, strings.Count(rotated.Body.String(), rotatedSecret) == 1, !strings.Contains(rotated.Body.String(), secret))
	}
	if _, err := auth.AuthenticateSCIMToken(context.Background(), secret, workspaceID); !errors.Is(err, identity.ErrInvalidToken) {
		t.Fatalf("old SCIM credential after rotation = %v", err)
	}
	if _, err := auth.AuthenticateSCIMToken(context.Background(), rotatedSecret, workspaceID); err != nil {
		t.Fatalf("rotated SCIM credential = %v", err)
	}

	revokeWithoutCSRF := request(handler, http.MethodPost, path, url.Values{"action": {"revoke"}}, cookies, nil)
	if revokeWithoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("SCIM revoke missing CSRF = %d", revokeWithoutCSRF.Code)
	}
	revoked := request(handler, http.MethodPost, path, url.Values{
		"action": {"revoke"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if revoked.Code != http.StatusSeeOther || revoked.Header().Get("Location") != path {
		t.Fatalf("SCIM revoke = %d, %q", revoked.Code, revoked.Header().Get("Location"))
	}
	if _, err := auth.AuthenticateSCIMToken(context.Background(), rotatedSecret, workspaceID); !errors.Is(err, identity.ErrInvalidToken) {
		t.Fatalf("revoked SCIM credential = %v", err)
	}

	wrongWorkspace := request(handler, http.MethodGet, "/workspaces/00000000000000000000000000000000/settings/provisioning", nil, cookies, nil)
	if wrongWorkspace.Code != http.StatusNotFound || strings.Contains(wrongWorkspace.Body.String(), secret) || strings.Contains(wrongWorkspace.Body.String(), rotatedSecret) {
		t.Fatalf("cross-workspace SCIM page = %d, one-time secret absent=%v", wrongWorkspace.Code, !strings.Contains(wrongWorkspace.Body.String(), secret) && !strings.Contains(wrongWorkspace.Body.String(), rotatedSecret))
	}
}

func TestSCIMEndpointUsesConfiguredPublicURL(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://browser.example/workspaces/workspace/settings/provisioning", nil)
	if got := scimEndpoint("https://app.example/base/", false, request, identity.WorkspaceID(42)); got != "https://app.example/base/scim/v2/"+identity.WorkspaceID(42).String() {
		t.Fatalf("SCIM endpoint = %q", got)
	}
}

func extractSCIMSecret(t *testing.T, body string) string {
	t.Helper()
	warning := "Copy this credential now. It will not be shown again."
	warningStart := strings.Index(body, warning)
	if warningStart < 0 {
		t.Fatal("SCIM secret warning missing")
	}
	start := strings.Index(body[warningStart:], "<code")
	if start < 0 {
		t.Fatal("SCIM secret code element missing")
	}
	start += warningStart
	contentStart := strings.Index(body[start:], ">")
	if contentStart < 0 {
		t.Fatal("SCIM secret element is malformed")
	}
	start += contentStart + 1
	end := strings.Index(body[start:], "</code>")
	if end < 0 {
		t.Fatal("SCIM secret element is not closed")
	}
	secret := body[start : start+end]
	if secret == "" {
		t.Fatal("SCIM secret is empty")
	}
	return secret
}

func TestWebMCPBrowserSurfaceContract(t *testing.T) {
	script, err := files.ReadFile("assets/alpine-components.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(script)
	// The WebMCP registration is one function in the shared component file;
	// the no-auto-submit and schema rules scope to that block only.
	webmcp := source
	if start := strings.Index(webmcp, "const wireWebMCP"); start >= 0 {
		end := strings.Index(webmcp[start:], "const pageBehaviors")
		if end < 0 {
			end = len(webmcp) - start
		}
		webmcp = webmcp[start : start+end]
	}
	for _, tool := range []string{
		"workspace-create-v1",
		"member-role-update-v1", "member-remove-v1", "ownership-transfer-v1",
		"invitation-revoke-v1", "token-revoke-v1", "session-revoke-v1", "workspace-export-v1",
	} {
		if !strings.Contains(webmcp, `"`+tool+`"`) {
			t.Errorf("WebMCP tool %q is not registered", tool)
		}
	}
	for _, tool := range []string{"item-create-v1", "item-update-v1", "item-delete-v1"} {
		if strings.Contains(webmcp, `"`+tool+`"`) {
			t.Errorf("removed WebMCP tool %q remains", tool)
		}
	}
	if !strings.Contains(webmcp, "document.modelContext") || !strings.Contains(webmcp, "ready-for-user-submission") || strings.Contains(webmcp, "requestSubmit") {
		t.Fatal("WebMCP must feature-detect and prepare without automatic form submission")
	}
	for _, forbidden := range []string{`name: "csrf"`, `name: "password"`, `name: "secret"`, `name: "oidc"`} {
		if strings.Contains(webmcp, forbidden) {
			t.Errorf("WebMCP schema exposes forbidden field %q", forbidden)
		}
	}
	handler, auth, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	pages := map[string]string{
		"/workspaces/" + workspaceID.String():                           "workspace",
		"/workspaces/" + workspaceID.String() + "/settings/members":     "members",
		"/workspaces/" + workspaceID.String() + "/settings/invitations": "invitations",
		"/workspaces/" + workspaceID.String() + "/settings/tokens":      "tokens",
		"/workspaces/" + workspaceID.String() + "/settings/export":      "export", "/sessions": "sessions",
	}
	for target, page := range pages {
		body := request(handler, http.MethodGet, target, nil, cookies, nil).Body.String()
		if !strings.Contains(body, `data-webmcp-page="`+page+`"`) || !strings.Contains(body, `/assets/alpine-components.js`) {
			t.Errorf("%s lacks WebMCP page marker or script", target)
		}
	}
	for _, target := range []string{"/login", "/setup", "/security", "/invitations/invalid"} {
		body := request(handler, http.MethodGet, target, nil, nil, nil).Body.String()
		if strings.Contains(body, "data-webmcp-page") || strings.Contains(body, "/assets/app.js") {
			t.Errorf("secret-bearing page %s exposes WebMCP", target)
		}
	}
	provisioning := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/provisioning", nil, cookies, nil)
	if strings.Contains(provisioning.Body.String(), "data-webmcp-page") || strings.Contains(provisioning.Body.String(), "/assets/app.js") {
		t.Fatal("SCIM provisioning page exposes WebMCP")
	}
}

func TestWebBaselineManagementRoutes(t *testing.T) {
	handler, auth, workspaceID, owner := testWeb(t, 10)
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	for _, target := range []string{
		"/workspaces/" + workspaceID.String() + "/settings/members",
		"/workspaces/" + workspaceID.String() + "/settings/invitations",
		"/workspaces/" + workspaceID.String() + "/settings/tokens",
		"/sessions", "/security", "/workspaces/" + workspaceID.String() + "/settings/export",
		"/workspaces/" + workspaceID.String() + "/settings/audit",
	} {
		response := request(handler, http.MethodGet, target, nil, cookies, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("baseline route %s = %d, %s", target, response.Code, response.Body.String())
		}
	}
	exportPage := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/export", nil, cookies, nil)
	if exportPage.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(exportPage.Body.String(), "Download JSON") || !strings.Contains(exportPage.Body.String(), "/settings/export/download") ||
		!strings.Contains(exportPage.Body.String(), `data-webmcp-page="export"`) || !strings.Contains(exportPage.Body.String(), `/assets/alpine-components.js`) {
		t.Fatal("browser export screen lacks download action")
	}
	importPage := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/import", nil, cookies, nil)
	if strings.Contains(importPage.Body.String(), "data-webmcp-page") || strings.Contains(importPage.Body.String(), `/assets/app.js`) {
		t.Fatal("browser import screen exposes WebMCP")
	}
	exportDownload := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"/settings/export/download", nil, cookies, nil)
	if exportDownload.Code != http.StatusOK || exportDownload.Header().Get("Cache-Control") != "no-store" ||
		exportDownload.Header().Get("Content-Disposition") == "" || exportDownload.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("browser export download = %d, %s", exportDownload.Code, exportDownload.Header().Get("Content-Type"))
	}
	created := request(handler, http.MethodPost, "/workspaces", url.Values{
		"name": {"Created from browser"}, "csrf": {session.CSRFSecret},
	}, cookies, nil)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/" {
		t.Fatalf("browser workspace creation = %d, %s", created.Code, created.Body.String())
	}
}

func multipartRequest(t *testing.T, handler http.Handler, target string, fields url.Values, filename, content string, cookies []*http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, values := range fields {
		for _, value := range values {
			if err := form.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	part, err := form.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, target, &body)
	request.Host = "example.com"
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("Content-Type", form.FormDataContentType())
	addCookies(request, cookies)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func importDigest(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

func TestWebSetupRequiresTokenAndBootstrapsFirstInstanceAdmin(t *testing.T) {
	setupToken := " " + strings.Repeat("x", 1020) + " ! "
	handler, auth := testUnbootstrappedWeb(t, setupToken)

	login := request(handler, http.MethodGet, "/login", nil, nil, nil)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/setup" {
		t.Fatalf("login redirect = %d, %q", login.Code, login.Header().Get("Location"))
	}
	page := request(handler, http.MethodGet, "/setup", nil, nil, nil)
	if page.Code != http.StatusOK ||
		!strings.Contains(page.Body.String(), `for="setup-token"`) ||
		!strings.Contains(page.Body.String(), `id="setup-token" name="setup_token" type="password"`) ||
		!strings.Contains(page.Body.String(), `required maxlength="4096"`) ||
		!strings.Contains(page.Body.String(), `autocomplete="off"`) ||
		!strings.Contains(page.Body.String(), `name="setup_token_format"`) {
		t.Fatalf("setup page = %d, %s", page.Code, page.Body.String())
	}
	assertAccessiblePage(t, page.Body.String(), "Setup token")
	csrf := responseCookie(page, "setup_csrf")

	missing := request(handler, http.MethodPost, "/setup", url.Values{"csrf": {csrf.Value}}, []*http.Cookie{csrf}, nil)
	if missing.Code != http.StatusForbidden {
		t.Fatalf("missing setup token = %d", missing.Code)
	}
	wrong := request(handler, http.MethodPost, "/setup", url.Values{
		"setup_token": {strings.TrimSpace(setupToken)}, "email": {"owner@example.com"},
		"workspace": {"Example"}, "password": {"long-enough-password"},
		"password_confirmation": {"long-enough-password"}, "csrf": {csrf.Value},
	}, []*http.Cookie{csrf}, nil)
	if wrong.Code != http.StatusForbidden || strings.Contains(wrong.Body.String(), setupToken) {
		t.Fatalf("non-exact setup token = %d, response leaked token=%t", wrong.Code, strings.Contains(wrong.Body.String(), setupToken))
	}
	created := request(handler, http.MethodPost, "/setup", url.Values{
		"setup_token": {setupToken}, "email": {"Owner@Example.com"},
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

func TestWebSetupEncodedToken(t *testing.T) {
	for _, token := range []string{"line\rbreak\nend\r\n!", strings.Repeat("界", 1024), "invalid-utf8-\xff"} {
		t.Run(fmt.Sprintf("bytes-%d", len(token)), func(t *testing.T) {
			handler, _ := testUnbootstrappedWeb(t, token)
			page := request(handler, http.MethodGet, "/setup", nil, nil, nil)
			csrf := responseCookie(page, "setup_csrf")
			encoded := base64.StdEncoding.EncodeToString([]byte(token))
			values := url.Values{"csrf": {csrf.Value}, "setup_token": {encoded}, "setup_token_format": {"base64"},
				"email": {"owner@example.com"}, "workspace": {"Example"}, "password": {"long-enough-password"},
				"password_confirmation": {"long-enough-password"}}
			for _, invalid := range []struct{ format, value, csrf string }{
				{"plain", encoded, csrf.Value}, {"base64", encoded + "!", csrf.Value},
				{"base64", encoded + "\n", csrf.Value}, {"unknown", encoded, csrf.Value}, {"base64", encoded, "wrong"},
			} {
				values.Set("setup_token_format", invalid.format)
				values.Set("setup_token", invalid.value)
				values.Set("csrf", invalid.csrf)
				response := request(handler, http.MethodPost, "/setup", values, []*http.Cookie{csrf}, nil)
				if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), encoded) {
					t.Fatalf("invalid encoded submission: status %d, leaked=%t", response.Code, strings.Contains(response.Body.String(), encoded))
				}
			}
			values.Set("setup_token_format", "base64")
			values.Set("setup_token", encoded)
			values.Set("csrf", csrf.Value)
			response := request(handler, http.MethodPost, "/setup", values, []*http.Cookie{csrf}, nil)
			if response.Code != http.StatusSeeOther {
				t.Fatalf("encoded submission: status %d", response.Code)
			}
		})
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

func TestWebRemoteSetupWorksWithoutToken(t *testing.T) {
	handler, auth := testUnbootstrappedWeb(t, " \t ")
	login := request(handler, http.MethodGet, "/login", nil, nil, nil)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/setup" {
		t.Fatalf("remote login redirect = %d, %q", login.Code, login.Header().Get("Location"))
	}
	page := request(handler, http.MethodGet, "/setup", nil, nil, nil)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), `name="setup_token"`) {
		t.Fatalf("remote tokenless setup page = %d, %s", page.Code, page.Body.String())
	}
	assertAccessiblePage(t, page.Body.String(), "Email")
	csrf := responseCookie(page, "setup_csrf")
	invalidCSRF := request(handler, http.MethodPost, "/setup", url.Values{
		"email": {"owner@example.com"}, "workspace": {"Example"},
		"password":              {"long-enough-password"},
		"password_confirmation": {"long-enough-password"}, "csrf": {"wrong"},
	}, []*http.Cookie{csrf}, nil)
	if invalidCSRF.Code != http.StatusForbidden {
		t.Fatalf("tokenless remote setup with invalid CSRF = %d", invalidCSRF.Code)
	}
	created := request(handler, http.MethodPost, "/setup", url.Values{
		"email": {"Owner@Example.com"}, "workspace": {"Example"},
		"password":              {"long-enough-password"},
		"password_confirmation": {"long-enough-password"}, "csrf": {csrf.Value},
	}, []*http.Cookie{csrf}, nil)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/login" {
		t.Fatalf("tokenless remote setup = %d, %s", created.Code, created.Body.String())
	}
	userID, err := auth.AuthenticateLocal(context.Background(), "owner@example.com", "long-enough-password")
	if err != nil || !auth.IsInstanceAdmin(context.Background(), userID) {
		t.Fatalf("tokenless first instance admin = %q, %v", userID, err)
	}
	workspaces, err := auth.ListWorkspaces(context.Background(), userID)
	if err != nil || len(workspaces) != 1 || workspaces[0].Name != "Example" || workspaces[0].Role != identity.Owner {
		t.Fatalf("tokenless first owner's workspaces = %+v, %v", workspaces, err)
	}
	if disabled := request(handler, http.MethodGet, "/setup", nil, nil, nil); disabled.Code != http.StatusNotFound {
		t.Fatalf("remote setup after bootstrap = %d", disabled.Code)
	}
}

func TestWebRemoteSetupConcurrentSubmissionsCreateOneOwner(t *testing.T) {
	handler, auth := testUnbootstrappedWeb(t, "")
	page := request(handler, http.MethodGet, "/setup", nil, nil, nil)
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), `name="setup_token"`) {
		t.Fatalf("remote tokenless setup page = %d, %s", page.Code, page.Body.String())
	}
	csrf := responseCookie(page, "setup_csrf")
	type ownerAttempt struct {
		email     string
		workspace string
	}
	attempts := []ownerAttempt{
		{email: "first@example.com", workspace: "First"},
		{email: "second@example.com", workspace: "Second"},
	}
	start := make(chan struct{})
	results := make(chan struct {
		attempt  ownerAttempt
		response *httptest.ResponseRecorder
	}, len(attempts))
	for _, attempt := range attempts {
		go func(attempt ownerAttempt) {
			<-start
			response := request(handler, http.MethodPost, "/setup", url.Values{
				"email": {attempt.email}, "workspace": {attempt.workspace},
				"password":              {"long-enough-password"},
				"password_confirmation": {"long-enough-password"}, "csrf": {csrf.Value},
			}, []*http.Cookie{csrf}, nil)
			results <- struct {
				attempt  ownerAttempt
				response *httptest.ResponseRecorder
			}{attempt: attempt, response: response}
		}(attempt)
	}
	close(start)
	for range attempts {
		result := <-results
		if result.response.Code != http.StatusSeeOther || result.response.Header().Get("Location") != "/login" {
			t.Errorf("bootstrap request for %s = %d, %s", result.attempt.email, result.response.Code, result.response.Body.String())
		}
	}
	owners := 0
	for _, attempt := range attempts {
		userID, err := auth.AuthenticateLocal(context.Background(), attempt.email, "long-enough-password")
		if err != nil {
			continue
		}
		owners++
		if !auth.IsInstanceAdmin(context.Background(), userID) {
			t.Errorf("user %s is not an instance administrator", attempt.email)
		}
		workspaces, err := auth.ListWorkspaces(context.Background(), userID)
		if err != nil || len(workspaces) != 1 || workspaces[0].Name != attempt.workspace || workspaces[0].Role != identity.Owner {
			t.Errorf("user %s workspaces = %+v, %v", attempt.email, workspaces, err)
		}
	}
	if owners != 1 {
		t.Fatalf("concurrent bootstrap created %d first owners, want 1", owners)
	}
	if disabled := request(handler, http.MethodGet, "/setup", nil, nil, nil); disabled.Code != http.StatusNotFound {
		t.Fatalf("remote setup after concurrent bootstrap = %d", disabled.Code)
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

func testWeb(t *testing.T, maximumStreams int, mailers ...appmail.Sender) (http.Handler, *identity.Service, identity.WorkspaceID, identity.BootstrapResult) {
	return testWebWithMFA(t, maximumStreams, false, mailers...)
}

func testWebRaw(t *testing.T, maximumStreams int, mailers ...appmail.Sender) (*Handler, *identity.Service, identity.WorkspaceID, identity.BootstrapResult) {
	t.Helper()
	return testWebWithMFARaw(t, maximumStreams, false, mailers...)
}

func testWebWithMFA(t *testing.T, maximumStreams int, mfaEnabled bool, mailers ...appmail.Sender) (http.Handler, *identity.Service, identity.WorkspaceID, identity.BootstrapResult) {
	t.Helper()
	return testWebConfigured(t, maximumStreams, mfaEnabled, nil, mailers...)
}

func testWebConfigured(t *testing.T, maximumStreams int, mfaEnabled bool, configure func(*Handler), mailers ...appmail.Sender) (http.Handler, *identity.Service, identity.WorkspaceID, identity.BootstrapResult) {
	t.Helper()
	webHandler, auth, workspaceID, owner := testWebWithMFARaw(t, maximumStreams, mfaEnabled, mailers...)
	if configure != nil {
		configure(webHandler)
	}
	return webHandler.Routes(), auth, workspaceID, owner
}

func testWebWithMFARaw(t *testing.T, maximumStreams int, mfaEnabled bool, mailers ...appmail.Sender) (*Handler, *identity.Service, identity.WorkspaceID, identity.BootstrapResult) {
	handler, auth, workspaceID, owner, _ := testWebWithMFARawDB(t, maximumStreams, mfaEnabled, mailers...)
	return handler, auth, workspaceID, owner
}

func testWebWithMFARawDB(t *testing.T, maximumStreams int, mfaEnabled bool, mailers ...appmail.Sender) (*Handler, *identity.Service, identity.WorkspaceID, identity.BootstrapResult, *sql.DB) {
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
	mfa := identity.MFAConfig{}
	if mfaEnabled {
		mfa = identity.MFAConfig{Enabled: true, EncryptionKey: []byte("01234567890123456789012345678901"), RelyingPartyID: "localhost", Origins: []string{"http://localhost:8080"}, DisplayName: "Dynamis Code", RequireForAdmins: true}
	}
	auth, err := identity.NewServiceWithMFA(db, cfg.Database.Driver, mfa)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: "long-enough-password", WorkspaceName: "Example",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var mailer appmail.Sender
	if len(mailers) > 0 {
		mailer = mailers[0]
	}
	webHandler, err := NewHandlerWithServices(auth,
		portability.NewService(db, cfg.Database.Driver, auth, cfg.Data.ExportMaxRecords, cfg.Data.ExportMaxBytes, nil),
		nil, cfg.HTTP, "", "", mailer)
	if err != nil {
		t.Fatal(err)
	}
	webHandler.EnableCatalog(catalog.NewService(db, auth, nil))
	webHandler.EnableListPreferences(listprefs.NewService(db, cfg.Database.Driver, auth))
	return webHandler, auth, owner.WorkspaceID, owner, db
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
	webHandler, err := NewHandler(auth, cfg.HTTP, setupToken)
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

func TestMFAPageInvalidChallengeClearsAllChallengeCookies(t *testing.T) {
	handler, _, _, _ := testWebWithMFA(t, 10, true)
	request := httptest.NewRequest(http.MethodGet, "/mfa", nil)
	request.AddCookie(&http.Cookie{Name: "mfa_challenge", Value: "invalid"})
	request.AddCookie(&http.Cookie{Name: "mfa_return_to", Value: "/invitations/example"})
	request.AddCookie(&http.Cookie{Name: "mfa_csrf", Value: "stale"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	for _, name := range []string{"mfa_challenge", "mfa_return_to", "mfa_csrf"} {
		if cookie := responseCookie(response, name); cookie.MaxAge != -1 {
			t.Fatalf("%s cookie max age = %d, want -1", name, cookie.MaxAge)
		}
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
	for _, required := range []string{`<!doctype html>`, `<html lang="en"`, `<meta name="viewport"`, `<main`, `<h1`} {
		if !strings.Contains(body, required) {
			t.Errorf("page lacks %s", required)
		}
	}
	if label != "" && !strings.Contains(body, label) {
		t.Errorf("page lacks label %q", label)
	}
}

// hasHTMLRootLang reports whether the document root declares lang. It matches
// only the lang attribute, so other root attributes such as data-theme do not
// make the assertion brittle.
func hasHTMLRootLang(body, locale string) bool {
	return strings.Contains(body, `<html lang="`+locale+`"`)
}

func loginWorkspaceUser(t *testing.T, handler http.Handler, email, password string) ([]*http.Cookie, string) {
	t.Helper()
	loginPage := request(handler, http.MethodGet, "/login", nil, nil, nil)
	loginCSRF := responseCookie(loginPage, "login_csrf")
	loggedIn := request(handler, http.MethodPost, "/login", url.Values{
		"email": {email}, "password": {password}, "csrf": {loginCSRF.Value},
	}, []*http.Cookie{loginCSRF}, nil)
	if loggedIn.Code != http.StatusSeeOther {
		t.Fatalf("login %s = %d", email, loggedIn.Code)
	}
	csrf := responseCookie(loggedIn, "csrf")
	return []*http.Cookie{responseCookie(loggedIn, "session"), csrf}, csrf.Value
}

func TestWebRootLandsInOldestWorkspaceInSingleMode(t *testing.T) {
	handler, auth, workspaceID, owner := testWebConfigured(t, 10, false, func(h *Handler) {
		h.SetWorkspaceMode(config.WorkspaceModeSingle)
	})
	cookies, _ := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	// Browser creation is closed in single mode; add the newer membership through the service.
	if _, err := auth.CreateWorkspace(context.Background(), identity.Principal{
		UserID: owner.UserID, AuthMethod: "local", AuthLevel: identity.AuthLevelPassword,
	}, identity.WorkspaceCreateInput{Name: "Newer"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	root := request(handler, http.MethodGet, "/", nil, cookies, nil)
	if root.Code != http.StatusFound || root.Header().Get("Location") != "/workspaces/"+workspaceID.String() {
		t.Fatalf("single-mode root = %d, Location %q; want 302 to oldest workspace", root.Code, root.Header().Get("Location"))
	}
}

func TestWebRootShowsNoMembershipsInSingleMode(t *testing.T) {
	handler, auth, workspaceID, owner := testWebConfigured(t, 10, false, func(h *Handler) {
		h.SetWorkspaceMode(config.WorkspaceModeSingle)
	})
	ctx := context.Background()
	inviter, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, inviter, "former@example.com", identity.Member, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	formerID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, "former-long-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	remover, err := auth.Authorize(ctx, owner.UserID, workspaceID, identity.MembersManage)
	if err != nil {
		t.Fatal(err)
	}
	members, err := auth.ListMembers(ctx, remover)
	if err != nil {
		t.Fatal(err)
	}
	var formerMember identity.MemberID
	for _, member := range members {
		if member.UserID == formerID {
			formerMember = member.ID
		}
	}
	if formerMember == 0 {
		t.Fatal("former member was not listed")
	}
	if err := auth.RemoveMember(ctx, remover, formerMember, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	cookies, _ := loginWorkspaceUser(t, handler, "former@example.com", "former-long-password")
	root := request(handler, http.MethodGet, "/", nil, cookies, nil)
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), "No workspace memberships.") {
		t.Fatalf("single-mode root without memberships = %d, %s", root.Code, root.Body.String())
	}
	if strings.Contains(root.Body.String(), `action="/workspaces"`) {
		t.Fatal("single-mode root shows the create-workspace form")
	}
}

func TestWebRootListsWorkspacesInMultiMode(t *testing.T) {
	handler, _, workspaceID, _ := testWebConfigured(t, 10, false, func(h *Handler) {
		h.SetWorkspaceMode(config.WorkspaceModeMulti)
	})
	cookies, csrf := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	// A single membership lands in its workspace; the picker needs two to show.
	second := request(handler, http.MethodPost, "/workspaces", url.Values{"csrf": {csrf}, "name": {"Second"}}, cookies, nil)
	if second.Code != http.StatusSeeOther {
		t.Fatalf("seed second workspace = %d", second.Code)
	}
	root := request(handler, http.MethodGet, "/", nil, cookies, nil)
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), workspaceID.String()) {
		t.Fatalf("multi-mode root = %d, %s; want workspace list", root.Code, root.Body.String())
	}
}

func TestSingleModeHidesWorkspaceListNavigation(t *testing.T) {
	handler, _, workspaceID, _ := testWebConfigured(t, 10, false, func(h *Handler) {
		h.SetWorkspaceMode(config.WorkspaceModeSingle)
	})
	cookies, _ := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	workspaceRoot := "/workspaces/" + workspaceID.String()
	for _, path := range []string{workspaceRoot, workspaceRoot + "/settings/members", workspaceRoot + "/settings/tokens", workspaceRoot + "/settings/provisioning"} {
		response := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil)
		if response.Code != http.StatusOK {
			t.Errorf("%s = %d in single mode; want 200", path, response.Code)
		}
		body := response.Body.String()
		if strings.Contains(body, `<li class="menu-title">Home</li>`) || strings.Contains(body, `href="/">← Workspaces</a>`) {
			t.Errorf("%s shows a workspace list link in single mode", path)
		}
	}
	for _, path := range []string{workspaceRoot, workspaceRoot + "/settings/members"} {
		body := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil).Body.String()
		if !strings.Contains(body, `<span class="truncate font-bold">`) {
			t.Errorf("%s lacks plain-text workspace name in single mode", path)
		}
	}
	for _, path := range []string{"/account", workspaceRoot + "/settings/audit", workspaceRoot + "/settings/export"} {
		body := request(handler, http.MethodGet, path+"?all=1", nil, cookies, nil).Body.String()
		if !strings.Contains(body, `href="`+workspaceRoot+`"`) {
			t.Errorf("%s no longer links back to the default workspace home in single mode", path)
		}
	}
}

func TestMultiModeKeepsWorkspaceListNavigation(t *testing.T) {
	handler, _, workspaceID, _ := testWebConfigured(t, 10, false, func(h *Handler) {
		h.SetWorkspaceMode(config.WorkspaceModeMulti)
	})
	cookies, _ := loginWorkspaceUser(t, handler, "owner@example.com", "long-enough-password")
	body := request(handler, http.MethodGet, "/workspaces/"+workspaceID.String()+"?all=1", nil, cookies, nil).Body.String()
	if !strings.Contains(body, `<details class="dropdown dropdown-end">`) || !strings.Contains(body, `<a class="btn btn-sm w-full justify-start font-normal" href="/">← Workspaces</a>`) {
		t.Error("multi-mode items page lost workspace list navigation")
	}
}
