package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
)

type memberResponse struct {
	UserID    string        `json:"userId"`
	Email     string        `json:"email"`
	Role      identity.Role `json:"role"`
	CreatedAt time.Time     `json:"createdAt"`
}

type invitationResponse struct {
	ID         string        `json:"id"`
	Workspace  string        `json:"workspaceId"`
	Email      string        `json:"email"`
	Role       identity.Role `json:"role"`
	CreatedAt  time.Time     `json:"createdAt"`
	ExpiresAt  time.Time     `json:"expiresAt"`
	AcceptedAt *time.Time    `json:"acceptedAt,omitempty"`
	ExpiredAt  *time.Time    `json:"expiredAt,omitempty"`
	RevokedAt  *time.Time    `json:"revokedAt,omitempty"`
}

type tokenResponse struct {
	ID          string                `json:"id"`
	UserID      string                `json:"userId"`
	WorkspaceID string                `json:"workspaceId"`
	Name        string                `json:"name"`
	Scopes      []identity.Permission `json:"scopes"`
	CreatedAt   time.Time             `json:"createdAt"`
	ExpiresAt   *time.Time            `json:"expiresAt,omitempty"`
	LastUsedAt  *time.Time            `json:"lastUsedAt,omitempty"`
	RevokedAt   *time.Time            `json:"revokedAt,omitempty"`
}

