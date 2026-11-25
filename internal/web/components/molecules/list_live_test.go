package molecules

import (
	"context"
	"strings"
	"testing"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// TestListResultsHrefKeepsQuery checks that a sort link asks for the results fragment and keeps the list query.
func TestListResultsHrefKeepsQuery(t *testing.T) {
	got := listResultsHref("/workspaces/1/clients?sort=name&limit=10")
	want := "/workspaces/1/clients?limit=10&part=results&sort=name"
	if got != want {
		t.Fatalf("listResultsHref = %q, want %q", got, want)
	}
}

func TestClientSortLinkPreservesLiveAndNativeNavigation(t *testing.T) {
	column := viewmodel.ListColumn{Label: "Name", SortHref: "/clients?sort=name"}
	for _, live := range []bool{false, true} {
		var output strings.Builder
		if err := clientSortLink(viewmodel.PageData{ListLive: live}, column).Render(context.Background(), &output); err != nil {
			t.Fatal(err)
		}
		html := output.String()
		if !strings.Contains(html, `href="/clients?sort=name"`) {
			t.Fatalf("native href missing: %s", html)
		}
		for _, attr := range []string{`hx-get="/clients?part=results&amp;sort=name"`, `hx-target="#list-results"`, `hx-swap="outerHTML"`, `hx-push-url="true"`, `hx-sync="this:replace"`} {
			if strings.Contains(html, attr) != live {
				t.Errorf("live=%t: %s in %s", live, attr, html)
			}
		}
	}
}

// TestListLiveAttributesOffWithoutLiveList checks that lists without live updates keep plain links.
func TestListLiveAttributesOffWithoutLiveList(t *testing.T) {
	data := viewmodel.PageData{CurrentPath: "/workspaces/1/clients"}
	if got := ListLiveLinkAttributes(data, "/workspaces/1/clients?sort=name"); got != nil {
		t.Fatalf("ListLiveLinkAttributes with ListLive off = %v, want nil", got)
	}
	if got := ListLivePageSizeAttributes(data); got != nil {
		t.Fatalf("ListLivePageSizeAttributes with ListLive off = %v, want nil", got)
	}
}
