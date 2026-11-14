package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// This file is the authorization and isolation matrix for Clients. Each row is
// one action, and each row lists the roles that may perform it. Every row runs
// on every surface the row defines (REST and web today). MCP and appctl rows
// are added as further surface functions on clientAuthorizationRow, reusing the
// same fixture and the same role table.

const matrixPassword = "long-enough-password"

// matrixMissingID is a well-formed key that names no record in any workspace.
const matrixMissingID = 999999

var clientAuthorizationRoles = []identity.Role{identity.Owner, identity.Admin, identity.Member, identity.Viewer}

var (
	allMatrixRoles = clientAuthorizationRoles
	writerRoles    = []identity.Role{identity.Owner, identity.Admin, identity.Member}
	managerRoles   = []identity.Role{identity.Owner, identity.Admin}
)

// matrixWorkspace names the workspace a request addresses and the record keys
// it uses. Foreign and missing variants reuse the same struct with other keys.
type matrixWorkspace struct {
	id             identity.WorkspaceID
	client         clients.ClientID
	archivedClient clients.ClientID
	version        int64
	textField      clients.FieldID
	choiceField    clients.FieldID
	option         clients.OptionID
	archiveField   clients.FieldID
	restoreField   clients.FieldID
	deleteField    clients.FieldID
}

type matrixRequest struct {
	method  string
	path    string
	body    string
	form    url.Values
	ifMatch string
}

type clientAuthorizationRow struct {
	action string
	allow  []identity.Role
	// rest builds the REST request; restOK is the status an allowed role gets.
	rest   func(ws matrixWorkspace) matrixRequest
	restOK int
	// web builds the browser form request; webOK is the status an allowed role gets.
	web   func(ws matrixWorkspace) matrixRequest
	webOK int
}

func restBase(ws matrixWorkspace) string { return "/api/v1/workspaces/" + ws.id.String() }
func webBase(ws matrixWorkspace) string  { return "/workspaces/" + ws.id.String() }

func versionTag(version int64) string { return `"v` + strconv.FormatInt(version, 10) + `"` }

var clientAuthorizationMatrix = []clientAuthorizationRow{
	{
		action: "list clients", allow: allMatrixRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodGet, path: restBase(ws) + "/clients"}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodGet, path: webBase(ws) + "/clients"}
		},
		webOK: http.StatusOK,
	},
	{
		action: "get client", allow: allMatrixRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodGet, path: restBase(ws) + "/clients/" + ws.client.String()}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodGet, path: webBase(ws) + "/clients/" + ws.client.String()}
		},
		webOK: http.StatusOK,
	},
	{
		action: "create client", allow: writerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: restBase(ws) + "/clients", body: `{"displayName":"Matrix"}`}
		},
		restOK: http.StatusCreated,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/clients", form: url.Values{"display_name": {"Matrix"}}}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "edit client", allow: writerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPatch, path: restBase(ws) + "/clients/" + ws.client.String(),
				body: `{"displayName":"Edited"}`, ifMatch: versionTag(ws.version)}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/clients/" + ws.client.String(),
				form: url.Values{"display_name": {"Edited"}, "version": {strconv.FormatInt(ws.version, 10)}}}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "set value", allow: writerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPut, path: restBase(ws) + "/clients/" + ws.client.String() + "/values/" + ws.textField.String(),
				body: `{"value":"Matrix"}`, ifMatch: versionTag(ws.version)}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/clients/" + ws.client.String() + "/values/" + ws.textField.String(),
				form: url.Values{"value": {"Matrix"}, "version": {strconv.FormatInt(ws.version, 10)}}}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "clear value", allow: writerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodDelete, path: restBase(ws) + "/clients/" + ws.client.String() + "/values/" + ws.textField.String(),
				ifMatch: versionTag(ws.version)}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/clients/" + ws.client.String() + "/values/" + ws.textField.String() + "/clear",
				form: url.Values{"version": {strconv.FormatInt(ws.version, 10)}}}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "archive client", allow: writerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: restBase(ws) + "/clients/" + ws.client.String() + "/archive"}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/clients/" + ws.client.String() + "/archive"}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "unarchive client", allow: writerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: restBase(ws) + "/clients/" + ws.archivedClient.String() + "/unarchive"}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/clients/" + ws.archivedClient.String() + "/unarchive"}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "delete client", allow: managerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodDelete, path: restBase(ws) + "/clients/" + ws.client.String()}
		},
		restOK: http.StatusNoContent,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/clients/" + ws.client.String() + "/delete"}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "define field", allow: managerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: restBase(ws) + "/client-fields", body: `{"label":"Tier","type":"text"}`}
		},
		restOK: http.StatusCreated,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/client-fields", form: url.Values{"label": {"Tier"}, "type": {"text"}}}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "archive field", allow: managerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: restBase(ws) + "/client-fields/" + ws.archiveField.String() + "/archive"}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/client-fields/" + ws.archiveField.String() + "/archive"}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "restore field", allow: managerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: restBase(ws) + "/client-fields/" + ws.restoreField.String() + "/restore"}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/client-fields/" + ws.restoreField.String() + "/restore"}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "delete field", allow: managerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodDelete, path: restBase(ws) + "/client-fields/" + ws.deleteField.String()}
		},
		restOK: http.StatusNoContent,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/client-fields/" + ws.deleteField.String() + "/delete"}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "add option", allow: managerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: restBase(ws) + "/client-fields/" + ws.choiceField.String() + "/options", body: `{"label":"Silver"}`}
		},
		restOK: http.StatusCreated,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/client-fields/" + ws.choiceField.String() + "/options", form: url.Values{"label": {"Silver"}}}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "delete option", allow: managerRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodDelete, path: restBase(ws) + "/client-fields/" + ws.choiceField.String() + "/options/" + ws.option.String()}
		},
		restOK: http.StatusNoContent,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodPost, path: webBase(ws) + "/client-fields/" + ws.choiceField.String() + "/options/" + ws.option.String() + "/delete"}
		},
		webOK: http.StatusSeeOther,
	},
	{
		action: "list fields", allow: allMatrixRoles,
		rest: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodGet, path: restBase(ws) + "/client-fields"}
		},
		restOK: http.StatusOK,
		web: func(ws matrixWorkspace) matrixRequest {
			return matrixRequest{method: http.MethodGet, path: webBase(ws) + "/client-fields"}
		},
		webOK: http.StatusOK,
	},
}

