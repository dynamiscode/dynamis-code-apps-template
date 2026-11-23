package web

import (
	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// ScreenshotLogin returns the real sign-in page with synthetic data.
func ScreenshotLogin() (templ.Component, error) {
	catalog, err := i18n.New()
	if err != nil {
		return nil, err
	}
	return authLogin(viewmodel.PageData{Locale: "en", Theme: "system", Catalog: catalog}), nil
}
