package molecules

import (
	"testing"

	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestListColumnOrderAfterMove(t *testing.T) {
	data := viewmodel.PageData{ListColumns: []viewmodel.ListColumn{{Key: "name"}, {Key: "email"}, {Key: "status"}}}
	cases := []struct {
		name  string
		index int
		delta int
		want  string
	}{
		{"move second column down", 1, 1, "name,status,email"},
		{"move last column up", 2, -1, "name,status,email"},
		{"first column is locked", 0, 1, "name,email,status"},
		{"second column cannot move above the locked name", 1, -1, "name,email,status"},
		{"last column cannot move down", 2, 1, "name,email,status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ListColumnOrderAfterMove(data, tc.index, tc.delta); got != tc.want {
				t.Fatalf("ListColumnOrderAfterMove(%d, %d) = %q, want %q", tc.index, tc.delta, got, tc.want)
			}
		})
	}
}
