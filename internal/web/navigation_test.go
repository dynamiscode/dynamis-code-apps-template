package web

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestNavigationComponentsEscapeInput(t *testing.T) {
	_, _, _, workspaceID, _ := testWeb(t, 10)
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	components := map[string]func(pageData) templ.Component{
		"ui-head": uiHead, "skip-link": skipLink, "workspace-context": workspaceContext,
		"workspace-list-crumb": workspaceListCrumb, "default-home-crumb": defaultHomeCrumb,
		"account-menu": accountMenu, "workspace-nav": workspaceNav, "people-tabs": peopleTabs,
	}
	data := pageData{
		Catalog: catalog, Locale: "es", NavSection: "settings", NavPage: "invitations",
		Workspace: identity.WorkspaceSummary{ID: workspaceID, Name: "<script>", Role: identity.Viewer},
		CSRF:      `"><script>`,
	}
	for name, component := range components {
		var body bytes.Buffer
		if err := component(data).Render(context.Background(), &body); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(body.String(), "<script>") {
			t.Errorf("%s renders unescaped input", name)
		}
		switch name {
		case "workspace-context":
			if !strings.Contains(body.String(), "&lt;script&gt;") {
				t.Error("workspace name was not escaped")
			}
		case "workspace-nav":
			if strings.Contains(body.String(), "/settings/import") || !strings.Contains(body.String(), `aria-current="page"`) {
				t.Error("viewer permissions or settings active state changed")
			}
		case "account-menu":
			if !strings.Contains(body.String(), `name="csrf" value="&#34;&gt;&lt;script&gt;"`) {
				t.Error("logout CSRF value was not preserved and escaped")
			}
		case "ui-head":
			if !strings.Contains(body.String(), `data-msg-step-of=`) || !strings.Contains(body.String(), "{step}") {
				t.Error("translated form-script messages are missing")
			}
		}
	}
}
