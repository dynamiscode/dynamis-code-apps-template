package identity

import "slices"

var rolePermissions = map[Role][]Permission{
	Owner: {
		WorkspaceRead,
		WorkspaceUpdate,
		WorkspaceDelete,
		WorkspaceExport,
		OwnershipTransfer,
		MembersRead,
		MembersManage,
		InvitationsManage,
		WebhooksRead,
		WebhooksManage,
		ResourcesRead,
		ResourcesWrite,
		SCIMManage,
	},
	Admin: {
		WorkspaceRead,
		WorkspaceUpdate,
		WorkspaceExport,
		MembersRead,
		MembersManage,
		InvitationsManage,
		WebhooksRead,
		WebhooksManage,
		ResourcesRead,
		ResourcesWrite,
		SCIMManage,
	},
	Member: {
		WorkspaceRead,
		MembersRead,
		ResourcesRead,
		ResourcesWrite,
	},
	Viewer: {
		WorkspaceRead,
		MembersRead,
		ResourcesRead,
	},
}

// CanWriteCatalog applies catalog's owner/admin rule to an authorized principal.
func CanWriteCatalog(principal Principal) bool {
	return principal.Permissions[ResourcesWrite] && permissionsForRole(principal.Role)[WorkspaceUpdate]
}

// CanManageScheduling applies the owner/admin rule to scheduling configuration.
func CanManageScheduling(principal Principal) bool {
	return principal.Permissions[ResourcesWrite] && permissionsForRole(principal.Role)[WorkspaceUpdate]
}

// CanReadTransactions restricts transactions entries to workspace staff with read scope.
func CanReadTransactions(principal Principal) bool {
	return principal.Permissions[ResourcesRead] && (principal.Role == Owner || principal.Role == Admin || principal.Role == Member)
}

// CanWriteTransactions restricts transactions creation to workspace staff with write scope.
func CanWriteTransactions(principal Principal) bool {
	return principal.Permissions[ResourcesWrite] && (principal.Role == Owner || principal.Role == Admin || principal.Role == Member)
}

// CanManageTransactions restricts transactions configuration to workspace administrators.
func CanManageTransactions(principal Principal) bool {
	return principal.Permissions[ResourcesWrite] && permissionsForRole(principal.Role)[WorkspaceUpdate]
}

// CanVoidTransactions restricts corrections to owners and administrators.
func CanVoidTransactions(principal Principal) bool {
	return principal.Permissions[ResourcesWrite] && (principal.Role == Owner || principal.Role == Admin)
}

func permissionsForRole(role Role) map[Permission]bool {
	permissions := make(map[Permission]bool, len(rolePermissions[role]))
	for _, permission := range rolePermissions[role] {
		permissions[permission] = true
	}
	return permissions
}

func validRole(role Role) bool {
	_, ok := rolePermissions[role]
	return ok
}

func validInvitationRole(role Role) bool {
	return role == Admin || role == Member || role == Viewer
}

func validScopes(scopes []Permission, allowed map[Permission]bool) bool {
	if len(scopes) == 0 {
		return false
	}
	seen := make(map[Permission]bool, len(scopes))
	for _, scope := range scopes {
		if seen[scope] || !allowed[scope] {
			return false
		}
		seen[scope] = true
	}
	return true
}

func normalizeScopes(scopes []Permission) []Permission {
	result := append([]Permission(nil), scopes...)
	slices.Sort(result)
	return result
}
