package httpapi

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"example.com/dynamis-code/apps-template/internal/identity"
)

const (
	scimUserSchema  = "urn:ietf:params:scim:schemas:core:2.0:User"
	scimGroupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"
	scimListSchema  = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	scimErrorSchema = "urn:ietf:params:scim:api:messages:2.0:Error"
)

type scimUserResponse struct {
	Schemas     []string    `json:"schemas"`
	ID          string      `json:"id"`
	ExternalID  string      `json:"externalId,omitempty"`
	UserName    string      `json:"userName"`
	DisplayName string      `json:"displayName,omitempty"`
	Active      bool        `json:"active"`
	Emails      []scimEmail `json:"emails"`
	Meta        scimMeta    `json:"meta"`
}

type scimEmail struct {
	Value   string `json:"value"`
	Primary bool   `json:"primary,omitempty"`
	Type    string `json:"type,omitempty"`
}

type scimMeta struct {
	ResourceType string `json:"resourceType"`
	Created      string `json:"created"`
	LastModified string `json:"lastModified"`
	Version      string `json:"version"`
}

type scimGroupResponse struct {
	Schemas     []string          `json:"schemas"`
	ID          string            `json:"id"`
	DisplayName string            `json:"displayName"`
	Members     []scimGroupMember `json:"members"`
	Meta        scimMeta          `json:"meta"`
}

type scimGroupMember struct {
	Value string `json:"value"`
}

type scimListResponse struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
	Resources    any      `json:"Resources"`
}

type scimErrorResponse struct {
	Schemas  []string `json:"schemas"`
	Status   string   `json:"status"`
	ScimType string   `json:"scimType,omitempty"`
	Detail   string   `json:"detail"`
}

type scimUserRequest struct {
	ExternalID  string      `json:"externalId"`
	UserName    string      `json:"userName"`
	DisplayName string      `json:"displayName"`
	Emails      []scimEmail `json:"emails"`
}

type scimPatchRequest struct {
	Operations []scimPatchOperation `json:"Operations"`
}

type scimPatchOperation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

type scimTokenResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	Secret      string `json:"secret"`
}

func (h *handler) createSCIMToken(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	token, err := h.identity.CreateSCIMToken(request.Context(), principal, h.auditContext(request))
	if err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusCreated, scimTokenResponse{ID: token.ID, WorkspaceID: token.WorkspaceID, Secret: token.Secret})
}

func (h *handler) revokeSCIMToken(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceUpdate)
	if !ok {
		return
	}
	if err := h.identity.RevokeSCIMToken(request.Context(), principal, h.auditContext(request)); err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) scimPrincipal(writer http.ResponseWriter, request *http.Request) (identity.Principal, bool) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) {
		h.scimProblem(writer, request, identity.ErrSCIMInvalid)
		return identity.Principal{}, false
	}
	parts := strings.Fields(request.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		h.scimUnauthorized(writer, request)
		return identity.Principal{}, false
	}
	principal, err := h.identity.AuthenticateSCIMToken(request.Context(), parts[1], workspaceID)
	if err != nil {
		h.scimUnauthorized(writer, request)
		return identity.Principal{}, false
	}
	return principal, true
}

func (h *handler) listSCIMUsers(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.scimPrincipal(writer, request)
	if !ok {
		return
	}
	field, value, ok := scimFilter(request)
	if !ok {
		h.scimProblem(writer, request, identity.ErrSCIMInvalid)
		return
	}
	start, count, ok := scimPage(request)
	if !ok {
		h.scimProblem(writer, request, identity.ErrSCIMInvalid)
		return
	}
	users, total, err := h.identity.ListSCIMUsers(request.Context(), principal, field, value, start, count)
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	resources := make([]scimUserResponse, len(users))
	for i, user := range users {
		resources[i] = scimUserDTO(user, request)
	}
	writeSCIMJSON(writer, http.StatusOK, scimListResponse{Schemas: []string{scimListSchema}, TotalResults: total, StartIndex: start, ItemsPerPage: len(resources), Resources: resources})
}

