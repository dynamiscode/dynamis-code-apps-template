package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

func TestSchedulingResourcesREST(t *testing.T) {
	h, _, workspace, token := testHandler(t)
	path := "/api/v1/workspaces/" + workspace + "/scheduling/resources"
	created := serveAuthorized(h, http.MethodPost, path, `{"name":"Room"}`, token, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var resource struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &resource); err != nil || resource.ID == "" {
		t.Fatalf("resource: %s, %v", created.Body.String(), err)
	}
	hoursPath := path + "/" + resource.ID + "/working-hours"
	if response := serveAuthorized(h, http.MethodPut, hoursPath, `{"hours":[{"weekday":1,"start":"09:00","end":"17:00"}]}`, token, nil); response.Code != http.StatusNoContent {
		t.Fatalf("hours: %d %s", response.Code, response.Body.String())
	}
	if response := serveAuthorized(h, http.MethodPut, hoursPath, `{"hours":[{"weekday":1,"start":"17:00","end":"09:00"}]}`, token, nil); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid hours: %d %s", response.Code, response.Body.String())
	}
	updated := serveAuthorized(h, http.MethodPut, path+"/"+resource.ID, `{"name":"Office"}`, token, nil)
	if updated.Code != http.StatusNoContent {
		t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
	}
	zone := serveAuthorized(h, http.MethodPut, "/api/v1/workspaces/"+workspace+"/scheduling/time-zone", `{"timeZone":"America/Bogota"}`, token, nil)
	if zone.Code != http.StatusNoContent {
		t.Fatalf("zone: %d %s", zone.Code, zone.Body.String())
	}
	list := serveAuthorized(h, http.MethodGet, path, "", token, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"Office"`) || !strings.Contains(list.Body.String(), `"timeZone":"America/Bogota"`) || !strings.Contains(list.Body.String(), `"start":"09:00"`) {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}
}

func TestSchedulingBookingsREST(t *testing.T) {
	h, db, workspace, token := testHandler(t)
	ctx := context.Background()
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	wid := identity.ParseWorkspaceIDOrZero(workspace)
	var owner identity.UserID
	if err := db.QueryRowContext(ctx, `SELECT user_id FROM workspace_members WHERE workspace_id = ? AND role = 'owner'`, wid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner, wid, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, actor, wid, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	es := engagement.NewService(db, config.SQLite, auth)
	stage, err := es.CreateStage(ctx, actor, wid, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := es.Create(ctx, actor, wid, engagement.Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/workspaces/" + workspace
	created := serveAuthorized(h, http.MethodPost, base+"/scheduling/resources", `{"name":"Room"}`, token, nil)
	var resource struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &resource); err != nil || resource.ID == "" {
		t.Fatalf("resource: %s %v", created.Body, err)
	}
	path := base + "/scheduling/resources/" + resource.ID
	if response := serveAuthorized(h, http.MethodPut, path+"/working-hours", `{"hours":[{"weekday":1,"start":"09:00","end":"17:00"}]}`, token, nil); response.Code != http.StatusNoContent {
		t.Fatalf("hours: %d %s", response.Code, response.Body)
	}
	created = serveAuthorized(h, http.MethodPost, base+"/catalog", `{"name":"Visit","durationMinutes":60,"prices":[{"mode":"manual","currency":"USD","variant":"Standard"}]}`, token, nil)
	var entry struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &entry); err != nil || entry.ID == "" {
		t.Fatalf("entry: %s %v", created.Body, err)
	}
	body := `{"entryId":"` + entry.ID + `","start":"2026-11-23T09:00:00Z"}`
	response := serveAuthorized(h, http.MethodPost, path+"/bookings", body, token, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("booking: %d %s", response.Code, response.Body)
	}
	var booking struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &booking); err != nil || booking.ID == "" {
		t.Fatalf("booking ID: %s %v", response.Body, err)
	}
	response = serveAuthorized(h, http.MethodPut, base+"/scheduling/bookings/"+booking.ID+"/engagement", `{}`, token, nil)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing engagement: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodPut, base+"/scheduling/bookings/"+booking.ID+"/engagement", `{"engagementId":"`+eng.ID.String()+`"}`, token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("set engagement: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodGet, path+"/bookings", "", token, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), eng.ID.String()) {
		t.Fatalf("linked booking: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodPut, base+"/scheduling/bookings/"+booking.ID+"/engagement", `{"engagementId":null}`, token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("clear engagement: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodPost, path+"/bookings", body, token, nil)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "overlap") {
		t.Fatalf("overlap: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodGet, path+"/bookings", "", token, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Visit") {
		t.Fatalf("schedule: %d %s", response.Code, response.Body)
	}
	statusPath := path + "/bookings/" + booking.ID + "/status"
	movePath := path + "/bookings/" + booking.ID + "/move"
	response = serveAuthorized(h, http.MethodPut, movePath, `{"start":"2026-11-23T10:00:00Z"}`, token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("move: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodGet, path+"/bookings/"+booking.ID+"/history", "", token, nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "oldStart") {
		t.Fatalf("history: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodPut, statusPath, `{"status":"cancelled"}`, token, nil)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodPut, statusPath, `{"status":"done"}`, token, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("terminal: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodPost, path+"/bookings", body, token, nil)
	if response.Code != http.StatusCreated {
		t.Fatalf("freed slot: %d %s", response.Code, response.Body)
	}
	response = serveAuthorized(h, http.MethodDelete, base+"/catalog/"+entry.ID, "", token, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("delete booked entry: %d %s", response.Code, response.Body)
	}
}

func TestSchedulingRESTReadOnlyRoles(t *testing.T) {
	h, db, workspace, ownerToken := testHandler(t)
	ctx := context.Background()
	auth, err := identity.NewService(db, config.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	wid := identity.ParseWorkspaceIDOrZero(workspace)
	var owner identity.UserID
	if err := db.QueryRowContext(ctx, `SELECT user_id FROM workspace_members WHERE workspace_id = ? AND role = 'owner'`, wid).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authorize(ctx, owner, wid, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + workspace + "/scheduling/resources"
	created := serveAuthorized(h, http.MethodPost, path, `{"name":"Room"}`, ownerToken, nil)
	var resource struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &resource); err != nil || resource.ID == "" {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	if response := serveAuthorized(h, http.MethodPut, path+"/"+resource.ID+"/working-hours", `{"hours":[{"weekday":1,"start":"09:00","end":"17:00"}]}`, ownerToken, nil); response.Code != http.StatusNoContent {
		t.Fatalf("seed hours: %d %s", response.Code, response.Body.String())
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, actor, wid, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	engagements := engagement.NewService(db, config.SQLite, auth)
	stage, err := engagements.CreateStage(ctx, actor, wid, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := engagements.Create(ctx, actor, wid, engagement.Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v1/workspaces/" + workspace
	entryResponse := serveAuthorized(h, http.MethodPost, base+"/catalog", `{"name":"Visit","durationMinutes":60,"prices":[{"mode":"manual","currency":"USD","variant":"Standard"}]}`, ownerToken, nil)
	var entry struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(entryResponse.Body.Bytes(), &entry); entryResponse.Code != http.StatusCreated || err != nil || entry.ID == "" {
		t.Fatalf("seed catalog = %d %s, %v", entryResponse.Code, entryResponse.Body.String(), err)
	}
	bookingsPath := path + "/" + resource.ID + "/bookings"
	bookingResponse := serveAuthorized(h, http.MethodPost, bookingsPath, `{"entryId":"`+entry.ID+`","start":"2026-11-23T09:00:00Z"}`, ownerToken, nil)
	var booking struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bookingResponse.Body.Bytes(), &booking); bookingResponse.Code != http.StatusCreated || err != nil || booking.ID == "" {
		t.Fatalf("seed booking = %d %s, %v", bookingResponse.Code, bookingResponse.Body.String(), err)
	}
	for _, role := range []identity.Role{identity.Member, identity.Viewer} {
		inv, err := auth.CreateInvitation(ctx, actor, string(role)+"@example.com", role, time.Hour, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		user, _, err := auth.CreateInvitedLocalUser(ctx, inv.Secret, "long-enough-password", identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		principal, err := auth.Authorize(ctx, user, wid, identity.ResourcesRead)
		if err != nil {
			t.Fatal(err)
		}
		scopes := []identity.Permission{identity.ResourcesRead}
		if role == identity.Member {
			scopes = append(scopes, identity.ResourcesWrite)
		}
		token, err := auth.CreateAPIToken(ctx, principal, "scheduling", scopes, nil, identity.AuditContext{})
		if err != nil {
			t.Fatal(err)
		}
		if response := serveAuthorized(h, http.MethodGet, path, "", token.Secret, nil); response.Code != http.StatusOK {
			t.Fatalf("%s list: %d", role, response.Code)
		}
		if response := serveAuthorized(h, http.MethodPost, path, `{"name":"Denied"}`, token.Secret, nil); response.Code != http.StatusForbidden {
			t.Fatalf("%s create: %d", role, response.Code)
		}
		if response := serveAuthorized(h, http.MethodPut, path+"/"+resource.ID+"/working-hours", `{"hours":[]}`, token.Secret, nil); response.Code != http.StatusForbidden {
			t.Fatalf("%s hours: %d", role, response.Code)
		}
		if role == identity.Viewer {
			before := serveAuthorized(h, http.MethodGet, bookingsPath, "", ownerToken, nil)
			if before.Code != http.StatusOK {
				t.Fatalf("bookings before denied writes: %d %s", before.Code, before.Body.String())
			}
			if response := serveAuthorized(h, http.MethodPut, bookingsPath+"/"+booking.ID+"/move", `{"start":"2026-11-23T10:00:00Z"}`, token.Secret, nil); response.Code != http.StatusForbidden {
				t.Fatalf("viewer move: %d", response.Code)
			}
			if response := serveAuthorized(h, http.MethodPut, base+"/scheduling/bookings/"+booking.ID+"/engagement", `{"engagementId":"`+eng.ID.String()+`"}`, token.Secret, nil); response.Code != http.StatusForbidden {
				t.Fatalf("viewer engagement: %d", response.Code)
			}
			if response := serveAuthorized(h, http.MethodPut, bookingsPath+"/"+booking.ID+"/status", `{"status":"done"}`, token.Secret, nil); response.Code != http.StatusForbidden {
				t.Fatalf("viewer status: %d", response.Code)
			}
			after := serveAuthorized(h, http.MethodGet, bookingsPath, "", ownerToken, nil)
			if after.Code != http.StatusOK || after.Body.String() != before.Body.String() {
				t.Fatalf("viewer denied writes changed bookings: before %s, after %d %s", before.Body.String(), after.Code, after.Body.String())
			}
		}
		after := serveAuthorized(h, http.MethodGet, path, "", ownerToken, nil)
		if after.Code != http.StatusOK || !strings.Contains(after.Body.String(), `"name":"Room"`) || strings.Contains(after.Body.String(), `"name":"Denied"`) || !strings.Contains(after.Body.String(), `"start":"09:00"`) {
			t.Fatalf("%s denied writes changed resources: %d %s", role, after.Code, after.Body.String())
		}
	}
}
