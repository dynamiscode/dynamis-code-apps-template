package identity

import "testing"

// Expected grants follow the workspace permissions table in docs/authentication.md.
var authorizationMatrix = map[Permission][4]bool{
	WorkspaceRead:     {true, true, true, true},
	MembersRead:       {true, true, true, true},
	ResourcesRead:     {true, true, true, true},
	ResourcesWrite:    {true, true, true, false},
	WorkspaceUpdate:   {true, true, false, false},
	MembersManage:     {true, true, false, false},
	InvitationsManage: {true, true, false, false},
	WorkspaceExport:   {true, true, false, false},
	WorkspaceDelete:   {true, false, false, false},
	OwnershipTransfer: {true, false, false, false},
	SCIMManage:        {true, true, false, false},
}

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

func TestTransactionsPolicy(t *testing.T) {
	for _, role := range []Role{Owner, Admin, Member, Viewer, "guest"} {
		principal := Principal{Role: role, Permissions: permissionsForRole(role)}
		want := role == Owner || role == Admin || role == Member
		if CanReadTransactions(principal) != want || CanWriteTransactions(principal) != want {
			t.Errorf("%s transactions read/write = %v/%v, want %v", role, CanReadTransactions(principal), CanWriteTransactions(principal), want)
		}
		if got, want := CanManageTransactions(principal), role == Owner || role == Admin; got != want {
			t.Errorf("%s transactions management = %v, want %v", role, got, want)
		}
	}
	if CanReadTransactions(Principal{Role: Owner}) || CanWriteTransactions(Principal{Role: Owner}) {
		t.Fatal("transactions access granted without credential scope")
	}
}
