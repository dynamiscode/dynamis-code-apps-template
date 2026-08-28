package identity

import (
	"encoding/json"
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
	ErrMFARequired          = errors.New("multi-factor authentication is required")
	ErrMFAUnavailable       = errors.New("multi-factor authentication is unavailable")
	ErrInvalidMFAChallenge  = errors.New("multi-factor challenge is invalid or expired")
	ErrInvalidMFACode       = errors.New("multi-factor code is invalid")
	ErrLastMFAFactor        = errors.New("the final authentication factor cannot be removed")
)

type AuthLevel uint8

const (
	AuthLevelPassword AuthLevel = 1
	AuthLevelMFA      AuthLevel = 2
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
	AuthLevel   AuthLevel
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
	AuthLevel      AuthLevel
	OIDCProviderID string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
}

type MFAConfig struct {
	Enabled          bool
	EncryptionKey    []byte
	RelyingPartyID   string
	Origins          []string
	DisplayName      string
	RequireForAdmins bool
}

type MFAStatus struct {
	Enabled        bool `json:"enabled"`
	TOTPEnabled    bool `json:"totpEnabled"`
	PasskeyCount   int  `json:"passkeyCount"`
	RecoveryRemain int  `json:"recoveryCodesRemaining"`
}

type MFALoginChallenge struct {
	Token          string          `json:"challenge"`
	UserID         string          `json:"-"`
	AuthMethod     string          `json:"-"`
	OIDCProviderID string          `json:"-"`
	Methods        []string        `json:"methods"`
	PasskeyJSON    json.RawMessage `json:"passkeyOptions,omitempty"`
	ExpiresAt      time.Time       `json:"expiresAt"`
}

type TOTPEnrollment struct {
	Challenge  string    `json:"challenge"`
	Secret     string    `json:"secret"`
	OTPAuthURL string    `json:"otpauthUrl"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type Passkey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
}

type PasskeyEnrollment struct {
	Challenge string          `json:"challenge"`
	Options   json.RawMessage `json:"options"`
	ExpiresAt time.Time       `json:"expiresAt"`
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
