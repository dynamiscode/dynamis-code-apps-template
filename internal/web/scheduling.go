package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/scheduling"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) EnableScheduling(s *scheduling.Service) { h.scheduling = s }

func (h *Handler) schedulingBookingStatus(w http.ResponseWriter, r *http.Request) {
	if h.scheduling == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	err := h.scheduling.SetBookingStatus(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), scheduling.ParseBookingID(r.PathValue("bookingId")), scheduling.BookingStatus(r.FormValue("status")))
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(w, http.StatusForbidden)
	case errors.Is(err, scheduling.ErrNotFound):
		h.renderError(w, http.StatusNotFound)
	case errors.Is(err, scheduling.ErrInvalidTransition):
		h.renderError(w, http.StatusConflict)
	case err != nil:
		h.renderError(w, http.StatusInternalServerError)
	default:
		h.redirect(w, r, "/workspaces/"+workspace.String()+"/scheduling")
	}
}

type schedulingDraft struct {
	action, resourceID, bookingID, name, userID, zone, entryID, start, bookingError, engagementID string
	hours                                                                                         []scheduling.WorkingHours
}

func (h *Handler) schedulingList(w http.ResponseWriter, r *http.Request) {
	h.renderScheduling(w, r, http.StatusOK)
}

func (h *Handler) renderScheduling(w http.ResponseWriter, r *http.Request, status int) {
	if h.scheduling == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	resources, zone, err := h.scheduling.List(r.Context(), principal, workspace)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	bookings := make(map[scheduling.ResourceID][]scheduling.Booking, len(resources))
	history := make(map[scheduling.BookingID][]scheduling.BookingMove)
	for _, resource := range resources {
		bookings[resource.ID], _, err = h.scheduling.Schedule(r.Context(), principal, workspace, resource.ID)
		if err != nil {
			h.renderError(w, http.StatusInternalServerError)
			return
		}
		for _, booking := range bookings[resource.ID] {
			history[booking.ID], err = h.scheduling.BookingHistory(r.Context(), principal, workspace, resource.ID, booking.ID)
			if err != nil {
				h.renderError(w, http.StatusInternalServerError)
				return
			}
		}
	}
	var entries []catalog.Entry
	var engagements []scheduling.EngagementOption
	if h.engagement != nil {
		engagements, err = h.scheduling.EngagementOptions(r.Context(), principal, workspace)
		if err != nil {
			h.renderError(w, http.StatusInternalServerError)
			return
		}
	}
	if h.catalogService != nil && principal.Permissions[identity.ResourcesWrite] {
		entries, err = h.catalogService.List(r.Context(), principal, workspace, false)
		if err != nil {
			h.renderError(w, http.StatusInternalServerError)
			return
		}
	}
	workspaces, err := h.identity.ListWorkspaces(r.Context(), session.UserID)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	members, err := h.identity.ListMembers(r.Context(), principal)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{CSRF: csrf, Workspace: workspaceByID(workspaces, workspace), Workspaces: workspaces, NavPage: "workspace", NavSection: "workspace", SingleWorkspace: h.workspaceMode == config.WorkspaceModeSingle, FilesEnabled: h.filesEnabled, ClientsEnabled: h.clientsEnabled}
	h.localizePage(w, &data)
	data.Title = data.T("scheduling.title")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	draft := schedulingDraft{}
	if status == http.StatusUnprocessableEntity {
		draft = schedulingDraft{action: r.PathValue("resourceId"), resourceID: r.PathValue("resourceId"), name: r.FormValue("name"), userID: r.FormValue("user_id"), zone: r.FormValue("time_zone")}
		if strings.HasSuffix(r.URL.Path, "/working-hours") {
			draft.action = "hours"
			draft.hours, _ = schedulingHoursInput(r)
		}
		if strings.HasSuffix(r.URL.Path, "/bookings") {
			draft.action = "booking"
			draft.entryID, draft.start, draft.bookingError = r.FormValue("entry_id"), r.FormValue("start"), r.FormValue("booking_error")
			draft.engagementID = r.FormValue("engagement_id")
		}
		if strings.HasSuffix(r.URL.Path, "/move") {
			draft.action, draft.bookingID = "move", r.PathValue("bookingId")
			draft.start, draft.bookingError = r.FormValue("start"), r.FormValue("booking_error")
		}
		if draft.action == "" {
			draft.action = "create"
			if strings.HasSuffix(r.URL.Path, "/time-zone") {
				draft.action = "zone"
			}
		}
	}
	_ = schedulingPage(data, resources, zone, members, bookings, history, entries, engagements, identity.CanManageScheduling(principal), principal.Permissions[identity.ResourcesWrite], status == http.StatusUnprocessableEntity, draft).Render(r.Context(), w)
}

