package web

import (
	"errors"
	"net/http"

	"example.com/dynamis-code/apps-template/internal/intake"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func (h *Handler) publicIntake(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if h.intake == nil {
		h.renderError(w, http.StatusNotFound)
		return
	}
	consent, err := h.intake.PublicForm(r.Context(), r.PathValue("token"))
	if errors.Is(err, intake.ErrNotFound) {
		h.renderError(w, http.StatusNotFound)
		return
	}
	if err != nil {
		h.renderError(w, http.StatusInternalServerError)
		return
	}
	data := viewmodel.PageData{Locale: string(h.catalog.Resolve("", "", "", r.Header.Get("Accept-Language"))), Catalog: h.catalog}
	w.Header().Set("Content-Language", data.Locale)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var submission intake.Submission
	status := http.StatusOK
	message := ""
	complete := false
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		if err := r.ParseForm(); err != nil {
			h.renderError(w, http.StatusBadRequest)
			return
		}
		if r.FormValue("website") != "" {
			w.WriteHeader(http.StatusOK)
			_ = publicIntakePage(data, "", submission, "", true).Render(r.Context(), w)
			return
		}
		submission = intake.Submission{DisplayName: r.FormValue("name"), Phone: r.FormValue("phone"), Email: r.FormValue("email"), Notes: r.FormValue("notes"), Consent: r.FormValue("consent") == "on"}
		err := h.intake.Submit(r.Context(), r.PathValue("token"), submission, auditContext(r))
		switch {
		case errors.Is(err, intake.ErrNotFound):
			h.renderError(w, http.StatusNotFound)
			return
		case errors.Is(err, intake.ErrInvalidInput):
			status, message = http.StatusUnprocessableEntity, data.T("public_intake.invalid")
		case err != nil:
			h.renderError(w, http.StatusInternalServerError)
			return
		default:
			complete = true
		}
	}
	w.WriteHeader(status)
	_ = publicIntakePage(data, consent, submission, message, complete).Render(r.Context(), w)
}
