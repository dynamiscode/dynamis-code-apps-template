package web

import (
	"net/http"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
)

// setFileColumnOrder saves the Files column order from the columns menu.
func (h *Handler) setFileColumnOrder(writer http.ResponseWriter, request *http.Request) {
	if h.files == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	h.storeColumnOrder(writer, request, listprefs.ListFiles,
		func(identity.Principal, identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
			return fileListColumns(), nil
		},
		h.filesPath(workspaceID)+fileQuerySuffix(request.URL.Query()),
		h.rerenderFiles)
}

func (h *Handler) rerenderFiles(writer http.ResponseWriter, request *http.Request) {
	h.renderFilesPage(writer, request, http.StatusOK, "")
}
