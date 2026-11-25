package web

import (
	"testing"

	"github.com/a-h/templ"

	"example.com/dynamis-code/apps-template/internal/web/components/organisms"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

func TestFileListStreamsRows(t *testing.T) {
	assertListStreamsRows(t, func(data viewmodel.PageData) templ.Component { return fileListView(data, false) })
}

func TestFileListPreservesRendering(t *testing.T) {
	assertListPreservesRendering(t, func(data viewmodel.PageData) templ.Component { return fileListView(data, true) }, func(data viewmodel.PageData) templ.Component {
		limit := data.FileListQuery.Limit
		results := organisms.ListResults(data, limit, listPageSizeOptions(limit, listPageSizes), data.ClientQuerySuffix, fileList(data, true), templ.NopComponent)
		return organisms.ListScreen(data, "files.search", fileListTools(data), results)
	})
}
