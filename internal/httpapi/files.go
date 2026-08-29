package httpapi

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
)

type fileResponse struct {
	appfiles.File
	UploadURL   string `json:"uploadUrl,omitempty"`
	CompleteURL string `json:"completeUrl,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
}

func (h *handler) listFiles(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if h.files == nil || !validID(workspaceID) || !onlyQuery(request, "limit") {
		h.invalidRequest(writer, request, "The file parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	limit := h.cfg.DefaultPageSize
	if raw := request.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > h.cfg.MaxPageSize {
			h.invalidRequest(writer, request, "The file page limit is invalid.")
			return
		}
		limit = value
	}
	result, err := h.files.List(request.Context(), principal, workspaceID, limit)
	if err != nil {
		h.fileProblem(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"files": result})
}

func (h *handler) createFile(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if h.files == nil || !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The file parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		h.invalidRequest(writer, request, "The file content type is invalid.")
		return
	}
	var file appfiles.File
	switch mediaType {
	case "multipart/form-data":
		if err := request.ParseMultipartForm(1024 * 1024); err != nil {
			h.fileProblem(writer, request, appfiles.ErrInvalidInput)
			return
		}
		upload, header, err := request.FormFile("file")
		if err != nil {
			h.fileProblem(writer, request, appfiles.ErrInvalidInput)
			return
		}
		defer upload.Close()
		file, err = h.files.Upload(request.Context(), principal, workspaceID, header.Filename, upload, h.auditContext(request))
	case "application/json":
		var input struct {
			OriginalName string `json:"originalName"`
			Size         int64  `json:"size"`
			ContentType  string `json:"contentType"`
		}
		if err := decodeJSON(request, &input); err != nil {
			h.badJSON(writer, request, err)
			return
		}
		file, err = h.files.Initiate(request.Context(), principal, workspaceID, appfiles.InitiateInput{
			OriginalName: input.OriginalName, Size: input.Size, ContentType: input.ContentType,
		}, h.auditContext(request))
		if err == nil {
			fileResponse, urlErr := h.fileUploadResponse(request, file)
			if urlErr != nil {
				h.internal(writer, request)
				return
			}
			writer.Header().Set("Location", "/api/v1/workspaces/"+workspaceID+"/files/"+file.ID)
			writeJSON(writer, http.StatusCreated, fileResponse)
			return
		}
	default:
		h.invalidRequest(writer, request, "The file content type is unsupported.")
		return
	}
	if err != nil {
		h.fileProblem(writer, request, err)
		return
	}
	response := fileResponse{File: file}
	if file.Status == appfiles.Ready {
		response, err = h.fileDownloadResponse(request, principal, file)
		if err != nil {
			h.internal(writer, request)
			return
		}
	}
	writer.Header().Set("Location", "/api/v1/workspaces/"+workspaceID+"/files/"+file.ID)
	writeJSON(writer, http.StatusCreated, response)
}

func (h *handler) initiateFileUpload(writer http.ResponseWriter, request *http.Request) {
	h.createFile(writer, request)
}

func (h *handler) uploadFileContent(writer http.ResponseWriter, request *http.Request) {
	workspaceID, fileID := request.PathValue("workspaceId"), request.PathValue("fileId")
	if h.files == nil || !validID(workspaceID) || !validID(fileID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The file parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	file, err := h.files.PutContent(request.Context(), principal, workspaceID, fileID, request.Body, h.auditContext(request))
	if err != nil {
		h.fileProblem(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, fileResponse{File: file})
}

func (h *handler) completeFileUpload(writer http.ResponseWriter, request *http.Request) {
	workspaceID, fileID := request.PathValue("workspaceId"), request.PathValue("fileId")
	if h.files == nil || !validID(workspaceID) || !validID(fileID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The file parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesWrite)
	if !ok {
		return
	}
	file, err := h.files.Complete(request.Context(), principal, workspaceID, fileID, h.auditContext(request))
	if err != nil {
		h.fileProblem(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, fileResponse{File: file})
}

func (h *handler) getFile(writer http.ResponseWriter, request *http.Request) {
	workspaceID, fileID := request.PathValue("workspaceId"), request.PathValue("fileId")
	if h.files == nil || !validID(workspaceID) || !validID(fileID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The file parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	file, err := h.files.Get(request.Context(), principal, workspaceID, fileID)
	if err != nil {
		h.fileProblem(writer, request, err)
		return
	}
	response, err := h.fileDownloadResponse(request, principal, file)
	if err != nil {
		h.internal(writer, request)
		return
	}
	writer.Header().Set("Cache-Control", "private, no-store")
	writeJSON(writer, http.StatusOK, response)
}

func (h *handler) getFileContent(writer http.ResponseWriter, request *http.Request) {
	h.fileContent(writer, request)
}

func (h *handler) fileContent(writer http.ResponseWriter, request *http.Request) {
	workspaceID, fileID := request.PathValue("workspaceId"), request.PathValue("fileId")
	if h.files == nil || !validID(workspaceID) || !validID(fileID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The file parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.ResourcesRead)
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
		h.fileProblem(writer, request, err)
		return
	}
	file, reader, err := h.files.Open(request.Context(), principal, workspaceID, fileID)
	if err != nil {
		h.fileProblem(writer, request, err)
		return
	}
	defer reader.Close()
	writer.Header().Set("Content-Type", contentType(file))
	writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.OriginalName}))
	writer.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	writer.WriteHeader(http.StatusOK)
	_, _ = io.Copy(writer, reader)
}

func (h *handler) fileUploadResponse(request *http.Request, file appfiles.File) (fileResponse, error) {
	response := fileResponse{File: file, CompleteURL: "/api/v1/workspaces/" + file.WorkspaceID + "/files/" + file.ID + "/complete"}
	url, err := h.files.PresignedPut(request.Context(), file)
	if errors.Is(err, appfiles.ErrNotSupported) {
		response.UploadURL = "/api/v1/workspaces/" + file.WorkspaceID + "/files/" + file.ID + "/content"
		return response, nil
	}
	if err != nil {
		return fileResponse{}, err
	}
	response.UploadURL = url
	return response, nil
}

func (h *handler) fileDownloadResponse(request *http.Request, principal identity.Principal, file appfiles.File) (fileResponse, error) {
	response := fileResponse{File: file}
	_, url, err := h.files.PresignedGet(request.Context(), principal, file.WorkspaceID, file.ID)
	if errors.Is(err, appfiles.ErrNotSupported) {
		response.DownloadURL = "/api/v1/workspaces/" + file.WorkspaceID + "/files/" + file.ID + "/content"
		return response, nil
	}
	if err != nil {
		return fileResponse{}, err
	}
	response.DownloadURL = url
	return response, nil
}

func contentType(file appfiles.File) string {
	if file.DetectedMIME == nil {
		return "application/octet-stream"
	}
	return *file.DetectedMIME
}

func (h *handler) fileProblem(writer http.ResponseWriter, request *http.Request, err error) {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		writeProblem(writer, request, http.StatusRequestEntityTooLarge, "body-too-large", "The file request body exceeds the configured limit.")
		return
	}
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, appfiles.ErrNotFound):
		notFoundProblem(writer, request)
	case errors.Is(err, appfiles.ErrInvalidInput):
		h.invalidRequest(writer, request, "The file input is invalid.")
	case errors.Is(err, appfiles.ErrObjectLimit):
		writeProblem(writer, request, http.StatusRequestEntityTooLarge, "file-too-large", "The file exceeds the configured per-file limit.")
	case errors.Is(err, appfiles.ErrLimit):
		writeProblem(writer, request, http.StatusConflict, "storage-limit", "The workspace file storage limit was reached.")
	case errors.Is(err, appfiles.ErrNotReady):
		writeProblem(writer, request, http.StatusConflict, "file-not-ready", "The file upload is not ready.")
	default:
		h.internal(writer, request)
	}
}
