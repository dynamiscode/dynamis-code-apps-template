package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/scheduling"
)

func TestSchedulingBookingWeb(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	s := scheduling.NewService(db, auth)
	c := catalog.NewService(db, auth, nil)
	h.EnableScheduling(s)
	h.EnableCatalog(c)
	es := engagement.NewService(db, config.SQLite, auth)
	h.EnableEngagement(es)
	h.EnableClients(clients.NewService(db, config.SQLite, auth, 50), listprefs.NewService(db, config.SQLite, auth))
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.ResourcesWrite)
	if err != nil {
		t.Fatal(err)
	}
	client, err := clients.NewService(db, config.SQLite, auth, 50).Create(ctx, actor, workspace, clients.Input{DisplayName: "Client"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	stage, err := es.CreateStage(ctx, actor, workspace, engagement.StageInput{Name: "Open", Kind: "open"}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := es.Create(ctx, actor, workspace, engagement.Input{Name: "Engagement", ClientID: client.ID, StageID: stage.ID}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Create(ctx, actor, workspace, "Room", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetWorkingHours(ctx, actor, workspace, r.ID, []scheduling.WorkingHours{{Weekday: 1, Start: "09:00", End: "17:00"}}); err != nil {
		t.Fatal(err)
	}
	duration := 60
	e, err := c.Create(ctx, actor, workspace, "Visit", &duration, []catalog.PriceOption{{Mode: "manual", Currency: "USD", Variant: "Standard"}})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspace.String() + "/scheduling"
	response := request(h.Routes(), http.MethodPost, path+"/"+r.ID.String()+"/bookings", url.Values{"csrf": {session.CSRFSecret}, "entry_id": {e.ID.String()}, "start": {"2026-11-23T09:00"}, "engagement_id": {eng.ID.String()}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("book: %d %s", response.Code, response.Body)
	}
	page := request(h.Routes(), http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Visit") || !strings.Contains(page.Body.String(), "2026-11-23 09:00") || !strings.Contains(page.Body.String(), eng.ID.String()) {
		t.Fatalf("schedule: %d %s", page.Code, page.Body)
	}
	for _, want := range []string{
		`for="scheduling-create-name"`, `id="scheduling-create-name"`,
		`for="scheduling-create-user"`, `id="scheduling-create-user"`,
		`for="booking-entry-` + r.ID.String() + `"`, `id="booking-entry-` + r.ID.String() + `"`,
		`for="booking-new-start-` + r.ID.String() + `"`, `type="datetime-local" name="start"`,
		`name="entry_id" required data-entry-picker`, `name="engagement_id" data-entry-picker`,
	} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("scheduling form missing %q", want)
		}
	}
	clientPage := request(h.Routes(), http.MethodGet, "/workspaces/"+workspace.String()+"/clients/"+client.ID.String(), nil, cookies, nil)
	engagementDetailPath := "/workspaces/" + workspace.String() + "/clients/" + client.ID.String() + "/engagements/" + eng.ID.String()
	tileStart := strings.Index(clientPage.Body.String(), `<ul class="detail-engagements`)
	tileEnd := -1
	if tileStart >= 0 {
		tileEnd = strings.Index(clientPage.Body.String()[tileStart:], `</ul>`)
	}
	if clientPage.Code != http.StatusOK || tileEnd < 0 || !strings.Contains(clientPage.Body.String()[tileStart:tileStart+tileEnd], engagementDetailPath) || strings.Contains(clientPage.Body.String()[tileStart:tileStart+tileEnd], "Visit") {
		t.Errorf("client engagement tile: %d %s", clientPage.Code, clientPage.Body)
	}
	engagementPage := request(h.Routes(), http.MethodGet, engagementDetailPath, nil, cookies, nil)
	if engagementPage.Code != http.StatusOK || !strings.Contains(engagementPage.Body.String(), "Visit") || !strings.Contains(engagementPage.Body.String(), "2026-11-23 09:00") {
		t.Errorf("engagement bookings: %d %s", engagementPage.Code, engagementPage.Body)
	}
	bookings, _, err := s.Schedule(ctx, actor, workspace, r.ID)
	if err != nil || len(bookings) != 1 {
		t.Fatalf("bookings: %v %v", bookings, err)
	}
	statusPath := path + "/" + r.ID.String() + "/bookings/" + bookings[0].ID.String() + "/status"
	movePath := path + "/" + r.ID.String() + "/bookings/" + bookings[0].ID.String() + "/move"
	if !strings.Contains(page.Body.String(), `data-inline-saved="2026-11-23T09:00"`) {
		t.Fatalf("missing inline booking start: %s", page.Body)
	}
	moveFormStart := strings.Index(page.Body.String(), `action="`+movePath+`"`)
	if moveFormStart < 0 {
		t.Fatal("missing move form")
	}
	moveFormEnd := strings.Index(page.Body.String()[moveFormStart:], "</form>")
	if moveFormEnd < 0 {
		t.Fatal("unterminated move form")
	}
	moveForm := page.Body.String()[moveFormStart : moveFormStart+moveFormEnd]
	for _, want := range []string{`type="datetime-local"`, `name="start"`, `value="2026-11-23T09:00"`, `data-inline-input`, `data-confirm="`, `type="submit"`, `data-inline-cancel`} {
		if !strings.Contains(moveForm, want) {
			t.Fatalf("move form missing %q: %s", want, moveForm)
		}
	}
	engagementPath := path + "/bookings/" + bookings[0].ID.String() + "/engagement"
	engagementFormStart := strings.Index(page.Body.String(), `action="`+engagementPath+`"`)
	if engagementFormStart < 0 {
		t.Fatal("missing engagement form")
	}
	engagementFormEnd := strings.Index(page.Body.String()[engagementFormStart:], "</form>")
	if engagementFormEnd < 0 {
		t.Fatal("unterminated engagement form")
	}
	engagementForm := page.Body.String()[engagementFormStart : engagementFormStart+engagementFormEnd]
	for _, want := range []string{`data-confirm-select`, `name="engagement_id"`, `value="` + eng.ID.String() + `" selected`, `data-confirm-select-dialog`, `data-confirm-select-fallback`, `type="submit"`} {
		if !strings.Contains(engagementForm, want) {
			t.Fatalf("engagement form missing %q: %s", want, engagementForm)
		}
	}
	response = request(h.Routes(), http.MethodPost, engagementPath, url.Values{"csrf": {session.CSRFSecret}, "engagement_id": {""}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("clear engagement without JavaScript: %d %s", response.Code, response.Body)
	}
	response = request(h.Routes(), http.MethodPost, engagementPath, url.Values{"csrf": {session.CSRFSecret}, "engagement_id": {eng.ID.String()}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("restore engagement without JavaScript: %d %s", response.Code, response.Body)
	}
	invalidMove := request(h.Routes(), http.MethodPost, movePath, url.Values{"csrf": {session.CSRFSecret}, "start": {"2026-11-23T08:00"}}, cookies, nil)
	if invalidMove.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidMove.Body.String(), "outside working hours") || !strings.Contains(invalidMove.Body.String(), `value="2026-11-23T08:00"`) {
		t.Fatalf("invalid move: %d %s", invalidMove.Code, invalidMove.Body)
	}
	response = request(h.Routes(), http.MethodPost, movePath, url.Values{"csrf": {session.CSRFSecret}, "start": {"2026-11-23T10:00"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("move: %d %s", response.Code, response.Body)
	}
	page = request(h.Routes(), http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "2026-11-23 10:00") || !strings.Contains(page.Body.String(), "2026-11-23 09:00") {
		t.Fatalf("history: %s", page.Body)
	}
	if !strings.Contains(page.Body.String(), statusPath) {
		t.Fatal("missing outcome controls")
	}
	if err := s.SetTimeZone(ctx, actor, workspace, "America/New_York"); err != nil {
		t.Fatal(err)
	}
	page = request(h.Routes(), http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "America/New_York") || !strings.Contains(page.Body.String(), `data-inline-saved="2026-11-23T05:00"`) {
		t.Fatalf("missing workspace time zone: %s", page.Body)
	}
	engagementPage = request(h.Routes(), http.MethodGet, engagementDetailPath, nil, cookies, nil)
	if engagementPage.Code != http.StatusOK || !strings.Contains(engagementPage.Body.String(), "2026-11-23 05:00") || !strings.Contains(engagementPage.Body.String(), "America/New_York") {
		t.Errorf("engagement bookings time zone: %d %s", engagementPage.Code, engagementPage.Body)
	}
	invalidMove = request(h.Routes(), http.MethodPost, movePath, url.Values{"csrf": {session.CSRFSecret}, "start": {"2026-03-08T02:30"}}, cookies, nil)
	if invalidMove.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidMove.Body.String(), `value="2026-03-08T02:30"`) || !strings.Contains(invalidMove.Body.String(), `data-inline-dirty-on-load`) {
		t.Fatalf("DST gap move draft: %d %s", invalidMove.Code, invalidMove.Body)
	}
	response = request(h.Routes(), http.MethodPost, statusPath, url.Values{"csrf": {session.CSRFSecret}, "status": {"no-show"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("outcome: %d %s", response.Code, response.Body)
	}
	response = request(h.Routes(), http.MethodPost, statusPath, url.Values{"csrf": {session.CSRFSecret}, "status": {"done"}}, cookies, nil)
	if response.Code != http.StatusConflict {
		t.Fatalf("terminal: %d", response.Code)
	}
}

func TestSchedulingResourcesWeb(t *testing.T) {
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableScheduling(scheduling.NewService(db, auth))
	routes := h.Routes()
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspace.String() + "/scheduling"
	response := request(routes, http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Room"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create: %d %s", response.Code, response.Body.String())
	}
	page := request(routes, http.MethodGet, path, nil, cookies, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Room") {
		t.Fatalf("list: %d %s", page.Code, page.Body.String())
	}
	actor, err := auth.Authorize(context.Background(), owner.UserID, workspace, identity.ResourcesRead)
	if err != nil {
		t.Fatal(err)
	}
	resources, _, err := scheduling.NewService(db, auth).List(context.Background(), actor, workspace)
	if err != nil {
		t.Fatal(err)
	}
	hoursPath := path + "/" + resources[0].ID.String() + "/working-hours"
	response = request(routes, http.MethodPost, hoursPath, url.Values{"csrf": {session.CSRFSecret}, "weekday": {"1", "1"}, "start": {"09:00", "13:00"}, "end": {"12:00", "17:00"}}, cookies, nil)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("hours: %d %s", response.Code, response.Body.String())
	}
	page = request(routes, http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "09:00") {
		t.Fatalf("hours missing: %s", page.Body.String())
	}
	invalidHours := request(routes, http.MethodPost, hoursPath, url.Values{"csrf": {session.CSRFSecret}, "weekday": {"1"}, "start": {"17:00"}, "end": {"09:00"}}, cookies, nil)
	if invalidHours.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidHours.Body.String(), `value="17:00"`) || !strings.Contains(invalidHours.Body.String(), "non-overlapping") {
		t.Fatalf("invalid hours: %d %s", invalidHours.Code, invalidHours.Body.String())
	}
	invalid := request(routes, http.MethodPost, path+"/time-zone", url.Values{"csrf": {session.CSRFSecret}, "time_zone": {"Not/AZone"}}, cookies, nil)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), `value="Not/AZone"`) || !strings.Contains(invalid.Body.String(), "Enter a valid IANA time zone") {
		t.Fatalf("invalid zone: %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestSchedulingReadOnlyWeb(t *testing.T) {
	ctx := context.Background()
	h, auth, workspace, owner, db := testWebWithMFARawDB(t, 2, false)
	h.EnableScheduling(scheduling.NewService(db, auth))
	actor, err := auth.Authorize(ctx, owner.UserID, workspace, identity.InvitationsManage)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := scheduling.NewService(db, auth).Create(ctx, actor, workspace, "Room", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduling.NewService(db, auth).SetWorkingHours(ctx, actor, workspace, resource.ID, []scheduling.WorkingHours{{Weekday: 1, Start: "09:00", End: "17:00"}}); err != nil {
		t.Fatal(err)
	}
	duration := 60
	entry, err := catalog.NewService(db, auth, nil).Create(ctx, actor, workspace, "Visit", &duration, []catalog.PriceOption{{Mode: "manual", Currency: "USD", Variant: "Standard"}})
	if err != nil {
		t.Fatal(err)
	}
	booking, err := scheduling.NewService(db, auth).Book(ctx, actor, workspace, resource.ID, entry.ID, time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduling.NewService(db, auth).MoveBooking(ctx, actor, workspace, resource.ID, booking.ID, time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	inv, err := auth.CreateInvitation(ctx, actor, "viewer@example.com", identity.Viewer, time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := auth.CreateInvitedLocalUser(ctx, inv.Secret, "long-enough-password", identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(ctx, user, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	path := "/workspaces/" + workspace.String() + "/scheduling"
	page := request(h.Routes(), http.MethodGet, path, nil, cookies, nil)
	if !strings.Contains(page.Body.String(), "Move history") || !strings.Contains(page.Body.String(), "2026-11-23 09:00") {
		t.Fatalf("viewer history: %s", page.Body)
	}
	statusPath := path + "/" + resource.ID.String() + "/bookings/" + booking.ID.String() + "/status"
	movePath := path + "/" + resource.ID.String() + "/bookings/" + booking.ID.String() + "/move"
	if strings.Contains(page.Body.String(), movePath) {
		t.Fatal("viewer saw move control")
	}
	if response := request(h.Routes(), http.MethodPost, movePath, url.Values{"csrf": {session.CSRFSecret}, "start": {"2026-11-23T10:00"}}, cookies, nil); response.Code != http.StatusForbidden {
		t.Fatalf("viewer move: %d", response.Code)
	}
	if strings.Contains(page.Body.String(), statusPath) {
		t.Fatal("viewer saw outcome controls")
	}
	if response := request(h.Routes(), http.MethodPost, statusPath, url.Values{"csrf": {session.CSRFSecret}, "status": {"done"}}, cookies, nil); response.Code != http.StatusForbidden {
		t.Fatalf("viewer outcome: %d", response.Code)
	}
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Monday") || !strings.Contains(page.Body.String(), "09:00") || strings.Contains(page.Body.String(), `name="weekday"`) || strings.Contains(page.Body.String(), `name="name"`) || strings.Contains(page.Body.String(), `name="time_zone"`) {
		t.Fatalf("viewer page: %d %s", page.Code, page.Body.String())
	}
	response := request(h.Routes(), http.MethodPost, path, url.Values{"csrf": {session.CSRFSecret}, "name": {"Denied"}}, cookies, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer create: %d", response.Code)
	}
	response = request(h.Routes(), http.MethodPost, path+"/"+resource.ID.String()+"/working-hours", url.Values{"csrf": {session.CSRFSecret}}, cookies, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer hours: %d", response.Code)
	}
}
