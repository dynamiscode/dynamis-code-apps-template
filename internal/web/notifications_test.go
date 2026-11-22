package web

import (
	"bufio"
	"context"
	"example.com/dynamis-code/apps-template/internal/identity"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNotificationSSELifetimeReconnectReplaysAfterLastEvent(t *testing.T) {
	handler, auth, workspaceID, owner := testWebConfigured(t, 4, false, func(handler *Handler) {
		handler.cfg.SSEPollInterval = 5 * time.Millisecond
		handler.cfg.SSEMaxLifetime = 80 * time.Millisecond
	})
	existing, err := auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "Existing", Body: "Existing body",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	list := request(handler, http.MethodGet, "/notifications", nil, cookies, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("notifications list = %d, %s", list.Code, list.Body.String())
	}
	for _, marker := range []string{
		`data-notification-cursor="` + existing.ID.String() + `"`,
		`<ul class="list notification-list">`, `<li class="list-row notification-card card grid grid-cols-[1fr_auto] items-start gap-4 p-5 max-md:grid-cols-1">`,
		`<span class="badge badge-primary">Unread</span>`,
		`<button class="btn btn-secondary" type="submit">Mark as read</button>`,
		`name="csrf" value="` + session.CSRFSecret + `"`,
	} {
		if !strings.Contains(list.Body.String(), marker) {
			t.Errorf("notifications list missing %q: %s", marker, list.Body.String())
		}
	}
	invalidCSRF := request(handler, http.MethodPost, "/notifications/"+existing.ID.String(), url.Values{"csrf": {"invalid"}}, cookies, nil)
	if invalidCSRF.Code != http.StatusForbidden {
		t.Fatalf("notification mutation without valid CSRF = %d", invalidCSRF.Code)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/notifications/events", nil)
	addCookies(request, cookies)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	created, err := auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "First live", Body: "First live body",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	event := readUntil(t, reader, "event: notification.created") + readUntil(t, reader, "\n\n")
	if !strings.Contains(event, created.ID.String()) {
		t.Fatalf("first notification event = %q", event)
	}
	if closeEvent := readUntil(t, reader, "event: close") + readUntil(t, reader, "\n\n"); !strings.Contains(closeEvent, `"reason":"lifetime"`) {
		t.Fatalf("lifetime close event = %q", closeEvent)
	}
	response.Body.Close()

	created, err = auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "Replayed", Body: "Replayed body",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	reconnect, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/notifications/events", nil)
	reconnect.Header.Set("Last-Event-ID", sseID(event))
	addCookies(reconnect, cookies)
	reconnected, err := server.Client().Do(reconnect)
	if err != nil {
		t.Fatal(err)
	}
	defer reconnected.Body.Close()
	replayReader := bufio.NewReader(reconnected.Body)
	replayed := readUntil(t, replayReader, "event: notification.created") + readUntil(t, replayReader, "\n\n")
	if !strings.Contains(replayed, created.ID.String()) || !strings.Contains(replayed, "Replayed body") {
		t.Fatalf("reconnected notification = %q", replayed)
	}
}

func TestNotificationSSELifetimeReconnectReplaysWithoutPriorEvent(t *testing.T) {
	handler, auth, workspaceID, owner := testWebConfigured(t, 4, false, func(handler *Handler) {
		handler.cfg.SSEPollInterval = 5 * time.Millisecond
		handler.cfg.SSEMaxLifetime = 80 * time.Millisecond
	})
	session, err := auth.CreateSession(context.Background(), owner.UserID, "local", "", time.Hour, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	cookies := []*http.Cookie{{Name: "session", Value: session.Secret}, {Name: "csrf", Value: session.CSRFSecret}}
	list := request(handler, http.MethodGet, "/notifications", nil, cookies, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `data-notification-cursor=""`) {
		t.Fatalf("empty notifications page = %d, %s", list.Code, list.Body.String())
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	open := func() (*http.Response, *bufio.Reader) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/notifications/events?after=", nil)
		if err != nil {
			t.Fatal(err)
		}
		addCookies(request, cookies)
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response, bufio.NewReader(response.Body)
	}
	first, firstReader := open()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first event stream = %d", first.StatusCode)
	}
	if closeEvent := readUntil(t, firstReader, "event: close") + readUntil(t, firstReader, "\n\n"); !strings.Contains(closeEvent, `"reason":"lifetime"`) {
		t.Fatalf("lifetime close event = %q", closeEvent)
	}
	first.Body.Close()
	created, err := auth.CreateNotification(context.Background(), PrincipalSystem(), identity.NotificationInput{
		RecipientUserID: owner.UserID, WorkspaceID: workspaceID, NotificationType: "workspace", Title: "Gap", Body: "Gap replay body",
	}, identity.AuditContext{})
	if err != nil {
		t.Fatal(err)
	}
	second, secondReader := open()
	defer second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("reconnected event stream = %d", second.StatusCode)
	}
	replayed := readUntil(t, secondReader, "event: notification.created") + readUntil(t, secondReader, "\n\n")
	if !strings.Contains(replayed, created.ID.String()) || !strings.Contains(replayed, "Gap replay body") {
		t.Fatalf("notification after lifetime close = %q", replayed)
	}
}
