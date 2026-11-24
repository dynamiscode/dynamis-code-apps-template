package viewmodel

import (
	"encoding/json"
	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	appfiles "example.com/dynamis-code/apps-template/internal/files"
	"example.com/dynamis-code/apps-template/internal/i18n"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/webhooks"
	"example.com/dynamis-code/apps-template/internal/workspaceemail"
)

type TimelineDraft struct {
	Type, Channel, Text, Corrects string
}

type PageData struct {
	EmailSettings              workspaceemail.Settings
	EmailSaved                 bool
	EmailLoginReason           string
	MCPConsent                 *McpConsentView
	Locale                     string
	Theme                      string
	Catalog                    *i18n.Catalog
	Title                      string
	NavPage                    string
	NavSection                 string
	Error                      string
	CSRF                       string
	Workspace                  identity.WorkspaceSummary
	Workspaces                 []identity.WorkspaceSummary
	SingleWorkspace            bool
	DefaultWorkspaceHref       string
	Clients                    []clients.Client
	Client                     clients.Client
	Engagements                []engagement.Engagement
	EngagementStages           []engagement.Stage
	OpenEngagementStages       []engagement.Stage
	EngagementName             string
	EngagementStage            string
	EngagementAssignee         string
	EngagementOwner            string
	EngagementOwnerError       bool
	EngagementQuoteStatusError bool
	EngagementQuoteAmountError bool
	EngagementQuoteDateError   bool
	EngagementActionTextError  bool
	EngagementActionDateError  bool
	EngagementOwnerUnavailable bool
	EngagementMembers          []identity.MemberSummary
	AgreementFiles             []appfiles.File
	AgreementFilesTotal        int
	AgreementFilesAvailable    bool
	EngagementEnabled          bool
	EngagementLabel            string
	EngagementNotes            string
	ClientValues               []ClientValueRow
	ClientTimeline             []clients.TimelineEntry
	TimelinePage               int
	TimelineDraft              TimelineDraft
	ClientDraft                clients.Input
	ClientFields               []ClientFieldRow
	FieldDraft                 clients.FieldInput
	CanManageFields            bool
	FieldsPath                 string
	CanEditClients             bool
	CanDeleteClients           bool
	ClientsEnabled             bool
	CatalogEnabled             bool
	ErrorKey                   string
	Files                      []appfiles.File
	FilesEnabled               bool
	ClientQuery                ClientQueryForm
	ClientQueryFields          []clients.Field
	ListColumns                []ListColumn
	ListRows                   []ListRow
	ListTotal                  int
	// ListStart is the 1-based position of the page's first row; zero when the list does not track it.
	ListStart int
	// ListPager is the list's page navigation.
	ListPager ListPager
	// TopbarTitleKey names the page title shown in the top bar; empty shows none.
	TopbarTitleKey       string
	IsMembersList        bool
	IsFilesList          bool
	MemberListQuery      MemberListQuery
	MemberTotal          int
	FileListQuery        FileListQuery
	FileMinSize          string
	FileMaxSize          string
	Filter               ListFilterView
	ClientPreservedQuery []ClientHiddenField
	ListPreservedQuery   []ClientHiddenField
	// ListLive makes the list's search update the results region in place.
	ListLive bool
	// ListSelectable adds a selection column and the bulk bar to a list whose rows can be acted on together.
	ListSelectable bool
	// ListFragment is set when the page is the results fragment of a live list, which also refreshes the
	// controls whose links depend on the search.
	ListFragment bool
	// ListSearch is the search text of the list on the page, highlighted in its cells.
	ListSearch string
	// Saved views: the list's views, the query a new view would save, where the view routes live, and whether this
	// list can save (and why not, when archived clients are showing).
	ListViews     []ListViewOption
	ListViewQuery string
	ListViewBase  string
	ListViewRoute string
	// ArchivedClientCount is shown on the footer link to archived clients.
	ArchivedClientCount              int
	ListViewCanShare                 bool
	ListViewSaveReason               string
	HasVisibleClientColumns          bool
	HasClientFilters                 bool
	ClearClientFiltersPath           string
	ClientQuerySuffix                string
	HiddenClientSort                 string
	HiddenClientSortLabelKey         string
	CreateKey                        string
	NewTitle                         string
	ToastKey                         string
	CurrentPath                      string
	CurrentSessionID                 string
	Email                            string
	UserLocale                       string
	Saved                            bool
	WorkspaceName                    string
	WorkspaceLocale                  string
	SetupTokenRequired               bool
	Members                          []identity.MemberSummary
	Invitations                      []identity.Invitation
	Tokens                           []identity.APIToken
	Sessions                         []identity.Session
	AuditHistory                     []identity.AuditHistoryEntry
	AuditActorEmails                 map[identity.UserID]string
	AuditLazy                        bool
	Profile                          identity.UserProfile
	NotificationPreferences          []identity.NotificationPreference
	WorkspaceNotificationPreferences []identity.NotificationPreference
	Notifications                    []identity.Notification
	NotificationCursor               string
	UnreadNotifications              int
	PasswordResetSecret              string
	PasswordResetCompleted           bool
	CurrentAuthMethod                string
	OIDCProviders                    []identity.OIDCProviderInfo
	Invitation                       identity.Invitation
	InvitationSecret                 string
	InvitationAuthenticated          bool
	InvitationURL                    string
	TokenSecret                      string
	SCIMEndpoint                     string
	SCIMTokenSecret                  string
	DeliveryWarning                  string
	CanManage                        bool
	CanCreateWorkspace               bool
	CanTransfer                      bool
	CanShare                         bool
	PublicShareURL                   string
	ReturnTo                         string
	MFAChallenge                     string
	MFAOptions                       json.RawMessage
	MFAError                         string
	TOTPEnrollment                   *identity.TOTPEnrollment
	MFARecoveryCodes                 []string
	Passkeys                         []identity.Passkey
	MFAStatus                        identity.MFAStatus
	Webhooks                         []webhooks.Webhook
	SelectedWebhook                  webhooks.Webhook
	WebhookDeliveries                []webhooks.Delivery
	WebhookSecret                    string
	WebhookSecretKeyConfigured       bool
	WebhookReadbackFailed            bool
}

type FileListQuery struct {
	Search, MIME, Sort     string
	MinSize, MaxSize       int64
	HasMinSize, HasMaxSize bool
	Match                  string
	Filter                 ListFilterInput
	Limit                  int
	Page                   int
}
