package web

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/listprefs"
)

type memberListQuery struct {
	Email  string
	Role   string
	Sort   string
	Limit  int
	Cursor string
}

func memberListColumns() []clientListColumn {
	return []clientListColumn{
		{Key: "id", LabelKey: "members.id", ValueType: "text", DefaultVisible: true},
		{Key: "email", LabelKey: "common.email", ValueType: "text", DefaultVisible: true},
		{Key: "role", LabelKey: "members.role", ValueType: "text", DefaultVisible: true},
		{Key: "created_at", LabelKey: "clients.created_at", ValueType: "timestamp"},
	}
}

func parseMemberListQuery(values url.Values, defaultLimit int) (memberListQuery, bool) {
	form := memberListQuery{Email: strings.TrimSpace(values.Get("email")), Role: values.Get("role"), Sort: values.Get("sort")}
	for key, entries := range values {
		if len(entries) != 1 {
			return form, false
		}
		switch key {
		case "email", "role", "sort":
		case "limit":
			if entries[0] == "" {
				continue
			}
			limit, err := strconv.Atoi(entries[0])
			if err != nil || limit < 1 || limit > 100 {
				return form, false
			}
			form.Limit = limit
		case "cursor":
			form.Cursor = entries[0]
			if form.Cursor == "" {
				return form, false
			}
		default:
			return form, false
		}
	}
	if form.Role != "" && form.Role != "owner" && form.Role != "admin" && form.Role != "member" && form.Role != "viewer" {
		return form, false
	}
	if len(form.Email) > 320 || len(form.Cursor) > 64 {
		return form, false
	}
	if form.Sort == "" {
		form.Sort = "email"
	}
	if form.Sort != "email" && form.Sort != "-email" && form.Sort != "role" && form.Sort != "-role" &&
		form.Sort != "created_at" && form.Sort != "-created_at" {
		return form, false
	}
	if form.Cursor != "" && form.Limit == 0 {
		form.Limit = defaultLimit
	}
	return form, true
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
	if query.Cursor != "" {
		index := slices.IndexFunc(members, func(member identity.MemberSummary) bool { return member.ID.String() == query.Cursor })
		if index < 0 {
			h.renderError(writer, http.StatusBadRequest)
			return
		}
		members = members[index+1:]
	}
	var nextCursor string
	if query.Limit > 0 && len(members) > query.Limit {
		members = members[:query.Limit]
		nextCursor = members[len(members)-1].ID.String()
	}
	workspaces, err := h.identity.ListWorkspaces(request.Context(), session.UserID)
	if err != nil {
		h.renderError(writer, http.StatusInternalServerError)
		return
	}
	columns := memberListColumns()
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
	filterChips := memberFilterChips(query, request.URL.Query(), request.URL.Path)
	nextPath := ""
	if nextCursor != "" {
		next := cloneClientQuery(request.URL.Query())
		next.Set("cursor", nextCursor)
		nextPath = request.URL.Path + "?" + next.Encode()
	}
	formValues := cloneClientQuery(request.URL.Query())
	formValues.Del("cursor")
	querySuffix := ""
	if encoded := formValues.Encode(); encoded != "" {
		querySuffix = "?" + encoded
	}
	clear := cloneClientQuery(request.URL.Query())
	clear.Del("email")
	clear.Del("role")
	clear.Del("cursor")
	clearHref := request.URL.Path
	if encoded := clear.Encode(); encoded != "" {
		clearHref += "?" + encoded
	}
	h.render(writer, http.StatusOK, "members.html", pageData{
		Title: "Members", NavPage: "members", NavSection: "settings", CSRF: csrf,
		Workspace: workspaceByID(workspaces, workspaceID), Workspaces: workspaces, Members: members,
		CanManage: principal.Permissions[identity.MembersManage], CanTransfer: principal.Permissions[identity.OwnershipTransfer],
		CurrentPath: request.URL.Path, MemberTotal: total, NextCursor: nextCursor, NextPath: nextPath,
		ClientListColumns: columns, ClientListRows: rows, ClientFilterChips: filterChips,
		HasVisibleClientColumns: hasVisibleClientColumns(visible), HasClientFilters: query.Email != "" || query.Role != "",
		ClearClientFiltersPath: clearHref, ClientQuerySuffix: querySuffix, IsMembersList: true,
		MemberListQuery: query,
	})
}

func filterAndSortMembers(members []identity.MemberSummary, query memberListQuery) []identity.MemberSummary {
	filtered := make([]identity.MemberSummary, 0, len(members))
	for _, member := range members {
		if query.Email != "" && !strings.Contains(strings.ToLower(member.Email), strings.ToLower(query.Email)) {
			continue
		}
		if query.Role != "" && string(member.Role) != query.Role {
			continue
		}
		filtered = append(filtered, member)
	}
	descending := strings.HasPrefix(query.Sort, "-")
	field := strings.TrimPrefix(query.Sort, "-")
	slices.SortFunc(filtered, func(a, b identity.MemberSummary) int {
		var order int
		switch field {
		case "role":
			order = strings.Compare(string(a.Role), string(b.Role))
		case "created_at":
			if a.CreatedAt.Before(b.CreatedAt) {
				order = -1
			} else if a.CreatedAt.After(b.CreatedAt) {
				order = 1
			}
		default:
			order = strings.Compare(strings.ToLower(a.Email), strings.ToLower(b.Email))
		}
		if order == 0 {
			order = strings.Compare(a.ID.String(), b.ID.String())
		}
		if descending {
			return -order
		}
		return order
	})
	return filtered
}

func memberListRows(members []identity.MemberSummary) []clientListRow {
	rows := make([]clientListRow, 0, len(members))
	for index, member := range members {
		rows = append(rows, clientListRow{ID: member.ID.String(), Member: member, ActionContext: member.Email, ActionPosition: index + 1,
			Values: map[string]string{"id": member.ID.String(), "email": member.Email, "role": string(member.Role), "created_at": member.CreatedAt.Format(time.RFC3339)}})
	}
	return rows
}

func memberSortHref(path string, query url.Values, key string) string {
	copy := cloneClientQuery(query)
	copy.Del("cursor")
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

func memberFilterChips(query memberListQuery, values url.Values, path string) []clientFilterChip {
	var chips []clientFilterChip
	for _, filter := range []struct{ key, value, label, operator string }{
		{"email", query.Email, "common.email", "clients.op_contains"},
		{"role", query.Role, "members.role", "clients.op_equals"},
	} {
		if filter.value == "" {
			continue
		}
		copy := cloneClientQuery(values)
		copy.Del(filter.key)
		copy.Del("cursor")
		href := path
		if encoded := copy.Encode(); encoded != "" {
			href += "?" + encoded
		}
		chips = append(chips, clientFilterChip{FieldLabelKey: filter.label, OperatorLabelKey: filter.operator, Value: filter.value, RemoveHref: href})
	}
	return chips
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
		visible, parseErr := strconv.ParseBool(request.FormValue("visible"))
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
	h.redirect(writer, request, "/workspaces/"+workspaceID.String()+"/settings/members"+clientQuerySuffix(request.URL.Query()))
}
