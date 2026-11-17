package web

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
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

func fileListColumns() []clientListColumn {
	return []clientListColumn{
		{Key: "original_name", LabelKey: "files.name", ValueType: "text", DefaultVisible: true},
		{Key: "detected_mime", LabelKey: "files.type", ValueType: "text", DefaultVisible: true},
		{Key: "size", LabelKey: "files.size", ValueType: "number"},
		{Key: "created_at", LabelKey: "clients.created_at", ValueType: "timestamp"},
	}
}

func parseFileListQuery(values url.Values, defaultLimit int) (fileListQuery, bool) {
	query := fileListQuery{Search: strings.TrimSpace(values.Get("search")), MIME: strings.TrimSpace(values.Get("mime")), Sort: values.Get("sort"), Limit: defaultLimit, Cursor: values.Get("cursor")}
	for key, entries := range values {
		if len(entries) != 1 {
			return query, false
		}
		switch key {
		case "search", "mime", "sort", "cursor":
		case "min_size", "max_size":
			if entries[0] == "" {
				continue
			}
			size, err := strconv.ParseInt(entries[0], 10, 64)
			if err != nil || size < 0 {
				return query, false
			}
			if key == "min_size" {
				query.MinSize, query.HasMinSize = size, true
			} else {
				query.MaxSize, query.HasMaxSize = size, true
			}
		case "limit":
			limit, err := strconv.Atoi(entries[0])
			if err != nil || limit < 1 || limit > 100 {
				return query, false
			}
			query.Limit = limit
		default:
			return query, false
		}
	}
	if len(query.Search) > 200 || len(query.MIME) > 128 || len(query.Cursor) > 64 || query.HasMaxSize && query.HasMinSize && query.MinSize > query.MaxSize {
		return query, false
	}
	if query.Sort == "" {
		query.Sort = "-created_at"
	}
	if query.Sort != "original_name" && query.Sort != "-original_name" && query.Sort != "size" && query.Sort != "-size" && query.Sort != "created_at" && query.Sort != "-created_at" {
		return query, false
	}
	return query, true
}

func fileSortHref(path string, values url.Values, sort string) string {
	copy := cloneClientQuery(values)
	copy.Del("cursor")
	if values.Get("sort") == sort {
		copy.Set("sort", "-"+sort)
	} else if values.Get("sort") == "-"+sort {
		copy.Set("sort", sort)
	} else {
		copy.Set("sort", sort)
	}
	return path + "?" + copy.Encode()
}

func fileListRows(files []appfiles.File) []clientListRow {
	rows := make([]clientListRow, 0, len(files))
	for index, file := range files {
		mimeType := ""
		if file.DetectedMIME != nil {
			mimeType = *file.DetectedMIME
		}
		rows = append(rows, clientListRow{ID: file.ID.String(), ActionContext: file.OriginalName, ActionPosition: index + 1, Values: map[string]string{
			"original_name": file.OriginalName, "detected_mime": mimeType, "size": strconv.FormatInt(file.Size, 10), "created_at": file.CreatedAt.Format(time.RFC3339),
		}})
	}
	return rows
}

func fileQuerySuffix(values url.Values) string { return clientQuerySuffix(values) }

func (h *Handler) EnableFilesPreferences(preferences *listprefs.Service) {
	h.listPreferences = preferences
	h.filesEnabled = h.files != nil
}

func (h *Handler) setFileColumn(writer http.ResponseWriter, request *http.Request) {
	h.updateFileColumns(writer, request, false)
}

func (h *Handler) resetFileColumns(writer http.ResponseWriter, request *http.Request) {
	h.updateFileColumns(writer, request, true)
}