func (h *handler) createSCIMUser(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.scimPrincipal(writer, request)
	if !ok {
		return
	}
	var input scimUserRequest
	if err := decodeSCIMJSON(request, &input); err != nil || strings.TrimSpace(input.UserName) == "" {
		h.scimProblem(writer, request, identity.ErrSCIMInvalid)
		return
	}
	email := input.UserName
	for _, candidate := range input.Emails {
		if candidate.Primary || email == input.UserName {
			email = candidate.Value
			if candidate.Primary {
				break
			}
		}
	}
	user, err := h.identity.CreateSCIMUser(request.Context(), principal, identity.SCIMUserInput{ExternalID: input.ExternalID, UserName: input.UserName, Email: email, DisplayName: input.DisplayName}, h.auditContext(request))
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(user.Version))
	writer.Header().Set("Location", "/scim/v2/"+principal.WorkspaceID+"/Users/"+user.ID)
	writeSCIMJSON(writer, http.StatusCreated, scimUserDTO(user, request))
}

func (h *handler) getSCIMUser(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.scimPrincipal(writer, request)
	if !ok {
		return
	}
	user, err := h.identity.GetSCIMUser(request.Context(), principal, request.PathValue("userId"))
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(user.Version))
	if request.Header.Get("If-None-Match") == etag(user.Version) {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writeSCIMJSON(writer, http.StatusOK, scimUserDTO(user, request))
}

func (h *handler) patchSCIMUser(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.scimPrincipal(writer, request)
	if !ok {
		return
	}
	version, ok := scimIfMatch(request)
	if !ok {
		h.scimProblem(writer, request, identity.ErrSCIMPrecondition)
		return
	}
	var input scimPatchRequest
	if err := decodeSCIMJSON(request, &input); err != nil {
		h.scimProblem(writer, request, identity.ErrSCIMInvalid)
		return
	}
	patch, err := scimUserPatch(input.Operations)
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	user, err := h.identity.PatchSCIMUser(request.Context(), principal, request.PathValue("userId"), patch, version, h.auditContext(request))
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(user.Version))
	writeSCIMJSON(writer, http.StatusOK, scimUserDTO(user, request))
}

func (h *handler) deleteSCIMUser(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.scimPrincipal(writer, request)
	if !ok {
		return
	}
	version, ok := scimIfMatch(request)
	if !ok {
		h.scimProblem(writer, request, identity.ErrSCIMPrecondition)
		return
	}
	if err := h.identity.DeleteSCIMUser(request.Context(), principal, request.PathValue("userId"), version, h.auditContext(request)); err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) listSCIMGroups(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.scimPrincipal(writer, request)
	if !ok {
		return
	}
	if len(request.URL.Query()) != 0 {
		h.scimProblem(writer, request, identity.ErrSCIMInvalid)
		return
	}
	groups, err := h.identity.ListSCIMGroups(request.Context(), principal)
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	resources := make([]scimGroupResponse, len(groups))
	for i, group := range groups {
		resources[i] = scimGroupDTO(group, request)
	}
	writeSCIMJSON(writer, http.StatusOK, scimListResponse{Schemas: []string{scimListSchema}, TotalResults: len(resources), StartIndex: 1, ItemsPerPage: len(resources), Resources: resources})
}

func (h *handler) getSCIMGroup(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.scimPrincipal(writer, request)
	if !ok {
		return
	}
	group, err := h.identity.GetSCIMGroup(request.Context(), principal, request.PathValue("groupId"))
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(group.Version))
	writeSCIMJSON(writer, http.StatusOK, scimGroupDTO(group, request))
}

func (h *handler) patchSCIMGroup(writer http.ResponseWriter, request *http.Request) {
	principal, ok := h.scimPrincipal(writer, request)
	if !ok {
		return
	}
	version, ok := scimIfMatch(request)
	if !ok {
		h.scimProblem(writer, request, identity.ErrSCIMPrecondition)
		return
	}
	var input scimPatchRequest
	if err := decodeSCIMJSON(request, &input); err != nil {
		h.scimProblem(writer, request, identity.ErrSCIMInvalid)
		return
	}
	operations, err := scimGroupOperations(input.Operations)
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	group, err := h.identity.PatchSCIMGroup(request.Context(), principal, request.PathValue("groupId"), operations, version, h.auditContext(request))
	if err != nil {
		h.scimProblem(writer, request, err)
		return
	}
	writer.Header().Set("ETag", etag(group.Version))
	writeSCIMJSON(writer, http.StatusOK, scimGroupDTO(group, request))
}

