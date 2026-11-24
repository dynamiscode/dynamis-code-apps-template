package web

import molecules "example.com/dynamis-code/apps-template/internal/web/components/molecules"
import organisms "example.com/dynamis-code/apps-template/internal/web/components/organisms"
import templates "example.com/dynamis-code/apps-template/internal/web/components/templates"

import (
	"bytes"
	"context"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"github.com/a-h/templ"
	"net/http"
	"strings"
	"testing"
)

func TestAvatarUsesSessionProfileAcrossRenderPaths(t *testing.T) {
	env := newClientsTestEnv(t)
	for _, path := range []string{"/account", "/workspaces/" + env.workspaceID.String(), env.listPath()} {
		response := request(env.routes, http.MethodGet, path, nil, env.cookies, nil)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `class="text-sm font-semibold">O</span>`) {
			t.Errorf("%s: expected signed-in avatar initial (status %d)", path, response.Code)
		}
	}
}

func TestCatalogWorkspaceNavigation(t *testing.T) {
	_, _, workspaceID, _ := testWeb(t, 10)
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en", Workspace: identity.WorkspaceSummary{ID: workspaceID}, NavPage: "catalog"}
	for _, enabled := range []bool{false, true} {
		data.CatalogEnabled = enabled
		var body bytes.Buffer
		if err := organisms.WorkspaceNav(data).Render(context.Background(), &body); err != nil {
			t.Fatal(err)
		}
		link := `aria-current="page" href="/workspaces/` + workspaceID.String() + `/catalog"`
		if strings.Contains(body.String(), link) != enabled {
			t.Errorf("catalog enabled=%v: active catalog link mismatch", enabled)
		}
	}
}

func TestAccountNavigationCurrentPage(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	for page, path := range map[string]string{"account": "/account", "notifications": "/notifications", "language": "/settings/language", "security": "/security", "sessions": "/sessions"} {
		data := viewmodel.PageData{Catalog: catalog, Locale: "en", NavPage: page}
		var body bytes.Buffer
		if err := organisms.AccountNavigation(data).Render(context.Background(), &body); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(body.String(), `aria-current="page" href="`+path+`"`) || strings.Count(body.String(), `aria-current="page"`) != 1 {
			t.Errorf("%s: expected one current navigation link", page)
		}
	}
}

func TestNavigationComponentsEscapeInput(t *testing.T) {
	_, _, workspaceID, _ := testWeb(t, 10)
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	components := map[string]func(viewmodel.PageData) templ.Component{
		"ui-head": templates.UiHead, "skip-link": molecules.SkipLink, "workspace-context": organisms.WorkspaceContext,
		"account-menu": organisms.AccountMenu, "workspace-nav": organisms.WorkspaceNav, "people-tabs": organisms.PeopleTabs,
	}
	data := viewmodel.PageData{
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
