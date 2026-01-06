//nolint:revive
package types

import (
	"time"
)

// APIKey represents an API key for MCP server access.
// The key format is: ok1-<user_id>-<key_id>-<secret>
// Lookups are done by key ID (extracted from the token), then bcrypt.CompareHashAndPassword
// is used to verify the secret portion.
type APIKey struct {
	ID           uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID       uint       `json:"userId" gorm:"index"`
	Name         string     `json:"name"`                  // User-provided name for the key
	Description  string     `json:"description,omitempty"` // Optional description
	HashedSecret string     `json:"-"`                     // bcrypt hash of the secret portion only
	CreatedAt    time.Time  `json:"createdAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"` // nil means no expiration

	// MCPServerIDs contains Kubernetes resource names of MCPServers this key can access.
	// Supports all server types: single-user, multi-user, remote, and composite.
	// Use "*" as a wildcard to grant access to all servers the user can access.
	// At least one MCPServerID must be specified.
	MCPServerIDs []string `json:"mcpServerIds,omitempty" gorm:"serializer:json"`
}

// APIKeyCreateResponse is returned when creating an API key.
// This is the only time the full key is visible.
type APIKeyCreateResponse struct {
	APIKey
	Key string `json:"key"` // The full key, only shown once
}
