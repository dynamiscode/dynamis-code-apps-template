package web

import (
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/web/viewmodel"
	"example.com/dynamis-code/apps-template/internal/workspaceemail"
	"github.com/a-h/templ"
	"net/http"
	"strconv"
)

func (h *Handler) renderSettingsPage(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	data viewmodel.PageData,
	component func(viewmodel.PageData) templ.Component,
) {
	data.FilesEnabled = h.filesEnabled
	data.ClientsEnabled = h.clientsEnabled
	data.SingleWorkspace = h.workspaceMode == config.WorkspaceModeSingle
	h.localizePage(writer, &data)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = component(data).Render(request.Context(), writer)
}

func emailPortValue(settings workspaceemail.Settings) string {
	if settings.Configured && settings.UsesSMTP() {
		return strconv.Itoa(settings.Port)
	}
	return "587"
}

func emailSMTPValue(settings workspaceemail.Settings, value string) string {
	if settings.UsesSMTP() {
		return value
	}
	return ""
}

func workspaceNotificationLabel(data viewmodel.PageData, notificationType string) string {
	switch notificationType {
	case "security":
		return data.T("workspace.notification_security")
	case "workspace":
		return data.T("workspace.notification_workspace")
	case "system":
		return data.T("workspace.notification_system")
	default:
		return notificationType
	}
}
