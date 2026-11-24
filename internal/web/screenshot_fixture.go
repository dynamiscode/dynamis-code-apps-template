package web

import (
	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
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

// PreviewWorkspaces renders the workspace selection page with synthetic data.
func PreviewWorkspaces(workspaces []identity.WorkspaceSummary, message string) (templ.Component, error) {
	catalog, err := i18n.New()
	if err != nil {
		return nil, err
	}
	return workspaceSelection(viewmodel.PageData{
		Locale: "en", Theme: "light", Catalog: catalog, Title: "Workspaces",
		NavPage: "home", Workspaces: workspaces, CanCreateWorkspace: true, Error: message,
	}), nil
}
