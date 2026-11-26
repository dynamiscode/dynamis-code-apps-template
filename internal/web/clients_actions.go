package web

import organisms "example.com/dynamis-code/apps-template/internal/web/components/organisms"

import (
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// clientRowActions is a client row's menu. An active client can be opened, edited or deleted. An archived
// client can be opened, restored or deleted. Archiving keeps the client and its values; deleting removes them.
func clientRowActions(data viewmodel.PageData, row viewmodel.ListRow) []organisms.ListAction {
	href := data.CurrentPath + "/" + row.ID
	open := organisms.ListAction{Label: data.T("ui.open"), Icon: "open", Href: clientDetailHref(data, row.ID)}
	remove := organisms.ListAction{Label: data.T("clients.delete_client"), Icon: "delete", PostPath: href + "/delete", Confirm: data.T("clients.delete_client_confirm"), Danger: true, ClientDeleteName: row.CellText(data.ClientListLinkKey())}
	// Each action appears only when the viewer may use it; the server enforces the same rules.
	actions := []organisms.ListAction{open}
	if row.Archived {
		// An archived client is read-only until it is restored, so its menu offers restore, open and delete only.
		if data.CanEditClients {
			actions = append(actions, organisms.ListAction{Label: data.T("clients.unarchive"), Icon: "unarchive", PostPath: href + "/unarchive"})
		}
	} else {
		if data.CanEditClients {
			actions = append(actions, organisms.ListAction{Label: data.T("ui.edit"), Icon: "edit", Href: clientDetailHref(data, row.ID)})
		}
	}
	if data.CanDeleteClients {
		actions = append(actions, remove)
	}
	return actions
}
