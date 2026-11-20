package web

import (
	"context"
	"errors"
	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

func fileListColumns() []viewmodel.ListColumn {
	return []viewmodel.ListColumn{
		{Key: "original_name", LabelKey: "files.name", DefaultVisible: true},
		{Key: "id", LabelKey: "ui.id"},
		{Key: "detected_mime", LabelKey: "files.type", DefaultVisible: true},
		{Key: "size", LabelKey: "files.size"},
		{Key: "created_at", LabelKey: "clients.created_at"},
	}
}

func fileSortHref(path string, values url.Values, sort string) string {
	copy := cloneClientQuery(values)
	copy.Del("page")
	if values.Get("sort") == sort {
		copy.Set("sort", "-"+sort)
	} else if values.Get("sort") == "-"+sort {
		copy.Set("sort", sort)
	} else {
		copy.Set("sort", sort)
	}
	return path + "?" + copy.Encode()
}

func fileListRows(files []appfiles.File) []viewmodel.ListRow {
	rows := make([]viewmodel.ListRow, 0, len(files))
	for index, file := range files {
		mimeType := ""
		if file.DetectedMIME != nil {
			mimeType = *file.DetectedMIME
		}
		rows = append(rows, viewmodel.ListRow{ID: file.ID.String(), ActionContext: file.OriginalName, ActionPosition: index + 1, Cells: map[string]viewmodel.ListCell{
			"original_name": viewmodel.TextCell(file.OriginalName), "detected_mime": viewmodel.TextCell(mimeType),
			"size": viewmodel.TextCell(strconv.FormatInt(file.Size, 10)), "created_at": viewmodel.TimeCell(file.CreatedAt),
		}})
	}
	return rows
}

func fileQuerySuffix(values url.Values) string { return clientQuerySuffix(values) }

func (h *Handler) EnableFilesPreferences(preferences *listprefs.Service) {
	h.listPreferences = preferences
	h.filesEnabled = h.files != nil
	h.registerListColumns(listviews.ListFiles, func(context.Context, identity.Principal, identity.WorkspaceID) ([]viewmodel.ListColumn, error) {
		return fileListColumns(), nil
	})
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
	h.afterListChange(writer, request, h.filesPath(workspaceID)+fileQuerySuffix(request.URL.Query()), h.rerenderFiles)
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
	// A lazy page sends the skeleton and loads the files in the results fragment.
	lazy := listLazyRequest(request)
	var page appfiles.Page
	var err error
	if !lazy {
		page, err = h.files.ListPage(request.Context(), principal, workspaceID, query.ListInput())
	}
	if err != nil {
		if errors.Is(err, appfiles.ErrInvalidInput) {
			h.renderError(writer, http.StatusBadRequest)
			return
		}
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	columns := fileListColumns()
	var orderErr error
	columns, orderErr = h.orderListColumns(request, principal, workspaceID, listprefs.ListFiles, columns)
	if orderErr != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
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
	filterView := buildListFilterView(request.URL.Path, request.URL.Query(), fileFilterFields, query.Filter)
	var resultsSrc, fullHref string
	if lazy {
		resultsSrc, fullHref = listResultsURLs(h.filesPath(workspaceID), request.URL.Query())
	}
	if h.openDefaultListView(writer, request, listviews.ListFiles, principal, workspaceID) {
		return
	}
	view := h.listViewState(request, principal, workspaceID, listviews.ListFiles, false)
	data := viewmodel.PageData{
		Title: "Files", TopbarTitleKey: "files.files", NavPage: "files", NavSection: "workspace", CSRF: csrf, ErrorKey: errorKey,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces,
		Files: page.Files, CurrentPath: h.filesPath(workspaceID),
		ListPager:   newListPager(request.URL.Path, request.URL.Query(), query.Page, query.Limit, page.Total),
		ListColumns: columns, ListRows: fileListRows(page.Files), ListTotal: page.Total, ListStart: listStart(query.Page, query.Limit), IsFilesList: true,
		ClientQuerySuffix: fileQuerySuffix(request.URL.Query()), FileListQuery: query, ListSearch: query.Search,
		ListPreservedQuery: filterView.Preserved,
		Filter:             filterView,
		ListLazy:           lazy, ListResultsSrc: resultsSrc, ListFullHref: fullHref, ListLive: true, ListFragment: listResultsFragment(request), ListViews: view.Views, ListViewQuery: view.Query, ListViewBase: view.Base, ListViewRoute: view.Route, ListViewCanShare: view.CanShare, ListViewSaveReason: view.SaveReason,
		FileMinSize: request.URL.Query().Get("min_size"), FileMaxSize: request.URL.Query().Get("max_size"),
		HasVisibleClientColumns: hasVisibleClientColumns(visible), HasClientFilters: query.Search != "" || query.MIME != "" || query.HasMinSize || query.HasMaxSize,
		ClearClientFiltersPath: fileClearFiltersPath(request.URL.Path, request.URL.Query()),
	}
	h.renderFilesTempl(writer, request, status, data, principal.Permissions[identity.ResourcesWrite])
}

func (h *Handler) renderFilesTempl(writer http.ResponseWriter, request *http.Request, status int, data viewmodel.PageData, canUpload bool) {
	data.FilesEnabled = h.filesEnabled
	data.ClientsEnabled = h.clients != nil
	data.SingleWorkspace = h.workspaceMode == config.WorkspaceModeSingle
	h.localizePage(writer, &data)
	data.Title = data.T("files.files")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	if listResultsFragment(request) {
		_ = fileResults(data).Render(request.Context(), writer)
		return
	}
	_ = filesPage(data, canUpload).Render(request.Context(), writer)
}

func fileClearFiltersPath(path string, query url.Values) string {
	values := cloneClientQuery(query)
	for _, key := range []string{"search", "mime", "min_size", "max_size", "page"} {
		values.Del(key)
	}
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
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
