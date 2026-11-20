package viewmodel

import (
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
)

// AuditActor is the actor's email when the workspace member list knows the user, or empty.
func (p PageData) AuditActor(id identity.UserID) string {
	return p.AuditActorEmails[id]
}

// AuditOutcome localizes a known audit outcome and passes any other value through unchanged.
func (p PageData) AuditOutcome(value string) string {
	switch value {
	case "success", "failure":
		return p.T("audit.outcome_" + value)
	}
	return value
}

func (p PageData) T(key string, values ...any) string {
	args := map[string]any{}
	if len(values) == 1 {
		if supplied, ok := values[0].(map[string]any); ok {
			args = supplied
		}
	}
	return p.Catalog.Translate(i18n.Locale(p.Locale), key, args)
}

func (p PageData) Date(value time.Time) string {
	return i18n.FormatDate(i18n.Locale(p.Locale), value)
}

func (p PageData) DateTime(value time.Time) string {
	return i18n.FormatDateTime(i18n.Locale(p.Locale), value)
}

// DefaultTokenExpiry is the smart default for a new API token: 90 days from
// today. The user can change it or clear it for a token without expiry.
func (p PageData) DefaultTokenExpiry() string {
	return time.Now().UTC().AddDate(0, 0, 90).Format("2006-01-02")
}

func (p PageData) Role(value identity.Role) string {
	keys := map[identity.Role]string{
		identity.Owner: "members.owner", identity.Admin: "members.admin",
		identity.Member: "members.member", identity.Viewer: "members.viewer",
	}
	return p.T(keys[value])
}

func (p PageData) Scope(value identity.Permission) string {
	keys := map[identity.Permission]string{
		identity.WorkspaceRead: "tokens.workspace_read", identity.ResourcesRead: "tokens.resources_read",
		identity.WorkspaceUpdate: "tokens.workspace_update",
		identity.ResourcesWrite:  "tokens.resources_write", identity.WorkspaceExport: "tokens.workspace_export",
	}
	return p.T(keys[value])
}

func (p PageData) ScopeList(values []identity.Permission) string {
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, p.Scope(value))
	}
	return strings.Join(labels, ", ")
}

func (p PageData) AuthMethod(value string) string {
	if value == "oidc" {
		return p.T("security.oidc")
	}
	return p.T("security.local")
}