type sessionResponse struct {
	ID             string             `json:"id"`
	UserID         string             `json:"userId"`
	AuthMethod     string             `json:"authMethod"`
	AuthLevel      identity.AuthLevel `json:"authLevel"`
	OIDCProviderID string             `json:"oidcProviderId,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	ExpiresAt      time.Time          `json:"expiresAt"`
	RevokedAt      *time.Time         `json:"revokedAt,omitempty"`
}

type memberRoleRequest struct {
	Role identity.Role `json:"role"`
}

type ownershipRequest struct {
	UserID string `json:"userId"`
}

type createInvitationRequest struct {
	Email           string        `json:"email"`
	Role            identity.Role `json:"role"`
	LifetimeSeconds int64         `json:"lifetimeSeconds"`
}

type invitationLinkResponse struct {
	Invitation      invitationResponse `json:"invitation"`
	InvitationURL   string             `json:"invitationUrl"`
	EmailDelivered  bool               `json:"emailDelivered"`
	DeliveryWarning string             `json:"deliveryWarning,omitempty"`
}

type createTokenRequest struct {
	Name      string                `json:"name"`
	Scopes    []identity.Permission `json:"scopes"`
	ExpiresAt *time.Time            `json:"expiresAt"`
}

type tokenScopesRequest struct {
	Scopes []identity.Permission `json:"scopes"`
}

func (h *handler) workspaceBearer(
	writer http.ResponseWriter,
	request *http.Request,
	workspaceID string,
	permission identity.Permission,
) (identity.Principal, bool) {
	principal, ok := h.bearerPrincipal(writer, request, permission)
	if !ok {
		return identity.Principal{}, false
	}
	if principal.WorkspaceID != workspaceID {
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
		return identity.Principal{}, false
	}
	return principal, true
}

func (h *handler) listMembers(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.MembersRead)
	if !ok {
		return
	}
	members, err := h.identity.ListMembers(request.Context(), principal)
	if err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	result := make([]memberResponse, len(members))
	for index, member := range members {
		result[index] = memberResponse{UserID: member.UserID, Email: member.Email, Role: member.Role, CreatedAt: member.CreatedAt}
	}
	writeJSON(writer, http.StatusOK, map[string]any{"members": result})
}

func (h *handler) changeMemberRole(writer http.ResponseWriter, request *http.Request) {
	workspaceID, userID := request.PathValue("workspaceId"), request.PathValue("userId")
	if !validID(workspaceID) || !validID(userID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.MembersManage)
	if !ok {
		return
	}
	var input memberRoleRequest
	if err := decodeJSON(request, &input); err != nil ||
		(input.Role != identity.Admin && input.Role != identity.Member && input.Role != identity.Viewer) {
		h.badJSON(writer, request, errors.New("invalid member role"))
		return
	}
	if err := h.identity.ChangeMemberRole(request.Context(), principal, userID, input.Role, h.auditContext(request)); err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) removeMember(writer http.ResponseWriter, request *http.Request) {
	workspaceID, userID := request.PathValue("workspaceId"), request.PathValue("userId")
	if !validID(workspaceID) || !validID(userID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.MembersManage)
	if !ok {
		return
	}
	if err := h.identity.RemoveMember(request.Context(), principal, userID, h.auditContext(request)); err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) transferOwnership(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.OwnershipTransfer)
	if !ok {
		return
	}
	var input ownershipRequest
	if err := decodeJSON(request, &input); err != nil || !validID(input.UserID) {
		h.badJSON(writer, request, errors.New("invalid owner"))
		return
	}
	if err := h.identity.TransferOwnership(request.Context(), principal, input.UserID, h.auditContext(request)); err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) listInvitations(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.InvitationsManage)
	if !ok {
		return
	}
	invitations, err := h.identity.ListInvitations(request.Context(), principal)
	if err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	result := make([]invitationResponse, len(invitations))
	for index, invitation := range invitations {
		result[index] = invitationDTO(invitation)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"invitations": result})
}

func (h *handler) createInvitation(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.InvitationsManage)
	if !ok {
		return
	}
	var input createInvitationRequest
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	lifetime, ok := invitationLifetime(input.LifetimeSeconds)
	if !ok {
		h.invalidRequest(writer, request, "The invitation lifetime is invalid.")
		return
	}
	invit, err := h.identity.CreateInvitation(request.Context(), principal, input.Email, input.Role, lifetime, h.auditContext(request))
	if err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	h.writeInvitation(writer, request, http.StatusCreated, invit)
}

func (h *handler) resendInvitation(writer http.ResponseWriter, request *http.Request) {
	workspaceID, invitationID := request.PathValue("workspaceId"), request.PathValue("invitationId")
	if !validID(workspaceID) || !validID(invitationID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.InvitationsManage)
	if !ok {
		return
	}
	secret, err := h.identity.ResendInvitation(request.Context(), principal, invitationID, 0, h.auditContext(request))
	if err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	invitations, err := h.identity.ListInvitations(request.Context(), principal)
	if err != nil {
		h.internal(writer, request)
		return
	}
	for _, invitation := range invitations {
		if invitation.ID == invitationID {
			h.writeInvitationLink(writer, request, http.StatusOK, invitation, secret)
			return
		}
	}
	h.identityProblem(writer, request, identity.ErrInvalidInvitation)
}

func (h *handler) revokeInvitation(writer http.ResponseWriter, request *http.Request) {
	workspaceID, invitationID := request.PathValue("workspaceId"), request.PathValue("invitationId")
	if !validID(workspaceID) || !validID(invitationID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.InvitationsManage)
	if !ok {
		return
	}
	if err := h.identity.RevokeInvitation(request.Context(), principal, invitationID, h.auditContext(request)); err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) writeInvitation(writer http.ResponseWriter, request *http.Request, status int, invitation identity.NewInvitation) {
	h.writeInvitationLink(writer, request, status, invitation.Invitation, invitation.Secret)
}

func (h *handler) writeInvitationLink(writer http.ResponseWriter, request *http.Request, status int, invitation identity.Invitation, secret string) {
	writer.Header().Set("Cache-Control", "no-store")
	link := invitationURL(h.publicURL, secret)
	delivered, warning := h.deliverInvitation(request, invitation.Email, link)
	writeJSON(writer, status, invitationLinkResponse{
		Invitation: invitationDTO(invitation), InvitationURL: link,
		EmailDelivered: delivered, DeliveryWarning: warning,
	})
}

func (h *handler) deliverInvitation(request *http.Request, recipient, link string) (bool, string) {
	if h.mailer == nil {
		return false, ""
	}
	if err := h.mailer.Send(request.Context(), recipient, "You are invited to Dynamis Code", "Open this invitation link: "+link); err != nil {
		return false, "Invitation email could not be delivered. Share the invitation link manually."
	}
	return true, ""
}

func (h *handler) listTokens(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	tokens, err := h.identity.ListAPITokens(request.Context(), principal)
	if err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	result := make([]tokenResponse, len(tokens))
	for index, token := range tokens {
		result[index] = tokenDTO(token)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"tokens": result})
}

func (h *handler) createToken(writer http.ResponseWriter, request *http.Request) {
	workspaceID := request.PathValue("workspaceId")
	if !validID(workspaceID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	var input createTokenRequest
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	token, err := h.identity.CreateAPIToken(request.Context(), principal, input.Name, input.Scopes, input.ExpiresAt, h.auditContext(request))
	if err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeJSON(writer, http.StatusCreated, map[string]any{"token": tokenDTO(token.APIToken), "secret": token.Secret})
}

func (h *handler) updateToken(writer http.ResponseWriter, request *http.Request) {
	workspaceID, tokenID := request.PathValue("workspaceId"), request.PathValue("tokenId")
	if !validID(workspaceID) || !validID(tokenID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	var input tokenScopesRequest
	if err := decodeJSON(request, &input); err != nil {
		h.badJSON(writer, request, err)
		return
	}
	if err := h.identity.UpdateAPITokenScopes(request.Context(), principal, tokenID, input.Scopes, h.auditContext(request)); err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) revokeToken(writer http.ResponseWriter, request *http.Request) {
	workspaceID, tokenID := request.PathValue("workspaceId"), request.PathValue("tokenId")
	if !validID(workspaceID) || !validID(tokenID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.workspaceBearer(writer, request, workspaceID, identity.WorkspaceRead)
	if !ok {
		return
	}
	if err := h.identity.RevokeAPIToken(request.Context(), principal, tokenID, h.auditContext(request)); err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) listSessions(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.bearerPrincipal(writer, request, identity.WorkspaceRead)
	if !ok {
		return
	}
	sessions, err := h.identity.ListSessions(request.Context(), principal.UserID)
	if err != nil {
		h.internal(writer, request)
		return
	}
	result := make([]sessionResponse, len(sessions))
	for index, session := range sessions {
		result[index] = sessionDTO(session)
	}
	writeJSON(writer, http.StatusOK, map[string]any{"sessions": result})
}

func (h *handler) revokeSession(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("sessionId")
	if !validID(sessionID) || len(request.URL.Query()) != 0 {
		h.invalidRequest(writer, request, "The request parameters are invalid.")
		return
	}
	principal, ok := h.bearerPrincipal(writer, request, identity.WorkspaceRead)
	if !ok {
		return
	}
	if _, err := h.identity.RevokeSession(request.Context(), principal.UserID, sessionID, h.auditContext(request)); err != nil {
		h.identityProblem(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *handler) identityProblem(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, identity.ErrForbidden):
		writeProblem(writer, request, http.StatusForbidden, "forbidden", "Access is denied.")
	case errors.Is(err, identity.ErrActiveInvitation):
		writeProblem(writer, request, http.StatusConflict, "active-invitation", "An active invitation already exists.")
	case errors.Is(err, identity.ErrLastOwner):
		writeProblem(writer, request, http.StatusConflict, "last-owner", "The final owner cannot be changed.")
	case errors.Is(err, identity.ErrInvalidInvitation):
		writeProblem(writer, request, http.StatusConflict, "invalid-invitation", "The invitation is invalid or expired.")
	case errors.Is(err, identity.ErrInvalidToken):
		writeProblem(writer, request, http.StatusConflict, "invalid-token", "The token is invalid or expired.")
	case errors.Is(err, identity.ErrInvalidSession):
		writeProblem(writer, request, http.StatusConflict, "invalid-session", "The session is invalid or expired.")
	case errors.Is(err, identity.ErrMFARequired):
		writeProblem(writer, request, http.StatusUnauthorized, "mfa-required", "Multi-factor authentication is required.")
	default:
		writeProblem(writer, request, http.StatusBadRequest, "invalid-request", "The identity request is invalid.")
	}
}

func invitationDTO(value identity.Invitation) invitationResponse {
	return invitationResponse{
		ID: value.ID, Workspace: value.WorkspaceID, Email: value.Email, Role: value.Role,
		CreatedAt: value.CreatedAt, ExpiresAt: value.ExpiresAt,
		AcceptedAt: value.AcceptedAt, ExpiredAt: value.ExpiredAt, RevokedAt: value.RevokedAt,
	}
}

func tokenDTO(value identity.APIToken) tokenResponse {
	return tokenResponse{
		ID: value.ID, UserID: value.UserID, WorkspaceID: value.WorkspaceID,
		Name: value.Name, Scopes: value.Scopes, CreatedAt: value.CreatedAt,
		ExpiresAt: value.ExpiresAt, LastUsedAt: value.LastUsedAt, RevokedAt: value.RevokedAt,
	}
}

func sessionDTO(value identity.Session) sessionResponse {
	return sessionResponse{
		ID: value.ID, UserID: value.UserID, AuthMethod: value.AuthMethod,
		AuthLevel:      value.AuthLevel,
		OIDCProviderID: value.OIDCProviderID, CreatedAt: value.CreatedAt,
		ExpiresAt: value.ExpiresAt, RevokedAt: value.RevokedAt,
	}
}

func invitationLifetime(seconds int64) (time.Duration, bool) {
	if seconds == 0 {
		return 0, true
	}
	if seconds < 60 || seconds > int64((30*24*time.Hour)/time.Second) {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

func invitationURL(publicURL, secret string) string {
	path := "/invitations/" + url.PathEscape(secret)
	if strings.TrimSpace(publicURL) == "" {
		return path
	}
	return strings.TrimRight(publicURL, "/") + path
}
