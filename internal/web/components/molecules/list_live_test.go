package molecules

import (
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
