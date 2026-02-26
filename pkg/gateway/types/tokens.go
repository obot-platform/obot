//nolint:revive
package types

import (
	"time"

	"golang.org/x/oauth2"
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
	State                 string `gorm:"index"`
	Nonce                 string
	Token                 string
	NoExpiration          bool
	ExpiresAt             time.Time
	CompletionRedirectURL string
	Error                 string
	TokenRetrieved        bool
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
	AccessToken        string
	TokenType          string
	RefreshToken       string
	Expiry             time.Time
	ExpiresIn          int64

	Encrypted bool
}

type MCPOAuthPendingState struct {
	HashedState        string `gorm:"primaryKey"`
	State              string
	Verifier           string
	UserID             string `gorm:"index:idx_pending_user_mcp"`
	MCPID              string `gorm:"index:idx_pending_user_mcp"`
	URL                string
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
}
