package clients

import (
	"errors"
	"testing"

	"example.com/dynamis-code/apps-template/internal/identity"
)

// TestListPagesCountTotalAndRows checks numbered pages: each page holds its slice of the sorted rows, the total
// counts every match, and a page past the end is empty.
func TestListPagesCountTotalAndRows(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		if _, err := f.service.Create(f.ctx, f.member, f.workspaceID, Input{DisplayName: name}, identity.AuditContext{}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[ClientID]bool{}
	for page := 1; page <= 3; page++ {
		got, err := f.service.List(f.ctx, f.member, f.workspaceID, ListInput{Page: page, Limit: 2})
		if err != nil || got.Total != 5 || got.Page != page {
			t.Fatalf("page %d = total %d, page %d, err %v; want total 5 and page %d", page, got.Total, got.Page, err, page)
		}
		want := 2
		if page == 3 {
			want = 1
		}
		if len(got.Clients) != want {
			t.Fatalf("page %d has %d rows, want %d", page, len(got.Clients), want)
		}
		for _, client := range got.Clients {
			if seen[client.ID] {
				t.Fatalf("client %d appears on two pages", client.ID)
			}
			seen[client.ID] = true
		}
	}
	past, err := f.service.List(f.ctx, f.member, f.workspaceID, ListInput{Page: 4, Limit: 2})
	if err != nil || len(past.Clients) != 0 || past.Total != 5 {
		t.Fatalf("page past the end = %d rows, total %d, err %v; want 0 rows, total 5", len(past.Clients), past.Total, err)
	}
	if _, err := f.service.List(f.ctx, f.member, f.workspaceID, ListInput{Page: -1, Limit: 2}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative page error = %v, want ErrInvalidInput", err)
	}
}
