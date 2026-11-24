package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"example.com/dynamis-code/apps-template/internal/catalog"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/scheduling"
)

func (h *handler) moveSchedulingBooking(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	var body struct {
		Start time.Time `json:"start"`
	}
	if err := decodeJSON(r, &body); err != nil {
		h.badJSON(w, r, err)
		return
	}
	h.schedulingResult(w, r, h.scheduling.MoveBooking(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), scheduling.ParseBookingID(r.PathValue("bookingId")), body.Start))
}

func (h *handler) schedulingBookingHistory(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	history, err := h.scheduling.BookingHistory(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), scheduling.ParseBookingID(r.PathValue("bookingId")))
	if err != nil {
		h.schedulingResult(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (h *handler) setSchedulingBookingStatus(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	var body struct {
		Status scheduling.BookingStatus `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		h.badJSON(w, r, err)
		return
	}
	h.schedulingResult(w, r, h.scheduling.SetBookingStatus(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), scheduling.ParseBookingID(r.PathValue("bookingId")), body.Status))
}

func (h *handler) schedulingBookings(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	permission := identity.ResourcesRead
	if r.Method == http.MethodPost {
		permission = identity.ResourcesWrite
	}
	principal, ok := h.workspaceBearer(w, r, workspace, permission)
	if !ok {
		return
	}
	resource := scheduling.ParseResourceID(r.PathValue("resourceId"))
	if r.Method == http.MethodGet {
		bookings, zone, err := h.scheduling.Schedule(r.Context(), principal, workspace, resource)
		if err != nil {
			h.schedulingResult(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Bookings []scheduling.Booking `json:"bookings"`
			TimeZone string               `json:"timeZone"`
		}{bookings, zone})
		return
	}
	var body struct {
		EntryID      string    `json:"entryId"`
		Start        time.Time `json:"start"`
		EngagementID *string   `json:"engagementId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		h.badJSON(w, r, err)
		return
	}
	link := schedulingEngagementID(body.EngagementID)
	booking, err := h.scheduling.BookWithEngagement(r.Context(), principal, workspace, resource, catalog.ParseEntryID(body.EntryID), body.Start, link)
	if err != nil {
		h.schedulingResult(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, booking)
}

func schedulingEngagementID(raw *string) *engagement.ID {
	if raw == nil {
		return nil
	}
	value := engagement.ParseID(*raw)
	return &value
}

func (h *handler) setSchedulingEngagement(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	var body struct {
		EngagementID json.RawMessage `json:"engagementId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if body.EngagementID == nil {
		h.schedulingResult(w, r, scheduling.ErrInvalidInput)
		return
	}
	var raw *string
	if err := json.Unmarshal(body.EngagementID, &raw); err != nil {
		h.badJSON(w, r, err)
		return
	}
	h.schedulingResult(w, r, h.scheduling.SetEngagement(r.Context(), principal, workspace, scheduling.ParseBookingID(r.PathValue("bookingId")), schedulingEngagementID(raw)))
}

func (h *handler) listSchedulingResources(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesRead)
	if !ok {
		return
	}
	resources, zone, err := h.scheduling.List(r.Context(), principal, workspace)
	if err != nil {
		h.schedulingResult(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Resources []scheduling.Resource `json:"resources"`
		TimeZone  string                `json:"timeZone"`
	}{resources, zone})
}

type schedulingResourceInput struct {
	Name   string  `json:"name"`
	UserID *string `json:"userId"`
}

func (in schedulingResourceInput) user() (*identity.UserID, error) {
	if in.UserID == nil {
		return nil, nil
	}
	user, err := identity.ParseUserID(*in.UserID)
	if err != nil {
		return nil, scheduling.ErrInvalidInput
	}
	return &user, nil
}

func (h *handler) createSchedulingResource(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	if !identity.CanManageScheduling(principal) {
		h.schedulingResult(w, r, identity.ErrForbidden)
		return
	}
	var body schedulingResourceInput
	if err := decodeJSON(r, &body); err != nil {
		h.badJSON(w, r, err)
		return
	}
	user, err := body.user()
	if err != nil {
		h.schedulingResult(w, r, err)
		return
	}
	resource, err := h.scheduling.Create(r.Context(), principal, workspace, body.Name, user)
	if err != nil {
		h.schedulingResult(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, resource)
}

func (h *handler) updateSchedulingResource(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	if !identity.CanManageScheduling(principal) {
		h.schedulingResult(w, r, identity.ErrForbidden)
		return
	}
	var body schedulingResourceInput
	if err := decodeJSON(r, &body); err != nil {
		h.badJSON(w, r, err)
		return
	}
	user, err := body.user()
	if err != nil {
		h.schedulingResult(w, r, err)
		return
	}
	h.schedulingResult(w, r, h.scheduling.Update(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), body.Name, user))
}

func (h *handler) setSchedulingTimeZone(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	if !identity.CanManageScheduling(principal) {
		h.schedulingResult(w, r, identity.ErrForbidden)
		return
	}
	var body struct {
		TimeZone string `json:"timeZone"`
	}
	if err := decodeJSON(r, &body); err != nil {
		h.badJSON(w, r, err)
		return
	}
	h.schedulingResult(w, r, h.scheduling.SetTimeZone(r.Context(), principal, workspace, body.TimeZone))
}

func (h *handler) setSchedulingWorkingHours(w http.ResponseWriter, r *http.Request) {
	workspace := identity.ParseWorkspaceIDOrZero(r.PathValue("workspaceId"))
	principal, ok := h.workspaceBearer(w, r, workspace, identity.ResourcesWrite)
	if !ok {
		return
	}
	if !identity.CanManageScheduling(principal) {
		h.schedulingResult(w, r, identity.ErrForbidden)
		return
	}
	var body struct {
		Hours []scheduling.WorkingHours `json:"hours"`
	}
	if err := decodeJSON(r, &body); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if body.Hours == nil {
		h.schedulingResult(w, r, scheduling.ErrInvalidInput)
		return
	}
	err := h.scheduling.SetWorkingHours(r.Context(), principal, workspace, scheduling.ParseResourceID(r.PathValue("resourceId")), body.Hours)
	if errors.Is(err, scheduling.ErrInvalidInput) {
		writeProblem(w, r, http.StatusUnprocessableEntity, "invalid-input", "Working hours must have valid weekdays and non-overlapping ranges with the end after the start.")
		return
	}
	h.schedulingResult(w, r, err)
}

func (h *handler) schedulingResult(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(w, r, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, scheduling.ErrInvalidInput):
		writeProblem(w, r, http.StatusUnprocessableEntity, "invalid-input", "Scheduling input is invalid.")
	case errors.Is(err, scheduling.ErrNotFound):
		writeProblem(w, r, http.StatusNotFound, "not-found", "Resource not found.")
	case errors.Is(err, scheduling.ErrNoDuration):
		writeProblem(w, r, http.StatusUnprocessableEntity, "no-duration", "Catalog entry has no duration.")
	case errors.Is(err, scheduling.ErrOutsideHours):
		writeProblem(w, r, http.StatusUnprocessableEntity, "outside-hours", "Booking is outside working hours.")
	case errors.Is(err, scheduling.ErrOverlap):
		writeProblem(w, r, http.StatusConflict, "overlap", "Booking overlaps another booking.")
	case errors.Is(err, scheduling.ErrInvalidTransition):
		writeProblem(w, r, http.StatusConflict, "invalid-transition", "Booking outcome cannot be changed.")
	case err != nil:
		h.internal(w, r)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
