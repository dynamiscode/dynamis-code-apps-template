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
	ErrNotFound             = errors.New("record not found")
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
	ErrSCIMNotFound         = errors.New("SCIM resource not found")
	ErrSCIMConflict         = errors.New("SCIM resource conflict")
	ErrSCIMPrecondition     = errors.New("SCIM resource precondition failed")
	ErrSCIMInvalid          = errors.New("SCIM request is invalid")
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
	SCIMManage        Permission = "scim:manage"
)

type Principal struct {
	UserID      UserID
	WorkspaceID WorkspaceID
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
	UserID      UserID
	WorkspaceID WorkspaceID
}

type WorkspaceSummary struct {
	ID     WorkspaceID
	Name   string
	Role   Role
	Locale string
}

type WorkspaceCreateInput struct {
	Name   string
	Locale string
}

type MemberSummary struct {
	ID        MemberID
	UserID    UserID
	Email     string
	Role      Role
	CreatedAt time.Time
}

type UserProfile struct {
	ID              UserID
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
	ID               NotificationID
	UserID           UserID
	WorkspaceID      WorkspaceID
	NotificationType string
	Title            string
	Body             string
	CreatedAt        time.Time
	ReadAt           *time.Time
}

type NotificationInput struct {
	RecipientUserID  UserID
	WorkspaceID      WorkspaceID
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
	UserID         UserID
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
	UserID         UserID          `json:"-"`
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
	ID              InvitationID
	WorkspaceID     WorkspaceID
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
	ID          APITokenID
	UserID      UserID
	WorkspaceID WorkspaceID
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
	ID          SCIMTokenID
	WorkspaceID WorkspaceID
	CreatedAt   time.Time
	Secret      string
}

type SCIMUser struct {
	ID          string
	UserID      UserID
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

// OIDCPurpose is the flow an OIDC transaction was started for.
type OIDCPurpose string

const (
	OIDCPurposeLogin OIDCPurpose = "login"
	OIDCPurposeLink  OIDCPurpose = "link"
)

type OIDCTransaction struct {
	ProviderID     string
	BrowserSession string
	Purpose        OIDCPurpose
	UserID         UserID
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
	ActorUserID   UserID
	AuthMethod    string
	WorkspaceID   WorkspaceID
	TargetType    string
	TargetID      string
	Action        string
	Outcome       string
	RequestID     string
	SourceAddress string
	Metadata      string
	CreatedAt     time.Time
}

// AuditHistoryLimit bounds the browser audit history projection.
const AuditHistoryLimit = 100

// AuditHistoryEntry contains only fields safe for the workspace browser.
type AuditHistoryEntry struct {
	EventType   string
	ActorUserID string
	AuthMethod  string
	TargetType  string
	Action      string
	Outcome     string
	CreatedAt   time.Time
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