func decodeSCIMJSON(request *http.Request, target any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil {
		return errors.New("unsupported SCIM content type")
	}
	if mediaType != "application/scim+json" && mediaType != "application/json" {
		return errors.New("unsupported SCIM content type")
	}
	decoder := json.NewDecoder(request.Body)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func scimFilter(request *http.Request) (string, string, bool) {
	values, ok := request.URL.Query()["filter"]
	if !ok {
		return "", "", true
	}
	if len(values) != 1 || len(values[0]) > 256 {
		return "", "", false
	}
	parts := strings.Fields(values[0])
	if len(parts) != 3 || !strings.EqualFold(parts[1], "eq") || len(parts[2]) < 2 || parts[2][0] != '"' || parts[2][len(parts[2])-1] != '"' {
		return "", "", false
	}
	field := parts[0]
	if !strings.EqualFold(field, "userName") && !strings.EqualFold(field, "externalId") {
		return "", "", false
	}
	return map[string]string{"username": "userName", "externalid": "externalId"}[strings.ToLower(field)], parts[2][1 : len(parts[2])-1], true
}

func scimPage(request *http.Request) (int, int, bool) {
	query := request.URL.Query()
	for key, values := range query {
		if key != "filter" && key != "startIndex" && key != "count" || len(values) != 1 {
			return 0, 0, false
		}
		if (key == "startIndex" || key == "count") && values[0] == "" {
			return 0, 0, false
		}
	}
	start, count := 1, identity.SCIMPageSize(0)
	var err error
	if value := query.Get("startIndex"); value != "" {
		start, err = strconv.Atoi(value)
		if err != nil || start < 1 {
			return 0, 0, false
		}
	}
	if value := query.Get("count"); value != "" {
		count, err = strconv.Atoi(value)
		if err != nil || count < 1 || count > 100 {
			return 0, 0, false
		}
	}
	return start, count, true
}

func scimIfMatch(request *http.Request) (int64, bool) {
	value := request.Header.Get("If-Match")
	if value == "" {
		return 0, false
	}
	return parseETag(value)
}

func scimUserPatch(operations []scimPatchOperation) (identity.SCIMUserPatch, error) {
	var patch identity.SCIMUserPatch
	if len(operations) == 0 || len(operations) > 10 {
		return patch, identity.ErrSCIMInvalid
	}
	for _, operation := range operations {
		path := strings.ToLower(strings.TrimSpace(operation.Path))
		switch path {
		case "active":
			var value bool
			if json.Unmarshal(operation.Value, &value) != nil {
				return patch, identity.ErrSCIMInvalid
			}
			patch.Active = &value
		case "username":
			var value string
			if json.Unmarshal(operation.Value, &value) != nil || strings.TrimSpace(value) == "" {
				return patch, identity.ErrSCIMInvalid
			}
			patch.UserName = &value
		case "displayname":
			var value string
			if json.Unmarshal(operation.Value, &value) != nil {
				return patch, identity.ErrSCIMInvalid
			}
			patch.DisplayName = &value
		case "emails":
			var emails []scimEmail
			if json.Unmarshal(operation.Value, &emails) != nil || len(emails) == 0 {
				return patch, identity.ErrSCIMInvalid
			}
			patch.Email = &emails[0].Value
		default:
			if strings.HasPrefix(path, "emails[") && strings.HasSuffix(path, "].value") {
				var value string
				if json.Unmarshal(operation.Value, &value) != nil || strings.TrimSpace(value) == "" {
					return patch, identity.ErrSCIMInvalid
				}
				patch.Email = &value
				break
			}
			return patch, identity.ErrSCIMInvalid
		}
		if operation.Op == "" || !strings.EqualFold(operation.Op, "replace") {
			return patch, identity.ErrSCIMInvalid
		}
	}
	return patch, nil
}

func scimGroupOperations(operations []scimPatchOperation) ([]identity.SCIMGroupOperation, error) {
	if len(operations) == 0 || len(operations) > 10 {
		return nil, identity.ErrSCIMInvalid
	}
	result := make([]identity.SCIMGroupOperation, len(operations))
	for i, operation := range operations {
		if !strings.EqualFold(operation.Path, "members") || (strings.ToLower(operation.Op) != "add" && strings.ToLower(operation.Op) != "remove") {
			return nil, identity.ErrSCIMInvalid
		}
		var members []scimGroupMember
		if err := json.Unmarshal(operation.Value, &members); err != nil {
			return nil, identity.ErrSCIMInvalid
		}
		result[i].Operation = strings.ToLower(operation.Op)
		for _, member := range members {
			if strings.TrimSpace(member.Value) == "" || len(member.Value) > 256 {
				return nil, identity.ErrSCIMInvalid
			}
			result[i].Members = append(result[i].Members, member.Value)
		}
		if len(result[i].Members) == 0 {
			return nil, identity.ErrSCIMInvalid
		}
	}
	return result, nil
}

func scimUserDTO(user identity.SCIMUser, request *http.Request) scimUserResponse {
	return scimUserResponse{Schemas: []string{scimUserSchema}, ID: user.ID, ExternalID: user.ExternalID, UserName: user.UserName, DisplayName: user.DisplayName, Active: user.Active, Emails: []scimEmail{{Value: user.Email, Primary: true, Type: "work"}}, Meta: scimMeta{ResourceType: "User", Created: user.CreatedAt.Format("2006-01-02T15:04:05.000000000Z"), LastModified: user.UpdatedAt.Format("2006-01-02T15:04:05.000000000Z"), Version: etag(user.Version)}}
}

func scimGroupDTO(group identity.SCIMGroup, request *http.Request) scimGroupResponse {
	members := make([]scimGroupMember, len(group.Members))
	for i, value := range group.Members {
		members[i] = scimGroupMember{Value: value}
	}
	return scimGroupResponse{Schemas: []string{scimGroupSchema}, ID: group.ID, DisplayName: group.DisplayName, Members: members, Meta: scimMeta{ResourceType: "Group", Version: etag(group.Version)}}
}

func writeSCIMJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/scim+json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func (h *handler) scimUnauthorized(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("WWW-Authenticate", `Bearer realm="scim"`)
	h.scimError(writer, request, http.StatusUnauthorized, "invalidToken", "Authentication is required or invalid.")
}

func (h *handler) scimProblem(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		h.scimError(writer, request, http.StatusForbidden, "", "Access is denied.")
	case errors.Is(err, identity.ErrSCIMNotFound):
		h.scimError(writer, request, http.StatusNotFound, "", "The SCIM resource was not found.")
	case errors.Is(err, identity.ErrSCIMConflict):
		h.scimError(writer, request, http.StatusConflict, "uniqueness", "The SCIM resource conflicts with an existing resource.")
	case errors.Is(err, identity.ErrSCIMPrecondition):
		status := http.StatusPreconditionFailed
		if request.Header.Get("If-Match") == "" {
			status = http.StatusPreconditionRequired
		}
		h.scimError(writer, request, status, "versionMismatch", "The entity tag is missing or stale.")
	case errors.Is(err, identity.ErrLastOwner):
		h.scimError(writer, request, http.StatusConflict, "mutability", "The final owner cannot be deactivated.")
	default:
		h.scimError(writer, request, http.StatusBadRequest, "invalidValue", "The SCIM request is invalid.")
	}
}

func (h *handler) scimError(writer http.ResponseWriter, _ *http.Request, status int, scimType, detail string) {
	writer.Header().Set("Content-Type", "application/scim+json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(scimErrorResponse{Schemas: []string{scimErrorSchema}, Status: strconv.Itoa(status), ScimType: scimType, Detail: detail})
}
