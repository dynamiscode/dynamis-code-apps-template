package web

import (
	"errors"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
	"example.com/dynamis-code/apps-template/internal/listviews"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func memberListColumns() []viewmodel.ListColumn {
	return []viewmodel.ListColumn{
		{Key: "id", LabelKey: "ui.id"},
		{Key: "email", LabelKey: "common.email", DefaultVisible: true},
		{Key: "role", LabelKey: "members.role", DefaultVisible: true},
		{Key: "created_at", LabelKey: "clients.created_at"},
	}
}

func (h *Handler) renderMembersList(writer http.ResponseWriter, request *http.Request) {
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, csrf, ok := h.workspaceSession(writer, request, workspaceID, identity.MembersRead)
	if !ok {
		return
	}
	query, valid := parseMemberListQuery(request.URL.Query(), h.cfg.DefaultPageSize)
	if !valid {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	members, err := h.identity.ListMembers(request.Context(), principal)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	members = filterAndSortMembers(members, query)
	total := len(members)
	page := max(query.Page, 1)
	if query.Limit > 0 {
		from := min((page-1)*query.Limit, len(members))
		to := min(from+query.Limit, len(members))
		members = members[from:to]
	}
	start := listStart(page, query.Limit)
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	columns := memberListColumns()
	var orderErr error
	columns, orderErr = h.orderListColumns(request, principal, workspaceID, listprefs.ListMembers, columns)
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
		visible, err = h.listPreferences.Get(request.Context(), principal, workspaceID, listprefs.ListMembers, defaults)
		if err != nil {
			h.renderError(writer, http.StatusInternalServerError)
			return
		}
	}
	for index := range columns {
		columns[index].Visible = visible[columns[index].Key]
		columns[index].SortHref = memberSortHref(request.URL.Path, request.URL.Query(), columns[index].Key)
		if strings.TrimPrefix(query.Sort, "-") == columns[index].Key {
			columns[index].SortDirection = "ascending"
			if strings.HasPrefix(query.Sort, "-") {
				columns[index].SortDirection = "descending"
			}
		}
	}
	rows := memberListRows(members)
	filterView := buildListFilterView(request.URL.Path, request.URL.Query(), memberFilterFields, query.Filter)
	pager := newListPager(request.URL.Path, request.URL.Query(), page, query.Limit, total)
	formValues := cloneClientQuery(request.URL.Query())
	formValues.Del("page")
	querySuffix := ""
	if encoded := formValues.Encode(); encoded != "" {
		querySuffix = "?" + encoded
	}
	if h.openDefaultListView(writer, request, listviews.ListMembers, principal, workspaceID) {
		return
	}
	view := h.listViewState(request, principal, workspaceID, listviews.ListMembers, false)
	h.render(writer, http.StatusOK, "members.html", viewmodel.PageData{
		Title: "Members", TopbarTitleKey: "members.members", NavPage: "members", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces, Members: members,
		CanManage: principal.Permissions[identity.MembersManage], CanTransfer: principal.Permissions[identity.OwnershipTransfer],
		CurrentPath: request.URL.Path, MemberTotal: total, ListPager: pager,
		ListColumns: columns, ListRows: rows, ListTotal: total, ListStart: start, Filter: filterView,
		HasVisibleClientColumns: hasVisibleClientColumns(visible), HasClientFilters: len(query.Filter.Filters) > 0,
		ClearClientFiltersPath: filterView.ClearHref, ClientQuerySuffix: querySuffix, IsMembersList: true,
		MemberListQuery: query, ListSearch: query.Search, ListPreservedQuery: filterView.Preserved,
		ListLive: true, ListFragment: listResultsFragment(request), ListViews: view.Views, ListViewQuery: view.Query, ListViewBase: view.Base, ListViewRoute: view.Route, ListViewCanShare: view.CanShare, ListViewSaveReason: view.SaveReason,
	})
}

func filterAndSortMembers(members []identity.MemberSummary, query viewmodel.MemberListQuery) []identity.MemberSummary {
	filtered := make([]identity.MemberSummary, 0, len(members))
	for _, member := range members {
		if query.Search != "" && !identity.MemberSearchMatches(member, query.Search) {
			continue
		}
		if matchListFilters(query.Filter.Filters, query.Filter.Match, func(filter viewmodel.ListFilter) bool {
			return memberMatchesFilter(member, filter)
		}) {
			filtered = append(filtered, member)
		}
	}
	return identity.SortMembers(filtered, query.Sort)
}

func memberListRows(members []identity.MemberSummary) []viewmodel.ListRow {
	rows := make([]viewmodel.ListRow, 0, len(members))
	for index, member := range members {
		rows = append(rows, viewmodel.ListRow{ID: member.ID.String(), Member: member, ActionContext: member.Email, ActionPosition: index + 1,
			Cells: map[string]viewmodel.ListCell{"email": viewmodel.TextCell(member.Email), "role": viewmodel.RoleCell(member.Role), "created_at": viewmodel.TimeCell(member.CreatedAt)}})
	}
	return rows
}

func memberSortHref(path string, query url.Values, key string) string {
	copy := cloneClientQuery(query)
	copy.Del("page")
	current := query.Get("sort")
	if current == "" {
		current = "email"
	}
	if current == key {
		copy.Set("sort", "-"+key)
	} else {
		copy.Set("sort", key)
	}
	return path + "?" + copy.Encode()
}

func (h *Handler) setMemberColumn(writer http.ResponseWriter, request *http.Request) {
	h.memberColumnPreference(writer, request, false)
}

func (h *Handler) resetMemberColumns(writer http.ResponseWriter, request *http.Request) {
	h.memberColumnPreference(writer, request, true)
}

func (h *Handler) memberColumnPreference(writer http.ResponseWriter, request *http.Request, reset bool) {
	if h.listPreferences == nil {
		h.renderError(writer, http.StatusNotFound)
		return
	}
	if err := request.ParseForm(); err != nil {
		h.renderError(writer, http.StatusBadRequest)
		return
	}
	workspaceID := identity.ParseWorkspaceIDOrZero(request.PathValue("workspaceId"))
	principal, session, _, ok := h.workspaceSession(writer, request, workspaceID, identity.MembersRead)
	if !ok {
		return
	}
	if !h.identity.VerifyCSRF(request.Context(), session.ID, request.FormValue("csrf")) {
		h.renderError(writer, http.StatusForbidden)
		return
	}
	var err error
	if reset {
		err = h.listPreferences.Reset(request.Context(), principal, workspaceID, listprefs.ListMembers)
	} else {
		defaults := map[string]bool{}
		for _, column := range memberListColumns() {
			defaults[column.Key] = column.DefaultVisible
		}
		visible, parseErr := strconv.ParseBool(listVisibleParam(request))
		if parseErr != nil {
			h.renderError(writer, http.StatusBadRequest)
			return
		}
		err = h.listPreferences.Set(request.Context(), principal, workspaceID, listprefs.ListMembers, request.PathValue("column"), visible, defaults)
	}
	if err != nil {
		switch {
		case errors.Is(err, listprefs.ErrInvalidInput):
			h.renderError(writer, http.StatusBadRequest)
		case errors.Is(err, identity.ErrForbidden):
			h.renderError(writer, http.StatusForbidden)
		default:
			h.renderError(writer, http.StatusInternalServerError)
		}
		return
	}
	h.afterListChange(writer, request, "/workspaces/"+workspaceID.String()+"/settings/members"+clientQuerySuffix(request.URL.Query()), h.rerenderMembers)
}
