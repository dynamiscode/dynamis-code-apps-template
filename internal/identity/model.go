package identity

import (
	"errors"
	"net/http"
	"time"
)

var (
	ErrAlreadyBootstrapped  = errors.New("instance is already bootstrapped")
	ErrActiveInvitation     = errors.New("an active invitation already exists")
	ErrForbidden            = errors.New("forbidden")
	ErrInvalidCredentials   = errors.New("invalid email or password")
	ErrInvalidBootstrap     = errors.New("bootstrap input is invalid")
	ErrInvalidInvitation    = errors.New("invitation is invalid or expired")
	ErrInvalidSession       = errors.New("session is invalid or expired")
	ErrInvalidToken         = errors.New("token is invalid or expired")
	ErrInvalidLocale        = errors.New("locale is invalid")
	ErrInvalidPassword      = errors.New("password is invalid")
	ErrInvalidProfile       = errors.New("profile is invalid")
	ErrInvalidNotification  = errors.New("notification cursor is invalid")
	ErrInvalidReset         = errors.New("password reset is invalid or expired")
	ErrInvalidVerification  = errors.New("email verification is invalid or expired")
	ErrEmailAlreadyVerified = errors.New("email is already verified")
	ErrOwnedWorkspace       = errors.New("account owns a workspace")
	ErrLastOwner            = errors.New("the final owner cannot be changed")
	ErrOIDCTransaction      = errors.New("OIDC transaction is invalid or expired")
	ErrUnknownOIDCProvider  = errors.New("unknown OIDC provider")
	ErrSCIMNotFound         = errors.New("SCIM resource not found")
	ErrSCIMConflict         = errors.New("SCIM resource conflict")
	ErrSCIMPrecondition     = errors.New("SCIM resource precondition failed")
	ErrSCIMInvalid          = errors.New("SCIM request is invalid")
)

type Role string

const (
	Owner  Role = "owner"
	Admin  Role = "admin"
	Member Role = "member"
	Viewer Role = "viewer"
)

type Permission string

const (
	WorkspaceRead     Permission = "workspace:read"
	WorkspaceUpdate   Permission = "workspace:update"
	WorkspaceDelete   Permission = "workspace:delete"
	WorkspaceExport   Permission = "workspace:export"
	OwnershipTransfer Permission = "ownership:transfer"
	MembersRead       Permission = "members:read"
	MembersManage     Permission = "members:manage"
	InvitationsManage Permission = "invitations:manage"
	WebhooksRead      Permission = "webhooks:read"
	WebhooksManage    Permission = "webhooks:manage"
	ResourcesRead     Permission = "resources:read"
	ResourcesWrite    Permission = "resources:write"
	SCIMManage        Permission = "scim:manage"
)

type Principal struct {
	UserID      string
	WorkspaceID string
	Role        Role
	Permissions map[Permission]bool
	AuthMethod  string
	TokenID     string
}

type BootstrapInput struct {
	Email           string
	Password        string
	WorkspaceName   string
	WorkspaceLocale string
}

type BootstrapResult struct {
	UserID      string
	WorkspaceID string
}

type WorkspaceSummary struct {
	ID     string
	Name   string
	Role   Role
	Locale string
}

type WorkspaceCreateInput struct {
	Name   string
	Locale string
}

type MemberSummary struct {
	UserID    string
	Email     string
	Role      Role
	CreatedAt time.Time
}

type UserProfile struct {
	ID              string
	Email           string
	DisplayName     string
	Locale          string
	Timezone        string
	Theme           string
	EmailVerifiedAt *time.Time
}

type ProfileUpdateInput struct {
	DisplayName string
	Locale      string
	Timezone    string
	Theme       string
}

type NewEmailVerification struct {
	Email     string
	Secret    string
	ExpiresAt time.Time
}

type NewPasswordReset struct {
	Email     string
	Secret    string
	ExpiresAt time.Time
}

type Notification struct {
	ID               string
	UserID           string
	WorkspaceID      string
	NotificationType string
	Title            string
	Body             string
	CreatedAt        time.Time
	ReadAt           *time.Time
}

type NotificationInput struct {
	RecipientUserID  string
	WorkspaceID      string
	NotificationType string
	Title            string
	Body             string
}

type NotificationPreference struct {
	Scope            string
	NotificationType string
	Enabled          bool
}
type Session struct {
	ID             string
	UserID         string
	AuthMethod     string
	OIDCProviderID string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
}

type NewSession struct {
	Session
	Secret     string
	CSRFSecret string
}

type Invitation struct {
	ID              string
	WorkspaceID     string
	WorkspaceLocale string
	Email           string
	Role            Role
	CreatedAt       time.Time
	ExpiresAt       time.Time
	AcceptedAt      *time.Time
	ExpiredAt       *time.Time
	RevokedAt       *time.Time
}

type NewInvitation struct {
	Invitation
	Secret string
}

type APIToken struct {
	ID          string
	UserID      string
	WorkspaceID string
	Name        string
	Scopes      []Permission
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	LastUsedAt  *time.Time
	RevokedAt   *time.Time
}

type NewAPIToken struct {
	APIToken
	Secret string
}

type NewSCIMToken struct {
	ID          string
	WorkspaceID string
	CreatedAt   time.Time
	Secret      string
}

type SCIMUser struct {
	ID          string
	UserID      string
	ExternalID  string
	UserName    string
	Email       string
	DisplayName string
	Active      bool
	Role        Role
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type SCIMUserInput struct {
	ExternalID  string
	UserName    string
	Email       string
	DisplayName string
}

type SCIMUserPatch struct {
	UserName    *string
	Email       *string
	DisplayName *string
	Active      *bool
}

type SCIMGroup struct {
	ID          string
	DisplayName string
	Role        Role
	Version     int64
	Members     []string
}

type SCIMGroupOperation struct {
	Operation string
	Members   []string
}

type OIDCTransaction struct {
	ProviderID     string
	BrowserSession string
	Purpose        string
	UserID         string
	State          string
	PKCEVerifier   string
	Nonce          string
	RedirectURI    string
	ExpiresAt      time.Time
}

type AuditContext struct {
	RequestID     string
	SourceAddress string
}

type AuditEvent struct {
	ID            string
	EventType     string
	ActorUserID   string
	AuthMethod    string
	WorkspaceID   string
	TargetType    string
	TargetID      string
	Action        string
	Outcome       string
	RequestID     string
	SourceAddress string
	Metadata      string
	CreatedAt     time.Time
}

type CookiePolicy struct {
	HTTPOnly bool
	SameSite http.SameSite
	Secure   bool
}

func BrowserCookiePolicy(https bool) CookiePolicy {
	return CookiePolicy{
		HTTPOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   https,
	}
}
