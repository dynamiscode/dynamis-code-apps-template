package web

import (
	"example.com/dynamis-code/apps-template/internal/web/components"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// clientRowActions is a client row's menu. An active client can be opened, edited, archived or deleted. An archived
// client can be opened, restored or deleted. Archiving keeps the client and its values; deleting removes them.
func clientRowActions(data viewmodel.PageData, row viewmodel.ListRow) []components.ListAction {
	href := data.CurrentPath + "/" + row.ID
	open := components.ListAction{Label: data.T("ui.open"), Icon: "open", Href: href}
	remove := components.ListAction{Label: data.T("clients.delete_client"), Icon: "delete", PostPath: href + "/delete", Confirm: data.T("clients.delete_client_confirm"), Danger: true}
	// Each action appears only when the viewer may use it; the server enforces the same rules.
	actions := []components.ListAction{open}
	if row.Archived {
		// An archived client is read-only until it is restored, so its menu offers restore, open and delete only.
		if data.CanEditClients {
			actions = append(actions, components.ListAction{Label: data.T("clients.unarchive"), Icon: "unarchive", PostPath: href + "/unarchive"})
		}
	} else {
		if data.CanEditClients {
			actions = append(actions,
				components.ListAction{Label: data.T("ui.edit"), Icon: "edit", Href: href},
				components.ListAction{Label: data.T("clients.archive"), Icon: "archive", PostPath: href + "/archive"},
			)
		}
	}
	if data.CanDeleteClients {
		actions = append(actions, remove)
	}
	return actions
}
