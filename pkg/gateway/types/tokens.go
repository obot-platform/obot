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
	HashedToken           string    `json:"-" gorm:"index:idx_id_hashed_token"`
	CreatedAt             time.Time `json:"createdAt"`
	ExpiresAt             time.Time `json:"expiresAt"`
}

type TokenRequest struct {
	ID                    string `gorm:"primaryKey"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	State                 string `gorm:"index"`
	Nonce                 string
	Token                 string
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

	State       string
	HashedState *string `gorm:"unique"`
	Verifier    string

	MCPServerInstance string `gorm:"primaryKey"`
	AccessToken       string
	TokenType         string
	RefreshToken      string
	Expiry            time.Time
	ExpiresIn         int64

	Encrypted bool
}
