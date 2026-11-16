package web

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func TestLegacyNavigationBridge(t *testing.T) {
	_, _, _, workspaceID, _ := testWeb(t, 10)
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := template.New("root").Funcs(template.FuncMap{"navigation": renderNavigation}).ParseFS(files, "templates/navigation.html")
	if err != nil {
		t.Fatal(err)
	}
	data := pageData{
		Catalog: catalog, Locale: "es", NavSection: "settings", NavPage: "invitations",
		Workspace: identity.WorkspaceSummary{ID: workspaceID, Name: "<script>", Role: identity.Viewer},
		CSRF:      `"><script>`,
	}
	for _, name := range []string{"ui-head", "skip-link", "workspace-context", "workspace-list-crumb", "default-home-crumb", "account-menu", "workspace-nav", "people-tabs"} {
		var body bytes.Buffer
		if err := legacy.ExecuteTemplate(&body, name, data); err != nil {
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
