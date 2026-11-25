package web

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestWorkspaceSwitcherStreamsItems(t *testing.T) {
	catalog, err := i18n.New()
	if err != nil {
		t.Fatal(err)
	}
	data := viewmodel.PageData{Catalog: catalog, Locale: "en", Workspace: identity.WorkspaceSummary{ID: 1, Name: "current", Role: identity.Member}}
	for i := 0; i < 1500; i++ {
		data.Workspaces = append(data.Workspaces, identity.WorkspaceSummary{ID: identity.WorkspaceID(i + 1), Name: fmt.Sprintf("member-%04d", i), Role: identity.Member})
	}
	var output memberStreamWriter
	if err := membersPage(data).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !output.seenThousand || !strings.Contains(output.String(), "member-1499") {
		t.Fatal("workspace switcher did not stream through item 1000 and finish")
	}
}
