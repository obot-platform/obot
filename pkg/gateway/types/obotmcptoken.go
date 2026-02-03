//nolint:revive
package types

import (
	"time"
)

// ObotMCPToken represents an MCP token for authenticating with the integrated MCP server.
// The token format is: mt1-<user_id>-<token_id>-<secret>
// Lookups are done by token ID (extracted from the token), then bcrypt.CompareHashAndPassword
// is used to verify the secret portion.
type ObotMCPToken struct {
	ID           uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID       uint       `json:"userId" gorm:"index"`
	HashedSecret string     `json:"-"` // bcrypt hash of the secret portion only
	CreatedAt    time.Time  `json:"createdAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty"`
}

// ObotMCPTokenCreateResponse is returned when creating an MCP token.
// This is the only time the full token is visible.
type ObotMCPTokenCreateResponse struct {
	ObotMCPToken
	Token string `json:"token"` // The full token, only shown once
}