func (h *Handler) updateFileColumns(writer http.ResponseWriter, request *http.Request, reset bool) {
	if h.files == nil || h.listPreferences == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	principal, ok := h.clientPreferencePrincipal(writer, request)
	if !ok {
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	defaults := make(map[string]bool)
	for _, column := range fileListColumns() {
		defaults[column.Key] = column.DefaultVisible
	}
	var err error
	if reset {
		err = h.listPreferences.Reset(request.Context(), principal, workspaceID, listprefs.ListFiles)
	} else {
		visible, parseErr := strconv.ParseBool(request.FormValue("visible"))
		if parseErr != nil {
			h.renderError(writer, http.StatusBadRequest)
			return
		}
		err = h.listPreferences.Set(request.Context(), principal, workspaceID, listprefs.ListFiles, request.PathValue("column"), visible, defaults)
	}
	if err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	h.redirect(writer, request, h.filesPath(workspaceID)+fileQuerySuffix(request.URL.Query()))
}

func (h *Handler) filesPath(workspaceID identity.WorkspaceID) string {
	return "/workspaces/" + workspaceID.String() + "/files"
}

func (h *Handler) filesPage(writer http.ResponseWriter, request *http.Request) {
	h.renderFilesPage(writer, request, http.StatusOK, "")
}

func (h *Handler) renderFilesPage(writer http.ResponseWriter, request *http.Request, status int, errorKey string) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.ResourcesRead)
	if !ok {
		return
	}
	query, valid := parseFileListQuery(request.URL.Query(), h.cfg.DefaultPageSize)
	if !valid {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	page, err := h.files.ListPage(request.Context(), principal, workspaceID, query.listInput())
	if err != nil {
		if errors.Is(err, appfiles.ErrInvalidInput) {
			h.renderError(writer, http.StatusBadRequest)
			return
		}
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	columns := fileListColumns()
	defaults := make(map[string]bool, len(columns))
	for _, column := range columns {
		defaults[column.Key] = column.DefaultVisible
	}
	visible := defaults
	if h.listPreferences != nil {
		visible, err = h.listPreferences.Get(request.Context(), principal, workspaceID, listprefs.ListFiles, defaults)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
	}
	for index := range columns {
		columns[index].Visible = visible[columns[index].Key]
		columns[index].SortHref = fileSortHref(h.filesPath(workspaceID), request.URL.Query(), columns[index].Key)
		if strings.TrimPrefix(query.Sort, "-") == columns[index].Key {
			columns[index].SortDirection = "ascending"
			if strings.HasPrefix(query.Sort, "-") {
				columns[index].SortDirection = "descending"
			}
		}
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	h.render(writer, status, "files.html", pageData{
		Title: "Files", NavPage: "files", NavSection: "workspace", CSRF: csrf, ErrorKey: errorKey,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		Files: page.Files, CurrentPath: h.filesPath(workspaceID), NextCursor: page.NextCursor,
		NextPath:          fileNextPath(request.URL.Path, request.URL.Query(), page.NextCursor),
		ClientListColumns: columns, ClientListRows: fileListRows(page.Files), IsFilesList: true,
		ClientQuerySuffix: fileQuerySuffix(request.URL.Query()), FileListQuery: query,
		FileMinSize: request.URL.Query().Get("min_size"), FileMaxSize: request.URL.Query().Get("max_size"),
		HasVisibleClientColumns: hasVisibleClientColumns(visible), HasClientFilters: query.Search != "" || query.MIME != "" || query.HasMinSize || query.HasMaxSize,
		ClearClientFiltersPath: fileClearFiltersPath(request.URL.Path, request.URL.Query()),
	})
}

func fileNextPath(path string, query url.Values, cursor string) string {
	if cursor == "" {
		return ""
	}
	values := cloneClientQuery(query)
	values.Set("cursor", cursor)
	return path + "?" + values.Encode()
}

func fileClearFiltersPath(path string, query url.Values) string {
	values := cloneClientQuery(query)
	for _, key := range []string{"search", "mime", "min_size", "max_size", "cursor"} {
		values.Del(key)
	}
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func (query fileListQuery) listInput() appfiles.ListInput {
	input := appfiles.ListInput{Limit: query.Limit, Cursor: query.Cursor, Search: query.Search, MIME: query.MIME, Sort: strings.TrimPrefix(query.Sort, "-"), Descending: strings.HasPrefix(query.Sort, "-")}
	if query.HasMinSize {
		input.MinSize = &query.MinSize
	}
	if query.HasMaxSize {
		input.MaxSize = &query.MaxSize
	}
	return input
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
			h.renderFilesError(writer, request, "files.error_oversize")
			return
		}
		h.renderFilesError(writer, request, "files.error_choose")
		return
	}
	// ParseMultipartForm fills the form values; ParseForm would not read the CSRF field here.
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	upload, header, err := request.FormFile("file")
	if err != nil {
		h.renderFilesError(writer, request, "files.error_choose")
		return
	}
	defer upload.Close()
	if _, err := h.files.Upload(request.Context(), principal, workspaceID, header.Filename, upload, auditContext(request)); err != nil {
		errorKey := "files.error_upload"
		switch {
		case errors.Is(err, appfiles.ErrObjectLimit):
			errorKey = "files.error_oversize"
		case errors.Is(err, appfiles.ErrLimit):
			errorKey = "files.error_workspace_limit"
		case errors.Is(err, appfiles.ErrInvalidInput):
			errorKey = "files.error_invalid"
		}
		h.renderFilesError(writer, request, errorKey)
		return
	}
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/files")
}

func (h *Handler) renderFilesError(writer http.ResponseWriter, request *http.Request, errorKey string) {
	h.renderFilesPage(writer, request, http.StatusUnprocessableEntity, errorKey)
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
