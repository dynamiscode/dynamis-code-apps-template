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
