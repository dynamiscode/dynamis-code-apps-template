package web

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
)

func (h *Handler) filesPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	fileList, err := h.files.List(request.Context(), principal, workspaceID, h.cfg.MaxPageSize)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusOK, "files.html", pageData{
		Title: "Files", NavPage: "files", NavSection: "workspace", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		Files:       fileList,
		CurrentPath: "/workspaces/" + workspaceID + "/files",
	})
}

func (h *Handler) filesUpload(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	principal, _, _, ok := h.managementPrincipal(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	upload, header, err := request.FormFile("file")
	if err != nil {
		h.renderFilesError(writer, request, "Choose a supported file to upload.")
		return
	}
	defer upload.Close()
	if _, err := h.files.Upload(request.Context(), principal, workspaceID, header.Filename, upload, auditContext(request)); err != nil {
		message := "The file could not be uploaded."
		switch {
		case errors.Is(err, appfiles.ErrObjectLimit):
			message = "The file exceeds the per-file size limit."
		case errors.Is(err, appfiles.ErrLimit):
			message = "The workspace file storage limit was reached."
		}
		h.renderFilesError(writer, request, message)
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID+"/files")
}

func (h *Handler) renderFilesError(writer http.ResponseWriter, request *http.Request, message string) {
	workspaceID := request.PathValue("workspaceId")
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	fileList, err := h.files.List(request.Context(), principal, workspaceID, h.cfg.MaxPageSize)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, http.StatusUnprocessableEntity, "files.html", pageData{
		Title: "Files", NavPage: "files", NavSection: "workspace", CSRF: csrf, Error: message,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		Files: fileList, CurrentPath: "/workspaces/" + workspaceID + "/files",
	})
}

func (h *Handler) fileDownload(writer http.ResponseWriter, request *http.Request) {
	workspaceID, fileID := request.PathValue("workspaceId"), request.PathValue("fileId")
	principal, _, _, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	file, url, err := h.files.PresignedGet(request.Context(), principal, workspaceID, fileID)
	if err == nil {
		writer.Header().Set("Cache-Control", "private, no-store")
		http.Redirect(writer, request, url, http.StatusFound)
		return
	}
	if !errors.Is(err, appfiles.ErrNotSupported) {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	file, reader, err := h.files.Open(request.Context(), principal, workspaceID, fileID)
	if err != nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	defer reader.Close()
	writer.Header().Set("Content-Type", fileMIME(file))
	writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.OriginalName}))
	writer.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	writer.WriteHeader(http.StatusOK)
	_, _ = io.Copy(writer, reader)
}

func fileMIME(file appfiles.File) string {
	if file.DetectedMIME == nil {
		return "application/octet-stream"
	}
	return *file.DetectedMIME
}
