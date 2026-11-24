package organisms

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/web/components/atoms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestNavigationDestinationsHaveVisibleBoundaries(t *testing.T) {
	var sidebar bytes.Buffer
	if err := NavLink(viewmodel.PageData{}, "/account", "account.account", true).Render(context.Background(), &sidebar); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sidebar.String(), `border`) || !strings.Contains(sidebar.String(), `aria-current="page"`) || !strings.Contains(sidebar.String(), `href="/account"`) {
		t.Fatalf("active navigation must retain its destination and visible boundary: %s", sidebar.String())
	}
	var crumbs bytes.Buffer
	if err := atoms.Breadcrumbs(atoms.BreadcrumbsProps{Items: []atoms.BreadcrumbsProp{{Label: "Home", Href: "/"}, {Label: "Current"}}}).Render(context.Background(), &crumbs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(crumbs.String(), `class="btn`) || !strings.Contains(crumbs.String(), `href="/"`) {
		t.Fatalf("breadcrumb destination must have a visible boundary: %s", crumbs.String())
	}
}
