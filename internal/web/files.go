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

const (
	// uploadMultipartMemory is how much of a multipart upload stays in memory
	// before it spills to a temporary file.
	uploadMultipartMemory = 1 << 20
	// uploadFramingBytes allows for multipart boundaries and the CSRF field
	// around a file at the per-object limit. The service enforces the exact
	// object limit; this only bounds how much request body is buffered.
	uploadFramingBytes = 1 << 20
)

func (h *Handler) filesPage(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
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
		CurrentPath: "/workspaces/" + workspaceID.String() + "/files",
	})
}

// filesUpload accepts the one browser upload request: an ordinary multipart
// form with the CSRF token and the file. The browser uploader and the plain
// form both post here.
func (h *Handler) filesUpload(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, _, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	if h.cfg.FileMaxBodyBytes > 0 {
		request.Body = http.MaxBytesReader(writer, request.Body, h.cfg.FileMaxBodyBytes+uploadFramingBytes)
	}
	if err := request.ParseMultipartForm(uploadMultipartMemory); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.renderFilesError(writer, request, "The file exceeds the per-file size limit.")
			return
		}
		h.renderFilesError(writer, request, "Choose a supported file to upload.")
		return
	}
	// ParseMultipartForm fills the form values; ParseForm would not read the CSRF field here.
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
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
		case errors.Is(err, appfiles.ErrInvalidInput):
			message = "The selected file is invalid."
		}
		h.renderFilesError(writer, request, message)
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/files")
}

func (h *Handler) renderFilesError(writer http.ResponseWriter, request *http.Request, message string) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
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
		Files: fileList, CurrentPath: "/workspaces/" + workspaceID.String() + "/files",
	})
}

func (h *Handler) fileDownload(writer http.ResponseWriter, request *http.Request) {
	workspaceID, fileID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId")), appfiles.ParseFileIDOrZero(request.PathValue("fileId"))
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
	if errors.Is(err, appfiles.ErrNotFound) || errors.Is(err, appfiles.ErrInvalidInput) {
		h.renderError(writer, http.StatusNotFound)
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
