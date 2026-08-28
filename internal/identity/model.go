package identity

import (
	"errors"
	"net/http"
	"time"
)

var (
	ErrAlreadyBootstrapped = errors.New("instance is already bootstrapped")
	ErrActiveInvitation    = errors.New("an active invitation already exists")
	ErrForbidden           = errors.New("forbidden")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrInvalidBootstrap    = errors.New("bootstrap input is invalid")
	ErrInvalidInvitation   = errors.New("invitation is invalid or expired")
	ErrInvalidSession      = errors.New("session is invalid or expired")
	ErrInvalidToken        = errors.New("token is invalid or expired")
	ErrInvalidLocale       = errors.New("locale is invalid")
	ErrLastOwner           = errors.New("the final owner cannot be changed")
	ErrOIDCTransaction     = errors.New("OIDC transaction is invalid or expired")
	ErrUnknownOIDCProvider = errors.New("unknown OIDC provider")
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
