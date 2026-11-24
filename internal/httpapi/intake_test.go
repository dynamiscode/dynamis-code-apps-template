package httpapi

import (
	"context"
	"net/http"
	"testing"

	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/intake"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestPublicIntakeSubmission(t *testing.T) {
	_, db, _, _ := testHandler(t)
	ctx := context.Background()
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	var user identity.UserID
	var workspace identity.WorkspaceID
	if err := db.QueryRow(`SELECT user_id, workspace_id FROM workspace_members LIMIT 1`).Scan(&user, &workspace); err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, user, workspace, identity.WorkspaceUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engagement.NewService(db, config.SQLite, auth).CreateStage(ctx, actor, workspace, engagement.StageInput{Name: "New", Kind: "open"}, identity.AuditContext{}); err != nil {
		t.Fatal(err)
	}
	svc := intake.NewService(db, config.SQLite, auth)
	token, err := svc.Create(ctx, actor, workspace, "Consent", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	h := &handler{intake: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/intake/{token}", h.submitPublicIntake)
	path := "/api/v1/intake/" + token
	response := serve(mux, http.MethodPost, path, `{"name":"Visitor","email":"visitor@example.com","consent":true}`, "application/json")
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("submit: %d %s", response.Code, response.Body.String())
	}
	response = serve(mux, http.MethodPost, path, `{"website":"spam"}`, "application/json")
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM clients`).Scan(&count); err != nil || response.Code != http.StatusNoContent || count != 1 {
		t.Fatalf("honeypot: status %d, clients %d, error %v", response.Code, count, err)
	}
	response = serve(mux, http.MethodPost, path, `{"name":"Visitor","consent":false}`, "application/json")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid: %d", response.Code)
	}
	response = serve(mux, http.MethodPost, "/api/v1/intake/unknown", `{}`, "application/json")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", response.Code)
	}
}