func (h *Handler) schedulingBook(w http.ResponseWriter, r *http.Request) {
	if h.scheduling == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	_, zone, err := h.scheduling.List(r.Context(), principal, workspace)
	if err != nil {
		h.renderError(w, http.StatusForbidden)
		return
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	raw := r.FormValue("start")
	start, err := time.ParseInLocation("2006-01-02T15:04", raw, location)
	if err == nil && start.Format("2006-01-02T15:04") != raw {
		err = scheduling.ErrInvalidInput
	}
	if err == nil {
		if strings.HasSuffix(r.URL.Path, "/move") {
			err = h.scheduling.MoveBooking(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), scheduling.ParseBookingID(r.PathValue("bookingId")), start)
		} else {
			_, err = h.scheduling.BookWithEngagement(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), catalog.ParseEntryID(r.FormValue("entry_id")), start, webEngagementID(r.FormValue("engagement_id")))
		}
	}
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(w, http.StatusForbidden)
	case errors.Is(err, scheduling.ErrNotFound):
		h.renderError(w, http.StatusNotFound)
	case errors.Is(err, scheduling.ErrInvalidTransition):
		h.renderError(w, http.StatusConflict)
	case err == nil:
		h.redirect(w, r, "/workspaces/"+workspace.String()+"/scheduling")
	case errors.Is(err, scheduling.ErrNoDuration), errors.Is(err, scheduling.ErrOutsideHours), errors.Is(err, scheduling.ErrOverlap), errors.Is(err, scheduling.ErrInvalidInput):
		key := "invalid_booking"
		if errors.Is(err, scheduling.ErrNoDuration) {
			key = "no_duration"
		}
		if errors.Is(err, scheduling.ErrOutsideHours) {
			key = "outside_hours"
		}
		if errors.Is(err, scheduling.ErrOverlap) {
			key = "overlap"
		}
		r.Form.Set("booking_error", key)
		h.renderScheduling(w, r, http.StatusUnprocessableEntity)
	default:
		if _, ok := err.(*time.ParseError); ok {
			r.Form.Set("booking_error", "invalid_booking")
			h.renderScheduling(w, r, http.StatusUnprocessableEntity)
			return
		}
		h.renderError(w, http.StatusInternalServerError)
	}
}

func webEngagementID(raw string) *engagement.ID {
	if raw == "" {
		return nil
	}
	value := engagement.ParseID(raw)
	return &value
}

func (h *Handler) schedulingSetEngagement(w http.ResponseWriter, r *http.Request) {
	if h.scheduling == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	actor, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	err := h.scheduling.SetEngagement(r.Context(), actor, workspace, scheduling.ParseBookingID(r.PathValue("bookingId")), webEngagementID(r.FormValue("engagement_id")))
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(w, http.StatusForbidden)
	case errors.Is(err, scheduling.ErrNotFound):
		h.renderError(w, http.StatusNotFound)
	case errors.Is(err, scheduling.ErrInvalidInput):
		h.renderError(w, http.StatusUnprocessableEntity)
	case err != nil:
		h.renderError(w, http.StatusInternalServerError)
	default:
		h.redirect(w, r, "/workspaces/"+workspace.String()+"/scheduling")
	}
}

func (h *Handler) schedulingCreate(w http.ResponseWriter, r *http.Request) {
	h.schedulingMutation(w, r, "create")
}
func (h *Handler) schedulingEdit(w http.ResponseWriter, r *http.Request) {
	h.schedulingMutation(w, r, "edit")
}
func (h *Handler) schedulingZone(w http.ResponseWriter, r *http.Request) {
	h.schedulingMutation(w, r, "zone")
}
func (h *Handler) schedulingHours(w http.ResponseWriter, r *http.Request) {
	h.schedulingMutation(w, r, "hours")
}

func schedulingHoursInput(r *http.Request) ([]scheduling.WorkingHours, error) {
	if err := r.ParseForm(); err != nil {
		return nil, scheduling.ErrInvalidInput
	}
	days, starts, ends := r.PostForm["weekday"], r.PostForm["start"], r.PostForm["end"]
	if len(days) != len(starts) || len(days) != len(ends) {
		return nil, scheduling.ErrInvalidInput
	}
	hours := make([]scheduling.WorkingHours, 0, len(days))
	for i, raw := range days {
		day, err := strconv.Atoi(raw)
		if err != nil {
			return nil, scheduling.ErrInvalidInput
		}
		if starts[i] == "" && ends[i] == "" {
			continue
		}
		hours = append(hours, scheduling.WorkingHours{Weekday: day, Start: starts[i], End: ends[i]})
	}
	return hours, nil
}

func (h *Handler) schedulingMutation(w http.ResponseWriter, r *http.Request, action string) {
	if h.scheduling == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	principal, ok := h.mutationPrincipal(w, r)
	if !ok {
		return
	}
	if !identity.CanManageScheduling(principal) {
		h.renderError(w, http.StatusForbidden)
		return
	}
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	var err error
	if action == "hours" {
		hours, parseErr := schedulingHoursInput(r)
		err = parseErr
		if err == nil {
			err = h.scheduling.SetWorkingHours(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), hours)
		}
	} else if action == "zone" {
		err = h.scheduling.SetTimeZone(r.Context(), principal, workspace, r.FormValue("time_zone"))
	} else {
		var user *identity.UserID
		if raw := r.FormValue("user_id"); raw != "" {
			var parsed identity.UserID
			parsed, err = identity.ParseUserID(raw)
			user = &parsed
		}
		if err != nil {
			err = scheduling.ErrInvalidInput
		} else if action == "create" {
			_, err = h.scheduling.Create(r.Context(), principal, workspace, r.FormValue("name"), user)
		} else {
			err = h.scheduling.Update(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), r.FormValue("name"), user)
		}
	}
	switch {
	case errors.Is(err, scheduling.ErrInvalidInput):
		h.renderScheduling(w, r, http.StatusUnprocessableEntity)
	case errors.Is(err, scheduling.ErrNotFound):
		h.renderError(w, http.StatusNotFound)
	case errors.Is(err, identity.ErrForbidden):
		h.renderError(w, http.StatusForbidden)
	case err != nil:
		h.renderError(w, http.StatusInternalServerError)
	default:
		h.redirect(w, r, "/workspaces/"+workspace.String()+"/scheduling")
	}
}
