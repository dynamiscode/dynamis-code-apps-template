package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestCatalogArchiveAndDelete(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspaceID + "/catalog"
	created := serveAuthorized(handler, http.MethodPost, path, `{"name":"Consultation","prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, token, nil)
	var entry struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &entry); err != nil || entry.ID == "" {
		t.Fatalf("create: %s, %v", created.Body.String(), err)
	}
	entryPath := path + "/" + entry.ID
	response := serveAuthorized(handler, http.MethodPost, entryPath+"/archive", "", token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("archive = %d %s", response.Code, response.Body.String())
	}
	active := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if strings.Contains(active.Body.String(), "Consultation") {
		t.Fatal("archived entry in default list")
	}
	all := serveAuthorized(handler, http.MethodGet, path+"?includeArchived=true", "", token, nil)
	if !strings.Contains(all.Body.String(), "Consultation") {
		t.Fatalf("archived list = %s", all.Body.String())
	}
	response = serveAuthorized(handler, http.MethodPost, entryPath+"/unarchive", "", token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("unarchive = %d %s", response.Code, response.Body.String())
	}
	response = serveAuthorized(handler, http.MethodDelete, entryPath, "", token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", response.Code, response.Body.String())
	}
}

func TestCatalogDeleteInUse(t *testing.T) {
	routes, db, workspaceID, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspaceID + "/catalog"
	created := serveAuthorized(routes, http.MethodPost, path, `{"name":"In use","prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, token, nil)
	var entry struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &entry); err != nil || entry.ID == "" {
		t.Fatal("missing entry ID")
	}
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	service := catalog.NewService(db, auth, func(context.Context, *sql.Tx, identity.WorkspaceID, catalog.EntryID) (bool, error) { return true, nil })
	h := &handler{identity: auth, catalogService: service}
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/workspaces/{workspaceId}/catalog/{entryId}", h.deleteCatalog)
	response := serveAuthorized(mux, http.MethodDelete, path+"/"+entry.ID, "", token, nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "archive") {
		t.Fatalf("delete = %d %s", response.Code, response.Body.String())
	}
}

func TestCatalogCreateAndList(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspaceID + "/catalog"
	created := serveAuthorized(handler, http.MethodPost, path, `{"name":"Consultation","durationMinutes":30,"prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, token, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", created.Code, created.Body.String())
	}
	list := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"Consultation"`) || !strings.Contains(list.Body.String(), `"durationMinutes":30`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	invalid := serveAuthorized(handler, http.MethodPost, path, `{"name":"  "}`, token, nil)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid create = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestCatalogRESTRolePermissions(t *testing.T) {
	handler, db, workspace, _ := testHandler(t)
	ctx := context.Background()
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(workspace)
	var ownerID identity.UserID
	if err := db.QueryRowContext(ctx, "SELECT user_id FROM workspace_members WHERE workspace_id = ? AND role = 'owner'", workspaceID).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authorize(ctx, ownerID, workspaceID, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + workspace + "/catalog"
	for _, role := range []identity.Role{identity.Owner, identity.Admin, identity.Member, identity.Viewer} {
		userID := ownerID
		if role != identity.Owner {
			invitation, err := auth.CreateInvitation(ctx, owner, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
			userID, _, err = auth.CreateInvitedLocalUser(ctx, invitation.Secret, "long-enough-password", identity.AuditContext{})
			if err != nil {
				t.Fatal(err)
			}
		}
		principal, err := auth.Authorize(ctx, userID, workspaceID, identity.ResourcesRead)
		if err != nil {
			t.Fatal(err)
		}
		scopes := []identity.Permission{identity.ResourcesRead}
		if role != identity.Viewer {
			scopes = append(scopes, identity.ResourcesWrite)
		}
		token, err := auth.CreateAPIToken(ctx, principal, "catalog", scopes, nil, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		response := serveAuthorized(handler, http.MethodPost, path, `{"name":"Role test","prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, token.Secret, nil)
		want := http.StatusForbidden
		if role == identity.Owner || role == identity.Admin {
			want = http.StatusCreated
		}
		if response.Code != want {
			t.Errorf("%s create = %d %s, want %d", role, response.Code, response.Body.String(), want)
		}
		response = serveAuthorized(handler, http.MethodGet, path, "", token.Secret, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Role test") {
			t.Errorf("%s list = %d %s", role, response.Code, response.Body.String())
		}
		if role == identity.Member || role == identity.Viewer {
			var entries []struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &entries); err != nil || len(entries) == 0 {
				t.Fatal("missing catalog entry")
			}
			for _, operation := range []struct{ method, suffix string }{{http.MethodPost, "/archive"}, {http.MethodPost, "/unarchive"}, {http.MethodDelete, ""}, {http.MethodPut, ""}, {http.MethodPut, "/prices/aaaa"}, {http.MethodDelete, "/prices/aaaa"}} {
				response = serveAuthorized(handler, operation.method, path+"/"+entries[0].ID+operation.suffix, "", token.Secret, nil)
				if response.Code != http.StatusForbidden {
					t.Errorf("%s %s = %d", role, operation.suffix, response.Code)
				}
			}
		}
	}
}

func TestCatalogList(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspaceID + "/catalog"
	response := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("catalog = %d %s", response.Code, response.Body.String())
	}
	foreignPath := "/api/v1/workspaces/" + identity.WorkspaceID(999999).String() + "/catalog"
	foreign := serveAuthorized(handler, http.MethodGet, foreignPath, "", token, nil)
	if foreign.Code == http.StatusOK {
		t.Fatal("catalog returned 200 outside workspace")
	}
	denied := serveAuthorized(handler, http.MethodGet, path, "", "invalid", nil)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated = %d", denied.Code)
	}
}

func TestCatalogEditREST(t *testing.T) {
	handler, db, workspace, token := testHandler(t)
	seedCatalogCurrencies(t, db, workspace)
	path := "/api/v1/workspaces/" + workspace + "/catalog"
	if invalid := serveAuthorized(handler, http.MethodPost, path, `{"name":"No price"}`, token, nil); invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing price: %d", invalid.Code)
	}
	created := serveAuthorized(handler, http.MethodPost, path, `{"name":"Visit","prices":[{"mode":"fixed","amount":"0.12","currency":"EUR","variant":"Initial"}]}`, token, nil)
	var entry struct {
		ID     string `json:"id"`
		Prices []struct {
			ID     string `json:"id"`
			Amount string `json:"amount"`
		} `json:"prices"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &entry); err != nil || entry.ID == "" || len(entry.Prices) != 1 || entry.Prices[0].Amount != "0.12" {
		t.Fatal(created.Body.String())
	}
	entryPath := path + "/" + entry.ID
	price := serveAuthorized(handler, http.MethodPost, entryPath+"/prices", `{"mode":"fixed","amount":"0.12","currency":"EUR","variant":"Visit"}`, token, nil)
	var row struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(price.Body.Bytes(), &row); err != nil || row.ID == "" {
		t.Fatalf("price ID: %s", price.Body.String())
	}
	response := serveAuthorized(handler, http.MethodPut, entryPath, `{"name":"Updated","durationMinutes":45}`, token, map[string]string{"If-Match": `"v1"`})
	if response.Code != http.StatusNoContent {
		t.Fatalf("edit entry: %d %s", response.Code, response.Body.String())
	}
	pricePath := entryPath + "/prices/" + row.ID
	response = serveAuthorized(handler, http.MethodPut, pricePath, `{"mode":"manual","currency":"EUR","variant":"Week"}`, token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("edit price: %d %s", response.Code, response.Body.String())
	}
	response = serveAuthorized(handler, http.MethodPut, pricePath, `{"mode":"fixed","amount":"0.12","currency":"EUR","variant":"Visit"}`, token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("fixed price: %d %s", response.Code, response.Body.String())
	}
	list := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if !strings.Contains(list.Body.String(), `"amount":"0.12"`) || !strings.Contains(list.Body.String(), `"name":"Updated"`) {
		t.Fatal(list.Body.String())
	}
	response = serveAuthorized(handler, http.MethodPut, pricePath, `{"mode":"fixed","amount":"-1","currency":"EUR","variant":"Visit"}`, token, nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid: %d", response.Code)
	}
	response = serveAuthorized(handler, http.MethodDelete, pricePath, "", token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete price: %d", response.Code)
	}
	response = serveAuthorized(handler, http.MethodDelete, entryPath+"/prices/"+entry.Prices[0].ID, "", token, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("delete last price: %d %s", response.Code, response.Body.String())
	}
}

func TestCatalogUpdateRevisionPreconditions(t *testing.T) {
	handler, db, workspace, token := testHandler(t)
	seedCatalogCurrencies(t, db, workspace)
	path := "/api/v1/workspaces/" + workspace + "/catalog"
	created := serveAuthorized(handler, http.MethodPost, path, `{"name":"Alpha","prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, token, nil)
	var entry struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &entry); err != nil || entry.ID == "" || entry.Revision != 1 {
		t.Fatalf("create = %d %s, %v", created.Code, created.Body.String(), err)
	}
	path += "/" + entry.ID
	update := func(name, tag string) *httptest.ResponseRecorder {
		headers := map[string]string{}
		if tag != "" {
			headers["If-Match"] = tag
		}
		return serveAuthorized(handler, http.MethodPut, path, `{"name":"`+name+`"}`, token, headers)
	}
	assertProblem(t, update("Missing", ""), http.StatusPreconditionRequired, "precondition-required")
	assertProblem(t, update("Malformed", "v1"), http.StatusPreconditionFailed, "precondition-failed")
	if r := update("Beta", `"v1"`); r.Code != http.StatusNoContent {
		t.Fatalf("first update: %d %s", r.Code, r.Body.String())
	}
	if r := update("Alpha", `"v2"`); r.Code != http.StatusNoContent {
		t.Fatalf("second update: %d %s", r.Code, r.Body.String())
	}
	assertProblem(t, update("Stale", `"v1"`), http.StatusPreconditionFailed, "precondition-failed")
	list := serveAuthorized(handler, http.MethodGet, strings.TrimSuffix(path, "/"+entry.ID), "", token, nil)
	if !strings.Contains(list.Body.String(), `"name":"Alpha"`) || !strings.Contains(list.Body.String(), `"revision":3`) {
		t.Fatalf("list = %s", list.Body.String())
	}
}

func TestCatalogPricesREST(t *testing.T) {
	handler, db, workspace, token := testHandler(t)
	seedCatalogCurrencies(t, db, workspace)
	path := "/api/v1/workspaces/" + workspace + "/catalog"
	created := serveAuthorized(handler, http.MethodPost, path, `{"name":"Visit","prices":[{"mode":"manual","currency":"USD","variant":"Initial"}]}`, token, nil)
	var entry struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &entry); err != nil || entry.ID == "" {
		t.Fatalf("entry: %s %v", created.Body.String(), err)
	}
	pricePath := path + "/" + entry.ID + "/prices"
	for _, body := range []string{`{"mode":"fixed","amount":"0.12","currency":"EUR","variant":"Visit"}`, `{"mode":"manual","currency":"USD","variant":"Week"}`} {
		response := serveAuthorized(handler, http.MethodPost, pricePath, body, token, nil)
		if response.Code != http.StatusCreated {
			t.Fatalf("price = %d %s", response.Code, response.Body.String())
		}
	}
	for _, body := range []string{`{"mode":"fixed","currency":"USD","variant":"Visit"}`, `{"mode":"fixed","amount":"-1","currency":"USD","variant":"Visit"}`, `{"mode":"fixed","amount":"1.234","currency":"USD","variant":"Visit"}`, `{"mode":"fixed","amount":"1.00","currency":"BTC","variant":"Visit"}`, `{"mode":"manual","currency":"JPY","variant":"Visit"}`, `{"mode":"manual","variant":"Visit"}`, `{"mode":"manual","currency":"USD"}`} {
		response := serveAuthorized(handler, http.MethodPost, pricePath, body, token, nil)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid = %d %s", response.Code, response.Body.String())
		}
	}
	list := serveAuthorized(handler, http.MethodGet, path, "", token, nil)
	if response := serveAuthorized(handler, http.MethodPost, pricePath, `{"mode":"fixed","amount":0.1,"currency":"USD","variant":"Visit"}`, token, nil); response.Code != http.StatusBadRequest {
		t.Fatalf("numeric amount = %d", response.Code)
	}
	if !strings.Contains(list.Body.String(), `"amount":"0.12"`) || !strings.Contains(list.Body.String(), `"mode":"manual"`) {
		t.Fatal(list.Body.String())
	}
}

func seedCatalogCurrencies(t *testing.T, db *sql.DB, workspace string) {
	t.Helper()
	workspaceID := identity.ParseWorkspaceIDOrZero(workspace)
	for _, row := range []struct {
		code            string
		decimals        int
		defaultCurrency bool
	}{{"USD", 2, true}, {"EUR", 2, false}} {
		if _, err := db.Exec(`INSERT INTO workspace_currencies (workspace_id, code, decimals, is_default) VALUES ($1, $2, $3, $4)`, workspaceID, row.code, row.decimals, row.defaultCurrency); err != nil {
			t.Fatal(err)
		}
	}
}
