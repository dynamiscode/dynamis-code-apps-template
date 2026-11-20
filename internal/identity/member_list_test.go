package identity

import "testing"

func TestMemberSearchMatchesEmailAndRoleIgnoringCase(t *testing.T) {
	member := MemberSummary{Email: "Ada@Example.test", Role: Admin}
	for _, search := range []string{"ada", "EXAMPLE", "admin"} {
		if !MemberSearchMatches(member, search) {
			t.Fatalf("search %q should match", search)
		}
	}
	if MemberSearchMatches(member, "viewer") {
		t.Fatal("search for a role the member does not have should not match")
	}
}

func TestSortMembersByFieldWithReverse(t *testing.T) {
	members := []MemberSummary{{Email: "b@x.test", Role: Viewer}, {Email: "a@x.test", Role: Admin}}
	if got := SortMembers(append([]MemberSummary(nil), members...), "email"); got[0].Email != "a@x.test" {
		t.Fatalf("email sort first = %q", got[0].Email)
	}
	if got := SortMembers(append([]MemberSummary(nil), members...), "-email"); got[0].Email != "b@x.test" {
		t.Fatalf("reverse email sort first = %q", got[0].Email)
	}
	if got := SortMembers(append([]MemberSummary(nil), members...), "role"); got[0].Role != Admin {
		t.Fatalf("role sort first = %q", got[0].Role)
	}
}
