package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhookRESTContracts(t *testing.T) {
	handler, _, workspaceID, token := testHandler(t)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	base := "/api/v1/workspaces/" + workspaceID + "/webhooks"
	created := serveAuthorized(handler, http.MethodPost, base,
		`{"name":"items","url":"`+server.URL+`/hook","events":["item.created"]}`,
		token, nil)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"secret"`) || created.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("webhook create = %d, %s", created.Code, created.Body.String())
	}
	var createdValue struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdValue); err != nil || createdValue.ID == "" || createdValue.Secret == "" {
		t.Fatalf("webhook response = %+v, %v", createdValue, err)
	}
	listed := serveAuthorized(handler, http.MethodGet, base, "", token, nil)
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), createdValue.Secret) {
		t.Fatalf("webhook list leaked secret: %d, %s", listed.Code, listed.Body.String())
	}
	rotated := serveAuthorized(handler, http.MethodPost, base+"/"+createdValue.ID+"/secret", "", token, nil)
	if rotated.Code != http.StatusOK || rotated.Header().Get("Cache-Control") != "no-store" || strings.Contains(rotated.Body.String(), createdValue.Secret) {
		t.Fatalf("webhook rotate = %d, %s", rotated.Code, rotated.Body.String())
	}
	item := serveAuthorized(handler, http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/items",
		`{"title":"Webhook item"}`, token, map[string]string{"Idempotency-Key": "webhook-rest-item"})
	if item.Code != http.StatusCreated {
		t.Fatalf("webhook item = %d, %s", item.Code, item.Body.String())
	}
	deliveries := serveAuthorized(handler, http.MethodGet, base+"/"+createdValue.ID+"/deliveries", "", token, nil)
	if deliveries.Code != http.StatusOK || !strings.Contains(deliveries.Body.String(), `"status":"pending"`) {
		t.Fatalf("webhook deliveries = %d, %s", deliveries.Code, deliveries.Body.String())
	}
	deleted := serveAuthorized(handler, http.MethodDelete, base+"/"+createdValue.ID, "", token, nil)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("webhook delete = %d, %s", deleted.Code, deleted.Body.String())
	}
}
