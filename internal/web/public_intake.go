package web

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/intake"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func publicIntakeErrors(data viewmodel.PageData, submission intake.Submission) map[string]string {
	fields := make(map[string]string)
	name := strings.TrimSpace(submission.DisplayName)
	phone := strings.TrimSpace(submission.Phone)
	email := strings.TrimSpace(submission.Email)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 200 {
		fields["name"] = data.T("public_intake.name_error")
	}
	if !utf8.ValidString(phone) || utf8.RuneCountInString(phone) > 50 {
		fields["phone"] = data.T("public_intake.phone_error")
	}
	if email != "" {
		address, err := mail.ParseAddress(email)
		if len(email) > 254 || !utf8.ValidString(email) || err != nil || address.Address != email {
			fields["email"] = data.T("public_intake.email_error")
		}
	}
	if email == "" && phone == "" {
		fields["phone"] = data.T("public_intake.contact_required")
		fields["email"] = data.T("public_intake.contact_required")
	}
	if !utf8.ValidString(submission.Notes) || utf8.RuneCountInString(submission.Notes) > 2000 {
		fields["notes"] = data.T("public_intake.notes_error")
	}
	if !submission.Consent {
		fields["consent"] = data.T("public_intake.consent_error")
	}
	return fields
}

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
	data := viewmodel.PageData{Locale: string(h.catalog.Resolve("", "", "", r.Header.Get("Accept-Language"))), Theme: h.pageTheme(w), Catalog: h.catalog}
	w.Header().Set("Content-Language", data.Locale)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var submission intake.Submission
	status := http.StatusOK
	message := ""
	var fields map[string]string
	complete := false
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
		if err := r.ParseForm(); err != nil {
			h.renderError(w, http.StatusBadRequest)
			return
		}
		if r.FormValue("website") != "" {
			w.WriteHeader(http.StatusOK)
			_ = publicIntakePage(data, "", submission, "", nil, true).Render(r.Context(), w)
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
			fields = publicIntakeErrors(data, submission)
			if strings.TrimSpace(submission.Email) == "" && strings.TrimSpace(submission.Phone) == "" {
				message = data.T("public_intake.contact_required")
			}
		case err != nil:
			h.renderError(w, http.StatusInternalServerError)
			return
		default:
			complete = true
		}
	}
	w.WriteHeader(status)
	_ = publicIntakePage(data, consent, submission, message, fields, complete).Render(r.Context(), w)
}
