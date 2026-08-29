package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestSCIMRESTContract(t *testing.T) {
	handler, _, workspaceID, apiToken := testHandler(t)
	management := serveAuthorized(handler, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/scim-token", "", apiToken, nil)
	if management.Code != http.StatusCreated || management.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("SCIM token = %d, %s", management.Code, management.Body.String())
	}
	var token struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(management.Body.Bytes(), &token); err != nil || token.Secret == "" {
		t.Fatalf("SCIM token body = %s", management.Body.String())
	}
	base := "/scim/v2/" + workspaceID
	created := serveSCIM(handler, http.MethodPost, base+"/Users", `{"externalId":"hr-1","userName":"person@example.com","emails":[{"value":"person@example.com","primary":true}]}`, token.Secret, nil)
	if created.Code != http.StatusCreated || created.Header().Get("ETag") != `"v1"` {
		t.Fatalf("SCIM create = %d, %s", created.Code, created.Body.String())
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &user); err != nil || user.ID != "hr-1" {
		t.Fatalf("SCIM user = %s", created.Body.String())
	}
	special := serveSCIM(handler, http.MethodPost, base+"/Users", `{"externalId":"hr?2#x","userName":"special@example.com"}`, token.Secret, nil)
	if special.Code != http.StatusCreated || !strings.HasSuffix(special.Header().Get("Location"), "/hr%3F2%23x") {
		t.Fatalf("SCIM escaped location = %d, %s", special.Code, special.Header().Get("Location"))
	}
	orderedEmail := serveSCIM(handler, http.MethodPost, base+"/Users", `{"externalId":"ordered-email","userName":"ordered@example.com","emails":[{"value":"alias@example.com"},{"value":"ordered@example.com","primary":true}]}`, token.Secret, nil)
	if orderedEmail.Code != http.StatusCreated {
		t.Fatalf("SCIM primary email ordering = %d, %s", orderedEmail.Code, orderedEmail.Body.String())
	}
	list := serveSCIM(handler, http.MethodGet, base+`/Users?filter=userName%20eq%20%22person%40example.com%22&count=1`, "", token.Secret, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "hr-1") {
		t.Fatalf("SCIM list = %d, %s", list.Code, list.Body.String())
	}
	groups := serveSCIM(handler, http.MethodGet, base+"/Groups", "", token.Secret, nil)
	if groups.Code != http.StatusOK || strings.Contains(groups.Body.String(), `"created":""`) || strings.Contains(groups.Body.String(), `"lastModified":""`) {
		t.Fatalf("SCIM groups metadata = %d, %s", groups.Code, groups.Body.String())
	}
	updated := serveSCIM(handler, http.MethodPatch, base+"/Users/hr-1", `{"Operations":[{"op":"Replace","path":"active","value":false}]}`, token.Secret, map[string]string{"If-Match": `"v1"`})
	if updated.Code != http.StatusOK || strings.Contains(updated.Body.String(), `"active":true`) {
		t.Fatalf("SCIM deactivate = %d, %s", updated.Code, updated.Body.String())
	}
}

func TestSCIMProblemHidesUnexpectedErrors(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/scim/v2/workspace/Users", nil)
	response := httptest.NewRecorder()
	(&handler{}).scimProblem(response, request, errors.New("database unavailable"))
	if response.Code != http.StatusInternalServerError || response.Header().Get("Content-Type") != "application/scim+json" {
		t.Fatalf("SCIM unexpected error = %d, %v", response.Code, response.Header())
	}
	if strings.Contains(response.Body.String(), "database unavailable") || !strings.Contains(response.Body.String(), `"status":"500"`) {
		t.Fatalf("SCIM unexpected error body = %s", response.Body.String())
	}
}

func TestSCIMDemotedTokenReturnsForbidden(t *testing.T) {
	handler, db, workspaceID, apiToken := testHandler(t)
	management := serveAuthorized(handler, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/scim-token", "", apiToken, nil)
	if management.Code != http.StatusCreated {
		t.Fatalf("SCIM token = %d, %s", management.Code, management.Body.String())
	}
	var token struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(management.Body.Bytes(), &token); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE workspace_members SET role = ? WHERE workspace_id = ?", "member", workspaceID); err != nil {
		t.Fatal(err)
	}
	response := serveSCIM(handler, http.MethodGet, "/scim/v2/"+workspaceID+"/Users", "", token.Secret, nil)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"status":"403"`) {
		t.Fatalf("demoted SCIM token = %d, %s", response.Code, response.Body.String())
	}
}

func TestSCIMFilterAcceptsQuotedSpaces(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, `/scim/v2/workspace/Users?filter=externalId%20eq%20%22external%20id%22`, nil)
	field, value, ok := scimFilter(request)
	if !ok || field != "externalId" || value != "external id" {
		t.Fatalf("SCIM filter = %q, %q, %v", field, value, ok)
	}
}

func TestDecodeSCIMJSONRejectsTrailingData(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/scim/v2/workspace/Users", strings.NewReader(`{"userName":"person@example.com"} garbage`))
	request.Header.Set("Content-Type", "application/scim+json")
	var input scimUserRequest
	if err := decodeSCIMJSON(request, &input); err == nil {
		t.Fatal("SCIM decoder accepted trailing data")
	}
}

func TestSCIMUserPatchRejectsNullActive(t *testing.T) {
	_, err := scimUserPatch([]scimPatchOperation{{
		Op: "replace", Path: "active", Value: json.RawMessage("null"),
	}})
	if !errors.Is(err, identity.ErrSCIMInvalid) {
		t.Fatalf("SCIM null active error = %v", err)
	}
}

func TestSCIMUserPatchRejectsDuplicateActive(t *testing.T) {
	_, err := scimUserPatch([]scimPatchOperation{
		{Op: "replace", Path: "active", Value: json.RawMessage("false")},
		{Op: "replace", Path: "active", Value: json.RawMessage("true")},
	})
	if !errors.Is(err, identity.ErrSCIMInvalid) {
		t.Fatalf("SCIM duplicate active error = %v", err)
	}
}

func TestSCIMGroupPatchAcceptsFilteredRemoval(t *testing.T) {
	operations, err := scimGroupOperations([]scimPatchOperation{{
		Op: "remove", Path: `members[value eq "external id"]`,
	}})
	if err != nil || len(operations) != 1 || len(operations[0].Members) != 1 || operations[0].Members[0] != "external id" {
		t.Fatalf("SCIM filtered group removal = %+v, %v", operations, err)
	}
}

func serveSCIM(handler http.Handler, method, target, body, token string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/scim+json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
