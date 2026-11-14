package identity

import "testing"

// The shared policy grants nothing unless a role table lists it. An unknown
// role or an unlisted permission must produce an empty grant.
func TestPolicyGrantsNothingForUnknownRoleOrUnlistedPermission(t *testing.T) {
	t.Parallel()
	if validRole("guest") {
		t.Fatal(`validRole("guest") = true, want false`)
	}
	if got := permissionsForRole("guest"); len(got) != 0 {
		t.Fatalf("permissionsForRole(guest) = %v, want no grants", got)
	}
	unlisted := Permission("clients.unlisted")
	for _, role := range []Role{Owner, Admin, Member, Viewer} {
		if permissionsForRole(role)[unlisted] {
			t.Fatalf("role %s grants unlisted permission %q", role, unlisted)
		}
	}
	if permissionsForRole(Viewer)[ResourcesWrite] {
		t.Fatal("viewer holds ResourcesWrite")
	}
}