// matrixActor is one signed-in identity in a workspace.
type matrixActor struct {
	cookies []*http.Cookie
	csrf    string
	token   string
}

// matrixFixture is a fresh application with workspace A seeded for every row,
// one signed-in actor per role in A, and a second workspace B whose admin is
// not a member of A.
type matrixFixture struct {
	app    *App
	a      matrixWorkspace
	actors map[identity.Role]matrixActor
	b      matrixWorkspace
	bAdmin matrixActor
}

func newClientAuthorizationFixture(t *testing.T) matrixFixture {
	t.Helper()
	ctx := context.Background()
	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.SQLitePath = ":memory:"
	cfg.Database.MaxOpenConns = 1
	cfg.Database.MaxIdleConns = 1
	app, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	auth := app.Identity
	audit := identity.AuditContext{}

	owner, err := auth.BootstrapFirstOwner(ctx, identity.BootstrapInput{
		Email: "owner@example.com", Password: matrixPassword, WorkspaceName: "Matrix A",
	}, audit)
	if err != nil {
		t.Fatal(err)
	}
	ownerPrincipal, err := auth.Authorize(ctx, owner.UserID, owner.WorkspaceID, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	workspaceB, err := auth.CreateWorkspace(ctx, ownerPrincipal, identity.WorkspaceCreateInput{Name: "Matrix B"}, audit)
	if err != nil {
		t.Fatal(err)
	}

	f := matrixFixture{app: app, actors: map[identity.Role]matrixActor{}}
	f.actors[identity.Owner] = matrixSignIn(t, auth, owner.UserID, owner.WorkspaceID)
	for role, email := range map[identity.Role]string{
		identity.Admin: "admin@example.com", identity.Member: "member@example.com", identity.Viewer: "viewer@example.com",
	} {
		userID := matrixInvite(t, auth, owner.UserID, owner.WorkspaceID, email, role)
		f.actors[role] = matrixSignIn(t, auth, userID, owner.WorkspaceID)
	}
	bAdminID := matrixInvite(t, auth, owner.UserID, workspaceB, "b-admin@example.com", identity.Admin)
	f.bAdmin = matrixSignIn(t, auth, bAdminID, workspaceB)

	f.a = seedMatrixWorkspace(t, app, cfg.Data.ClientFieldsMaxPerWorkspace, ownerPrincipal, owner.WorkspaceID)
	f.b = matrixWorkspace{id: workspaceB}
	return f
}

// seedMatrixWorkspace creates the records each row targets. The service is
// called directly so setup does not depend on the surfaces under test.
func seedMatrixWorkspace(t *testing.T, app *App, maxFields int, owner identity.Principal, wsID identity.WorkspaceID) matrixWorkspace {
	t.Helper()
	ctx := context.Background()
	audit := identity.AuditContext{}
	svc := clients.NewService(app.DB, config.SQLite, app.Identity, maxFields)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ws := matrixWorkspace{id: wsID}

	active, err := svc.Create(ctx, owner, wsID, clients.Input{DisplayName: "Acme"}, audit)
	must(err)
	archived, err := svc.Create(ctx, owner, wsID, clients.Input{DisplayName: "Old Account"}, audit)
	must(err)
	_, err = svc.Archive(ctx, owner, wsID, archived.ID, audit)
	must(err)

	text, err := svc.DefineField(ctx, owner, wsID, clients.FieldInput{Label: "Source", Type: clients.FieldTypeText}, audit)
	must(err)
	choice, err := svc.DefineField(ctx, owner, wsID, clients.FieldInput{Label: "Tier", Type: clients.FieldTypeSingleChoice}, audit)
	must(err)
	option, err := svc.AddOption(ctx, owner, wsID, choice.ID, "Gold", audit)
	must(err)
	archiveField, err := svc.DefineField(ctx, owner, wsID, clients.FieldInput{Label: "Notes", Type: clients.FieldTypeText}, audit)
	must(err)
	restoreField, err := svc.DefineField(ctx, owner, wsID, clients.FieldInput{Label: "Legacy", Type: clients.FieldTypeText}, audit)
	must(err)
	_, err = svc.ArchiveField(ctx, owner, wsID, restoreField.ID, audit)
	must(err)
	deleteField, err := svc.DefineField(ctx, owner, wsID, clients.FieldInput{Label: "Retired", Type: clients.FieldTypeText}, audit)
	must(err)
	_, err = svc.ArchiveField(ctx, owner, wsID, deleteField.ID, audit)
	must(err)

	valued, err := svc.SetValue(ctx, owner, wsID, active.ID, active.Version, text.ID, "Referral", audit)
	must(err)

	ws.client = active.ID
	ws.archivedClient = archived.ID
	ws.version = valued.Version
	ws.textField = text.ID
	ws.choiceField = choice.ID
	ws.option = option.ID
	ws.archiveField = archiveField.ID
	ws.restoreField = restoreField.ID
	ws.deleteField = deleteField.ID
	return ws
}

func matrixInvite(t *testing.T, auth *identity.Service, ownerID identity.UserID, wsID identity.WorkspaceID, email string, role identity.Role) identity.UserID {
	t.Helper()
	ctx := context.Background()
	principal, err := auth.Authorize(ctx, ownerID, wsID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := auth.CreateInvitation(ctx, principal, email, role, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	userID, _, err := auth.CreateInvitedLocalUser(ctx, invitation.Secret, matrixPassword, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return userID
}

// matrixSignIn gives a user a browser session and an API token whose scopes
// are the workspace permissions the user holds, as a real client would obtain.
func matrixSignIn(t *testing.T, auth *identity.Service, userID identity.UserID, wsID identity.WorkspaceID) matrixActor {
	t.Helper()
	ctx := context.Background()
	principal, err := auth.Authorize(ctx, userID, wsID, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, userID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	var scopes []identity.Permission
	for _, permission := range []identity.Permission{
		identity.WorkspaceRead, identity.WorkspaceUpdate, identity.ResourcesRead, identity.ResourcesWrite,
	} {
		if principal.Permissions[permission] {
			scopes = append(scopes, permission)
		}
	}
	token, err := auth.CreateAPIToken(ctx, principal, "matrix", scopes, nil, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	return matrixActor{
		cookies: []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}},
		csrf:    session.CSRFSecret,
		token:   token.Secret,
	}
}

func serveMatrixREST(handler http.Handler, actor matrixActor, req matrixRequest) *httptest.ResponseRecorder {
	request := httptest.NewRequest(req.method, req.path, bytes.NewBufferString(req.body))
	request.Header.Set("Authorization", "Bearer "+actor.token)
	if req.body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if req.ifMatch != "" {
		request.Header.Set("If-Match", req.ifMatch)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func serveMatrixWeb(handler http.Handler, actor matrixActor, req matrixRequest) *httptest.ResponseRecorder {
	form := url.Values{}
	for key, values := range req.form {
		form[key] = values
	}
	if req.method != http.MethodGet {
		form.Set("csrf", actor.csrf)
	}
	request := httptest.NewRequest(req.method, req.path, strings.NewReader(form.Encode()))
	request.Host = "example.com"
	request.RemoteAddr = "192.0.2.1:1234"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range actor.cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// matrixProblem keeps the parts of a Problem Details body that must match
// between a foreign and a missing record. Request IDs and instances differ per
// request by design.
func matrixProblem(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Header().Get("Content-Type") != "application/problem+json" {
		return response.Body.String()
	}
	var value map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	delete(value, "requestId")
	delete(value, "instance")
	normalized, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(normalized)
}

func TestClientAuthorizationMatrix(t *testing.T) {
	t.Parallel()
	surfaces := []struct {
		name    string
		allowed func(row clientAuthorizationRow) int
		call    func(f matrixFixture, actor matrixActor, row clientAuthorizationRow) *httptest.ResponseRecorder
	}{
		{
			name:    "rest",
			allowed: func(row clientAuthorizationRow) int { return row.restOK },
			call: func(f matrixFixture, actor matrixActor, row clientAuthorizationRow) *httptest.ResponseRecorder {
				return serveMatrixREST(f.app.Handler, actor, row.rest(f.a))
			},
		},
		{
			name:    "web",
			allowed: func(row clientAuthorizationRow) int { return row.webOK },
			call: func(f matrixFixture, actor matrixActor, row clientAuthorizationRow) *httptest.ResponseRecorder {
				return serveMatrixWeb(f.app.Handler, actor, row.web(f.a))
			},
		},
	}
	for _, surface := range surfaces {
		for _, row := range clientAuthorizationMatrix {
			for _, role := range clientAuthorizationRoles {
				t.Run(surface.name+"/"+row.action+"/"+string(role), func(t *testing.T) {
					t.Parallel()
					f := newClientAuthorizationFixture(t)
					response := surface.call(f, f.actors[role], row)
					if slices.Contains(row.allow, role) {
						if want := surface.allowed(row); response.Code != want {
							t.Fatalf("%s %s as %s = %d, want %d: %s", surface.name, row.action, role, response.Code, want, response.Body.String())
						}
						return
					}
					if response.Code != http.StatusForbidden {
						t.Fatalf("%s %s as %s = %d, want 403: %s", surface.name, row.action, role, response.Code, response.Body.String())
					}
				})
			}
		}
	}
}

// TestClientAuthorizationIsolatesWorkspaces checks that a caller in workspace B
// reaching workspace A's records gets the same response as for a record that
// does not exist. Workspace B's admin is used because that role passes the
// permission check in B, so the answer comes from the data layer.
func TestClientAuthorizationIsolatesWorkspaces(t *testing.T) {
	t.Parallel()
	for _, surface := range []string{"rest", "web"} {
		for _, row := range clientAuthorizationMatrix {
			t.Run(surface+"/"+row.action, func(t *testing.T) {
				t.Parallel()
				f := newClientAuthorizationFixture(t)
				call := func(ws matrixWorkspace) *httptest.ResponseRecorder {
					if surface == "rest" {
						return serveMatrixREST(f.app.Handler, f.bAdmin, row.rest(ws))
					}
					return serveMatrixWeb(f.app.Handler, f.bAdmin, row.web(ws))
				}
				foreign := f.a
				foreign.id = f.b.id
				missing := matrixWorkspace{
					id: f.b.id, client: matrixMissingID, archivedClient: matrixMissingID, version: f.a.version,
					textField: matrixMissingID, choiceField: matrixMissingID, option: matrixMissingID,
					archiveField: matrixMissingID, restoreField: matrixMissingID, deleteField: matrixMissingID,
				}
				// Rows that name a record in the path compare the foreign key with a
				// missing key. Rows that name none (list, create, define) have the
				// same path for both, so they are checked on what they return.
				if row.rest(foreign).path != row.rest(missing).path || row.web(foreign).path != row.web(missing).path {
					foreignResponse := call(foreign)
					missingResponse := call(missing)
					if foreignResponse.Code < 400 || missingResponse.Code < 400 {
						t.Fatalf("B admin reached %s with status %d/%d, want a refusal", row.action, foreignResponse.Code, missingResponse.Code)
					}
					if foreignResponse.Code != missingResponse.Code || matrixProblem(t, foreignResponse) != matrixProblem(t, missingResponse) {
						t.Fatalf("foreign %s = %d %s, missing = %d %s; want identical",
							row.action, foreignResponse.Code, foreignResponse.Body.String(), missingResponse.Code, missingResponse.Body.String())
					}
				} else if row.rest(foreign).method == http.MethodGet {
					listed := call(foreign)
					for _, key := range []string{f.a.client.String(), f.a.archivedClient.String(), f.a.textField.String(), f.a.choiceField.String(), f.a.option.String()} {
						if strings.Contains(listed.Body.String(), key) {
							t.Fatalf("B admin %s lists workspace A record %s: %s", row.action, key, listed.Body.String())
						}
					}
				}

				// A caller in B naming workspace A itself must see workspace-not-found.
				ownWorkspace := f.a
				ownResponse := call(ownWorkspace)
				if ownResponse.Code != http.StatusNotFound {
					t.Fatalf("B admin on workspace A %s = %d, want 404: %s", row.action, ownResponse.Code, ownResponse.Body.String())
				}
			})
		}
	}
}
