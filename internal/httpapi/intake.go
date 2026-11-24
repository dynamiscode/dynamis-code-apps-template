package httpapi

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/intake"
)

func (h *handler) submitPublicIntake(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if h.intake == nil {
		notFoundProblem(w, r)
		return
	}
	if _, err := h.intake.PublicForm(r.Context(), r.PathValue("token")); err != nil {
		if errors.Is(err, intake.ErrNotFound) {
			notFoundProblem(w, r)
		} else {
			h.internal(w, r)
		}
		return
	}
	var input struct {
		DisplayName string `json:"name"`
		Phone       string `json:"phone"`
		Email       string `json:"email"`
		Notes       string `json:"notes"`
		Consent     bool   `json:"consent"`
		Website     string `json:"website"`
	}
	if err := decodeJSON(r, &input); err != nil {
		h.badJSON(w, r, err)
		return
	}
	if input.Website == "" {
		err := h.intake.Submit(r.Context(), r.PathValue("token"), intake.Submission{DisplayName: input.DisplayName, Phone: input.Phone, Email: input.Email, Notes: input.Notes, Consent: input.Consent}, h.auditContext(r))
		switch {
		case errors.Is(err, intake.ErrNotFound):
			notFoundProblem(w, r)
			return
		case errors.Is(err, intake.ErrInvalidInput):
			h.invalidRequest(w, r, "The submission is invalid.")
			return
		case err != nil:
			h.internal(w, r)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
