package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/clients"
)

type testOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func defineChoiceFieldOverREST(t *testing.T, handler http.Handler, fieldBase, token, label string) string {
	t.Helper()
	defined := serveAuthorized(handler, http.MethodPost, fieldBase, `{"label":"`+label+`","type":"single_choice"}`, token, nil)
	if defined.Code != http.StatusCreated {
		t.Fatalf("define %s = %d %s", label, defined.Code, defined.Body.String())
	}
	var field struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(defined.Body.Bytes(), &field); err != nil {
		t.Fatal(err)
	}
	return field.ID
}

func TestClientChoiceOptionsOverREST(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	clientBase := "/api/v1/workspaces/" + workspaceID + "/clients"
	fieldBase := "/api/v1/workspaces/" + workspaceID + "/client-fields"
	fieldID := defineChoiceFieldOverREST(t, handler, fieldBase, token, "Tier")
	optionBase := fieldBase + "/" + fieldID + "/options"

	added := serveAuthorized(handler, http.MethodPost, optionBase, `{"label":"Gold"}`, token, nil)
	if added.Code != http.StatusCreated || added.Header().Get("Location") == "" {
		t.Fatalf("add option = %d headers %v %s", added.Code, added.Header(), added.Body.String())
	}
	var gold testOption
	if err := json.Unmarshal(added.Body.Bytes(), &gold); err != nil {
		t.Fatal(err)
	}
	if clients.ParseOptionIDOrZero(gold.ID) == 0 || gold.Label != "Gold" {
		t.Fatalf("option body = %+v; want a choice_field_option short ID and Gold", gold)
	}
	if strings.Contains(added.Body.String(), `"slot"`) {
		t.Fatalf("slot leaks into the response: %s", added.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPost, optionBase, `{"label":"Gold","id":7}`, token, nil), http.StatusBadRequest, "invalid-request")
	assertProblem(t, serveAuthorized(handler, http.MethodPost, optionBase, `{"label":"   "}`, token, nil), http.StatusBadRequest, "invalid-request")
	if listed := serveAuthorized(handler, http.MethodGet, optionBase, "", token, nil); listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), gold.ID) {
		t.Fatalf("list options = %d %s", listed.Code, listed.Body.String())
	}

	otherFieldID := defineChoiceFieldOverREST(t, handler, fieldBase, token, "Other")
	elsewhere := serveAuthorized(handler, http.MethodPost, fieldBase+"/"+otherFieldID+"/options", `{"label":"Elsewhere"}`, token, nil)
	var otherOption testOption
	if err := json.Unmarshal(elsewhere.Body.Bytes(), &otherOption); err != nil || otherOption.ID == "" {
		t.Fatalf("other option = %d %s, %v", elsewhere.Code, elsewhere.Body.String(), err)
	}

	created := serveAuthorized(handler, http.MethodPost, clientBase, `{"displayName":"Ada"}`, token, nil)
	var client struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &client); err != nil {
		t.Fatal(err)
	}
	valuePath := clientBase + "/" + client.ID + "/values/" + fieldID
	ifMatch := map[string]string{"If-Match": `"v1"`}
	for _, body := range []string{
		`{"value":"` + otherOption.ID + `"}`,
		`{"value":"Gold"}`,
		`{"value":"1"}`,
	} {
		assertProblem(t, serveAuthorized(handler, http.MethodPut, valuePath, body, token, ifMatch), http.StatusBadRequest, "invalid-request")
	}
	set := serveAuthorized(handler, http.MethodPut, valuePath, `{"value":"`+gold.ID+`"}`, token, ifMatch)
	if set.Code != http.StatusOK || set.Header().Get("ETag") != `"v2"` {
		t.Fatalf("set option value = %d headers %v %s", set.Code, set.Header(), set.Body.String())
	}

	assertProblem(t, serveAuthorized(handler, http.MethodDelete, optionBase+"/"+clients.OptionID(999999).String(), "", token, nil), http.StatusNotFound, "not-found")
	assertProblem(t, serveAuthorized(handler, http.MethodDelete, optionBase+"/1", "", token, nil), http.StatusNotFound, "not-found")
	deleted := serveAuthorized(handler, http.MethodDelete, optionBase+"/"+gold.ID, "", token, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete option = %d %s", deleted.Code, deleted.Body.String())
	}
	assertProblem(t, serveAuthorized(handler, http.MethodDelete, optionBase+"/"+gold.ID, "", token, nil), http.StatusNotFound, "not-found")

	values := serveAuthorized(handler, http.MethodGet, clientBase+"/"+client.ID+"/values", "", token, nil)
	if values.Code != http.StatusOK || strings.Contains(values.Body.String(), gold.ID) {
		t.Fatalf("client values after option delete = %d %s", values.Code, values.Body.String())
	}
	clientNow := serveAuthorized(handler, http.MethodGet, clientBase+"/"+client.ID, "", token, nil)
	if clientNow.Code != http.StatusOK || clientNow.Header().Get("ETag") != `"v3"` {
		t.Fatalf("client after option delete = %d, ETag %q, want 200 and v3", clientNow.Code, clientNow.Header().Get("ETag"))
	}
	if listed := serveAuthorized(handler, http.MethodGet, optionBase, "", token, nil); listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), gold.ID) {
		t.Fatalf("options after delete = %d %s", listed.Code, listed.Body.String())
	}
}

func TestClientChoiceOptionLimitOverREST(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	fieldBase := "/api/v1/workspaces/" + workspaceID + "/client-fields"
	fieldID := defineChoiceFieldOverREST(t, handler, fieldBase, token, "Tier")
	optionBase := fieldBase + "/" + fieldID + "/options"
	for i := 1; i <= 100; i++ {
		response := serveAuthorized(handler, http.MethodPost, optionBase, `{"label":"`+fmt.Sprintf("Option %03d", i)+`"}`, token, nil)
		if response.Code != http.StatusCreated {
			t.Fatalf("option %d = %d %s", i, response.Code, response.Body.String())
		}
	}
	assertProblem(t, serveAuthorized(handler, http.MethodPost, optionBase, `{"label":"Over"}`, token, nil), http.StatusConflict, "resource-limit")
}
