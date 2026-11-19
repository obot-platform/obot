//nolint:revive
package types

import (
	"time"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"golang.org/x/oauth2"
)

const (
	TokenRequestPurposeDeviceLogin = "device-login"
	TokenRequestPurposeSetup       = "setup"

	// TokenRequestPurposeAuthProviderVerify backs the one-time login used to prove a staged
	// replacement auth provider works before it is activated.
	TokenRequestPurposeAuthProviderVerify = "auth-provider-verify"
)

type AuthToken struct {
	ID                    string    `json:"id" gorm:"index:idx_id_hashed_token"`
	UserID                uint      `json:"-" gorm:"index"`
	AuthProviderNamespace string    `json:"-" gorm:"index"`
	AuthProviderName      string    `json:"-" gorm:"index"`
	AuthProviderUserID    string    `json:"-"`
	HashedToken           string    `json:"-" gorm:"index:idx_id_hashed_token"`
	CreatedAt             time.Time `json:"createdAt"`
	ExpiresAt             time.Time `json:"expiresAt,omitzero"`
	NoExpiration          bool      `json:"noExpiration"`
}

type TokenRequest struct {
	ID                    string `gorm:"primaryKey"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Purpose               string    `gorm:"index"`
	DeviceCodeDigest      *string   `gorm:"uniqueIndex"`
	RequestExpiresAt      time.Time `gorm:"index"`
	DeviceCodeVerifiedAt  *time.Time
	State                 string `gorm:"index"`
	Name                  string
	Description           string
	Scopes                APIKeyScopes `gorm:"embedded"`
	Token                 string
	NoExpiration          bool
	ExpiresAt             time.Time
	CompletionRedirectURL string
	Error                 string
	TokenRetrieved        bool

	// OwnerUserID is the user who started a staged auth provider verification. Only they may open
	// the resulting login, so a verification cannot be picked up by another signed-in user.
	OwnerUserID *uint
}

type MCPOAuthToken struct {
	oauth2.Endpoint
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       string

	MCPID              string `gorm:"primaryKey"`
	UserID             string `gorm:"primaryKey"`
	URL                string
	OAuthAuthRequestID string `gorm:"index"`
	GrantID            string
	UpstreamChallenge  string
	AccessToken        string
	TokenType          string
	RefreshToken       string
	Expiry             time.Time
	ExpiresIn          int64

	// AuthorizedBy and AuthorizedAt record who authorized a vMCP's agent OAuth grant (UserID is a
	// principal.AgentOAuthGrantOwner), and when: the encoded principal of the administrator. They are empty for
	// everyone else's tokens, and a refresh keeps them.
	AuthorizedBy string
	AuthorizedAt *time.Time
	// InvalidatedAt and InvalidReason record that a vMCP's agent OAuth grant stopped working, such as when its refresh
	// token was refused (invalid_grant). Every agent that uses the grant is refused until an administrator authorizes
	// it again, which replaces the row and clears them. They never carry a token.
	InvalidatedAt *time.Time
	InvalidReason string

	Encrypted bool
}

// MCPOAuthGrantMetadata is what is known about a stored MCP OAuth token without its secrets: none of its tokens or
// client credentials. It is read for vMCPs' agent OAuth grants, to select one for an agent and to report its state.
type MCPOAuthGrantMetadata struct {
	MCPID         string
	UserID        string
	URL           string
	GrantID       string
	Scopes        string
	AuthorizedBy  string
	AuthorizedAt  *time.Time
	InvalidatedAt *time.Time
	InvalidReason string
}

// MCPOAuthTokenKey identifies one stored MCP OAuth token.
type MCPOAuthTokenKey struct {
	UserID string
	MCPID  string
}

type MCPOAuthPendingState struct {
	UpstreamChallenge string
	HashedState       string `gorm:"primaryKey"`
	State             string
	Verifier          string
	UserID            string `gorm:"index:idx_pending_user_mcp"`
	// InitiatorUserID is the Obot user who started the attempt. It differs from
	// UserID for "system"-owned tool previews, whose callback must still come from
	// the user who requested the preview, not from whoever opens the authorize URL.
	InitiatorUserID string
	MCPID           string `gorm:"index:idx_pending_user_mcp"`
	URL             string

	// TunnelName is the MCPTunnel the server is reached through, if any. The callback's
	// code exchange goes through the same tunnel, since the token endpoint may be private.
	TunnelName         string
	ResourceURL        string
	OAuthAuthRequestID string
	ClientID           string
	ClientSecret       string
	AuthURL            string
	TokenURL           string
	AuthStyle          oauth2.AuthStyle
	RedirectURL        string
	Scopes             string
	Encrypted          bool
	CreatedAt          time.Time

	// GrantInitiatorUserID and GrantDefinitionHash bind the pending authorization of a vMCP's agent OAuth grant
	// (UserID is a principal.AgentOAuthGrantOwner): the callback is accepted only from the administrator who started
	// it, and only while the component still has the definition it was started for. Both are empty for every other
	// authorization.
	GrantInitiatorUserID string
	GrantDefinitionHash  string
}

// MCPOAuthChallenge is a short-lived, single-transaction handoff, never a grant.
type MCPOAuthChallenge struct {
	ID            string `gorm:"primaryKey"`
	CreatedAt     time.Time
	ExpiresAt     time.Time `gorm:"index"`
	UserID        string
	Namespace     string
	MCPID         string
	ClientID      string
	Resource      string
	AuthRequestID string
	Requirements  []v1.UpstreamOAuthRequirement `gorm:"serializer:json"`
}

// MCPOAuthAttemptBudget bounds repeated challenge/OAuth cycles independently of random tickets.
type MCPOAuthAttemptBudget struct {
	ID        string    `gorm:"primaryKey"`
	ExpiresAt time.Time `gorm:"index"`
	Starts    int
}
