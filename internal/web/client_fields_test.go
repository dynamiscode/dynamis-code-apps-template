package web

import (
	"context"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func (env clientsTestEnv) fieldsPath() string {
	return "/workspaces/" + env.workspaceID.String() + "/client-fields"
}

func TestClientFieldsDefineArchiveAndRestoreThroughForms(t *testing.T) {
	env := newClientsTestEnv(t)
	page := request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `name="label"`) ||
		!strings.Contains(page.Body.String(), `id="client-field-label"`) ||
		!strings.Contains(page.Body.String(), `class="input w-full"`) ||
		!strings.Contains(page.Body.String(), `class="btn btn-primary" type="submit"`) {
		t.Fatalf("owner fields page rendering is incorrect (%d): %s", page.Code, page.Body.String())
	}

	created := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
		"csrf": {env.csrf}, "label": {"<b>Referral</b> source"}, "type": {"text"},
	}, env.cookies, nil)
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != env.fieldsPath() {
		t.Fatalf("define = %d location %q: %s", created.Code, created.Header().Get("Location"), created.Body.String())
	}
	list := request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil)
	if !strings.Contains(list.Body.String(), "&lt;b&gt;Referral&lt;/b&gt; source") || strings.Contains(list.Body.String(), "<b>Referral") {
		t.Fatalf("label is not rendered exactly as typed: %s", list.Body.String())
	}

	var fieldKey int64
	if err := env.db.QueryRow("SELECT id FROM client_fields").Scan(&fieldKey); err != nil {
		t.Fatal(err)
	}
	shortID := clients.FieldID(fieldKey).String()
	archive := request(env.routes, http.MethodPost, env.fieldsPath()+"/"+shortID+"/archive", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if archive.Code != http.StatusSeeOther {
		t.Fatalf("archive = %d: %s", archive.Code, archive.Body.String())
	}
	archived := request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.cookies, nil)
	if !strings.Contains(archived.Body.String(), env.fieldsPath()+"/"+shortID+"/restore") {
		t.Fatalf("archived field does not offer restore: %s", archived.Body.String())
	}
	restore := request(env.routes, http.MethodPost, env.fieldsPath()+"/"+shortID+"/restore", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if restore.Code != http.StatusSeeOther {
		t.Fatalf("restore = %d: %s", restore.Code, restore.Body.String())
	}

	blank := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
		"csrf": {env.csrf}, "label": {"   "}, "type": {"text"},
	}, env.cookies, nil)
	if blank.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank label = %d, want 422", blank.Code)
	}
	unknown := request(env.routes, http.MethodPost, env.fieldsPath()+"/"+clients.FieldID(999999).String()+"/archive", url.Values{"csrf": {env.csrf}}, env.cookies, nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown field archive = %d, want 404", unknown.Code)
	}
}

func TestClientFieldFormsRefuseViewersAndMembers(t *testing.T) {
	env := newClientsTestEnv(t)
	viewerPage := request(env.routes, http.MethodGet, env.fieldsPath(), nil, env.viewerCookies, nil)
	if viewerPage.Code != http.StatusOK || strings.Contains(viewerPage.Body.String(), `name="label"`) {
		t.Fatalf("viewer fields page = %d, want read-only: %s", viewerPage.Code, viewerPage.Body.String())
	}
	viewerPost := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
		"csrf": {env.viewerCSRF}, "label": {"Blocked"}, "type": {"text"},
	}, env.viewerCookies, nil)
	if viewerPost.Code != http.StatusForbidden {
		t.Fatalf("viewer define = %d, want 403", viewerPost.Code)
	}

	memberCookies, memberCSRF := addWebMember(t, env, "web-member@example.com")
	memberPost := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
		"csrf": {memberCSRF}, "label": {"Blocked"}, "type": {"text"},
	}, memberCookies, nil)
	if memberPost.Code != http.StatusForbidden {
		t.Fatalf("member define = %d, want 403", memberPost.Code)
	}
}

// addWebMember signs in a Member of the test workspace and returns its cookies
// and CSRF token.
func addWebMember(t *testing.T, env clientsTestEnv, email string) ([]*http.Cookie, string) {
	t.Helper()
	ctx := context.Background()
	var userID identity.UserID
	if err := env.db.QueryRow("INSERT INTO users (email, created_at) VALUES (?, ?) RETURNING id",
		email, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	owner, err := env.auth.Authorize(ctx, env.ownerID, env.workspaceID, identity.MembersManage)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.auth.AddMember(ctx, owner, userID, identity.Member, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	session, err := env.auth.CreateSession(ctx, userID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}, session.CSRFSecret
}

// TestClientFieldRefusalKeepsTheChosenType re-renders the define form with the
// type the owner picked, so a refused save does not reset it to Text.
func TestClientFieldRefusalKeepsTheChosenType(t *testing.T) {
	env := newClientsTestEnv(t)
	refused := request(env.routes, http.MethodPost, env.fieldsPath(), url.Values{
		"csrf": {env.csrf}, "label": {""}, "type": {"number"},
	}, env.cookies, nil)
	if refused.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank label = %d: %s", refused.Code, refused.Body.String())
	}
	if !strings.Contains(refused.Body.String(), `<option selected value="number">`) {
		t.Fatalf("refused form lost the chosen type: %s", refused.Body.String())
	}
}
