package web

import (
	"net/http"
	"time"
)

// themeCookie mirrors the signed-in user's theme preference so anonymous and
// first-paint renders match it. The stored preference always wins on conflict.
const (
	themeCookie    = "theme"
	themeCookieTTL = 365 * 24 * time.Hour
)

// pageTheme resolves the theme for a rendered page: the signed-in user's
// preference, then the mirrored cookie, then system. It refreshes the mirror
// when it disagrees with the stored preference.
func (h *Handler) pageTheme(writer http.ResponseWriter) string {
	state, _ := writer.(*localeWriter)
	if state == nil {
		return "system"
	}
	values, _ := state.context.Value(localeContextKey{}).(localeState)
	if validTheme(values.userTheme) {
		if values.cookieTheme != values.userTheme {
			h.setCookie(writer, themeCookie, values.userTheme, time.Now().Add(themeCookieTTL), true)
		}
		return values.userTheme
	}
	if validTheme(values.cookieTheme) {
		return values.cookieTheme
	}
	return "system"
}

func (h *Handler) accountThemeMutation(request *http.Request) authPageOutput {
	if err := request.ParseForm(); err != nil {
		return authPageOutput{status: http.StatusBadRequest}
	}
	session, status := h.checkCSRFSession(request)
	if status != http.StatusOK {
		return authSessionFailure(status)
	}
	theme := request.FormValue("theme")
	if !validTheme(theme) {
		return authPageOutput{status: http.StatusBadRequest}
	}
	if err := h.identity.SetUserTheme(request.Context(), session.UserID, theme, auditContext(request)); err != nil {
		return authPageOutput{status: http.StatusInternalServerError}
	}
	return authPageOutput{kind: authPageThemeChanged, theme: theme, target: safeReturnTo(request.FormValue("return_to"))}
}

func validTheme(theme string) bool {
	switch theme {
	case "system", "light", "dark":
		return true
	default:
		return false
	}
}
