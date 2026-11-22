package molecules

import atoms "example.com/dynamis-code/apps-template/internal/web/components/atoms"
import "example.com/dynamis-code/apps-template/internal/web/viewmodel"

// HomeCrumbItems returns the leading workspace crumb, or nil when a single
// workspace with no href renders no leading crumb at all.
func HomeCrumbItems(data viewmodel.PageData) []atoms.BreadcrumbsProp {
	if data.SingleWorkspace && data.DefaultWorkspaceHref == "" {
		return nil
	}
	href := "/"
	if data.DefaultWorkspaceHref != "" {
		href = data.DefaultWorkspaceHref
	}
	return []atoms.BreadcrumbsProp{{Label: data.T("workspace.workspaces"), Href: href}}
}
