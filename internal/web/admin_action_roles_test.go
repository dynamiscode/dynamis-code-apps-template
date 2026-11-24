package web

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestWorkspaceCreationDisclosureRespectsPermission(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, allowed := range []bool{false, true} {
		data := viewmodel.PageData{Catalog: catalog, Locale: "en", CanCreateWorkspace: allowed}
		var body bytes.Buffer
		if err := workspaceSelection(data).Render(context.Background(), &body); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body.String(), `class="collapse-title font-semibold rounded-field border border-base-300"`) != allowed {
			t.Errorf("bounded workspace creation disclosure visibility differs from canCreate=%v", allowed)
		}
	}
}

func TestCurrencyNavigationAndViewerPermissions(t *testing.T) {
	_, _, workspaceID, _ := testWeb(t, 10)
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en", Workspace: identity.WorkspaceSummary{ID: workspaceID}}
	for _, canWrite := range []bool{false, true} {
		var body bytes.Buffer
		if err := currencySettingsView(data, nil, "", "", "", false, canWrite).Render(context.Background(), &body); err != nil {
			t.Fatal(err)
		}
		page := body.String()
		if !strings.Contains(page, `class="btn btn-outline btn-sm" href="/workspaces/`+workspaceID.String()+`/settings/general"`) {
			t.Error("currency settings must provide a visibly bounded native back link")
		}
		if strings.Contains(page, `name="new_code"`) != canWrite {
			t.Errorf("currency write controls visibility differs from canWrite=%v", canWrite)
		}
	}
}
