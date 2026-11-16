package web

import (
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/a-h/templ"
)

func currentPage(active bool) string {
	if active {
		return "page"
	}
	return "false"
}

// renderNavigation bridges legacy html/template callers to escaped templ output.
func renderNavigation(name string, data pageData) (template.HTML, error) {
	components := map[string]func(pageData) templ.Component{
		"ui-head": uiHead, "skip-link": skipLink, "workspace-context": workspaceContext,
		"workspace-list-crumb": workspaceListCrumb, "default-home-crumb": defaultHomeCrumb,
		"account-menu": accountMenu, "workspace-nav": workspaceNav, "people-tabs": peopleTabs,
	}
	component, ok := components[name]
	if !ok {
		return "", fmt.Errorf("unknown navigation component %q", name)
	}
	var out bytes.Buffer
	if err := component(data).Render(context.Background(), &out); err != nil {
		return "", err
	}
	return template.HTML(out.String()), nil
}
