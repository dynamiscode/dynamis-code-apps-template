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

func TestWorkspaceUpdateAndResourcesWriteRoles(t *testing.T) {
	if CanWriteCatalog(Principal{Role: Owner, Permissions: map[Permission]bool{ResourcesRead: true}}) {
		t.Fatal("catalog write granted without resources:write")
	}
	for _, role := range []Role{Owner, Admin, Member, Viewer} {
		permissions := permissionsForRole(role)
		if got, want := permissions[WorkspaceUpdate], role == Owner || role == Admin; got != want {
			t.Errorf("%s WorkspaceUpdate = %v, want %v", role, got, want)
		}
		if got, want := permissions[ResourcesWrite], role != Viewer; got != want {
			t.Errorf("%s ResourcesWrite = %v, want %v", role, got, want)
		}
		if got, want := CanWriteCatalog(Principal{Role: role, Permissions: permissions}), role == Owner || role == Admin; got != want {
			t.Errorf("%s catalog write = %v, want %v", role, got, want)
		}
	}
}
