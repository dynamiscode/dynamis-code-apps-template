package viewmodel

// mcpConsentView is the data for the MCP consent page. Every value here is
// re-validated on submission; hidden fields are never trusted on their own.
type McpConsentView struct {
	ClientName string
	Scopes     []string
	Workspaces []McpWorkspaceChoice
	Params     McpAuthorizeParams
}

type McpWorkspaceChoice struct {
	ID   string
	Name string
}

type McpAuthorizeParams struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	Scope         string
	State         string
}
