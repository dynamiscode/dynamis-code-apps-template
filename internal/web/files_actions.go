package web

import organisms "example.com/dynamis-code/apps-template/internal/web/components/organisms"

import (
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// fileRowActions is a file row's menu. Files have no delete route in the web layer yet, so the menu opens the
// file, which downloads it.
func fileRowActions(data viewmodel.PageData, row viewmodel.ListRow) []organisms.ListAction {
	return []organisms.ListAction{
		{Label: data.T("ui.download"), Icon: "download", Href: data.CurrentPath + "/" + row.ID + "/content"},
	}
}
