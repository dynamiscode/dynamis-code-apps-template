package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	list := serveSCIM(handler, http.MethodGet, base+`/Users?filter=userName%20eq%20%22person%40example.com%22&count=1`, "", token.Secret, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "hr-1") {
		t.Fatalf("SCIM list = %d, %s", list.Code, list.Body.String())
	}
	updated := serveSCIM(handler, http.MethodPatch, base+"/Users/hr-1", `{"Operations":[{"op":"Replace","path":"active","value":false}]}`, token.Secret, map[string]string{"If-Match": `"v1"`})
	if updated.Code != http.StatusOK || strings.Contains(updated.Body.String(), `"active":true`) {
		t.Fatalf("SCIM deactivate = %d, %s", updated.Code, updated.Body.String())
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
