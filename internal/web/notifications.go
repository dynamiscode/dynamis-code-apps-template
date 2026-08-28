package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
)

func (h *Handler) notificationsPage(writer http.ResponseWriter, request *http.Request) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return
	}
	notifications, err := h.identity.ListNotifications(request.Context(), session.UserID, "", true, 100)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	unread, err := h.identity.UnreadNotificationCount(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "notifications.html", pageData{
		Title: "Notifications", NavPage: "notifications", CSRF: csrf,
		Notifications: notifications, UnreadNotifications: unread,
	})
}

func (h *Handler) notificationMutation(writer http.ResponseWriter, request *http.Request) {
	session, ok := h.validCSRFSession(writer, request)
	if !ok {
		return
	}
	if err := h.identity.MarkNotificationRead(request.Context(), session.UserID,
		request.PathValue("notificationId"), auditContext(request)); err != nil {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	h.redirect(writer, request, "/notifications")
}

func (h *Handler) notificationEvents(writer http.ResponseWriter, request *http.Request) {
	principal, session, _, ok := h.notificationSession(writer, request)
	if !ok {
		return
	}
	if !h.streams.acquire(principal.UserID) {
		telemetry.RecordStream(request.Context(), 0, true)
		writer.Header().Set("Content-Type", "application/problem+json")
		writer.Header().Set("Retry-After", "30")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(writer, `{"type":"urn:dynamis-code:problem:stream-limit","title":"Too Many Requests","status":429,"code":"stream-limit"}`)
		return
	}
	telemetry.RecordStream(request.Context(), 1, false)
	defer func() {
		h.streams.release(principal.UserID)
		telemetry.RecordStream(request.Context(), -1, false)
	}()
	flusher, ok := writer.(http.Flusher)
	if !ok {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Accel-Buffering", "no")
	after := request.Header.Get("Last-Event-ID")
	if after == "" {
		existing, err := h.identity.ListNotifications(request.Context(), session.UserID, "", true, 1)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
		if len(existing) == 1 {
			after = existing[0].ID
		}
	}
	writer.WriteHeader(http.StatusOK)
	if !h.sendNotifications(writer, request, session.UserID, &after) {
		return
	}
	flusher.Flush()
	poll := time.NewTicker(h.cfg.SSEPollInterval)
	heartbeat := time.NewTicker(h.cfg.SSEHeartbeat)
	lifetime := time.NewTimer(h.cfg.SSEMaxLifetime)
	defer poll.Stop()
	defer heartbeat.Stop()
	defer lifetime.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-lifetime.C:
			_, _ = fmt.Fprint(writer, "event: close\ndata: {\"reason\":\"lifetime\"}\n\n")
			flusher.Flush()
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(writer, ": heartbeat\n\n")
			flusher.Flush()
		case <-poll.C:
			if !h.sendNotifications(writer, request, session.UserID, &after) {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Handler) notificationSession(
	writer http.ResponseWriter,
	request *http.Request,
) (identity.Principal, identity.Session, string, bool) {
	session, csrf, ok := h.session(writer, request)
	if !ok {
		return identity.Principal{}, identity.Session{}, "", false
	}
	return identity.Principal{UserID: session.UserID, AuthMethod: session.AuthMethod}, session, csrf, true
}

func (h *Handler) sendNotifications(
	writer http.ResponseWriter,
	request *http.Request,
	userID string,
	after *string,
) bool {
	notifications, err := h.identity.NotificationsAfter(request.Context(), userID, *after, 100)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidNotification) {
			latest, listErr := h.identity.ListNotifications(request.Context(), userID, "", true, 1)
			if listErr != nil {
				return false
			}
			if len(latest) == 1 {
				_, _ = fmt.Fprintf(writer, "id: %s\n", latest[0].ID)
				*after = latest[0].ID
			} else {
				*after = ""
			}
			_, _ = fmt.Fprint(writer, "event: resync\ndata: {\"reason\":\"cursor\"}\n\n")
			return true
		}
		return false
	}
	for _, notification := range notifications {
		encoded, err := json.Marshal(notificationEvent{
			ID: notification.ID, Type: notification.NotificationType,
			Title: notification.Title, Body: notification.Body,
			CreatedAt: notification.CreatedAt.UTC(),
		})
		if err != nil {
			return false
		}
		_, _ = fmt.Fprintf(writer, "id: %s\nevent: notification.created\ndata: %s\n\n", notification.ID, encoded)
		*after = notification.ID
	}
	return true
}

type notificationEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}
