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
	if err := sidebarNavLi(viewmodel.PageData{}, "/account", "account.account", true).Render(context.Background(), &sidebar); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sidebar.String(), `class="menu-active"`) || !strings.Contains(sidebar.String(), `aria-current="page"`) || !strings.Contains(sidebar.String(), `href="/account"`) || strings.Contains(sidebar.String(), `border`) || strings.Contains(sidebar.String(), `btn`) {
		t.Fatalf("sidebar navigation must use a daisyUI current-page link: %s", sidebar.String())
	}
	sidebar.Reset()
	if err := sidebarNavLi(viewmodel.PageData{}, "/account", "account.account", false).Render(context.Background(), &sidebar); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sidebar.String(), `menu-active`) || strings.Contains(sidebar.String(), `aria-current="page"`) || !strings.Contains(sidebar.String(), `href="/account"`) {
		t.Fatalf("inactive sidebar destination must not be marked current: %s", sidebar.String())
	}
	var crumbs bytes.Buffer
	if err := atoms.Breadcrumbs(atoms.BreadcrumbsProps{Items: []atoms.BreadcrumbsProp{{Label: "Home", Href: "/"}, {Label: "Current"}}}).Render(context.Background(), &crumbs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(crumbs.String(), `href="/"`) || !strings.Contains(crumbs.String(), `class="link inline-flex min-h-11`) || !strings.Contains(crumbs.String(), `focus-visible:outline-2`) || !strings.Contains(crumbs.String(), `<span aria-current="page" class="break-words font-semibold">Current</span>`) || strings.Contains(crumbs.String(), `border`) || strings.Contains(crumbs.String(), `class="btn`) {
		t.Fatalf("breadcrumbs must use plain links with visible focus and a current page: %s", crumbs.String())
	}
}
