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

	// MCPServerNames contains Kubernetes resource names of MCPServers this key can access.
	// Used for single-user, remote, and composite server types.
	// Empty slice means unrestricted access to all servers the user can access.
	MCPServerNames []string `json:"mcpServerNames,omitempty"`

	// MCPServerInstanceNames contains Kubernetes resource names of MCPServerInstances.
	// Used for multi-user server types where each user has their own instance.
	// Empty slice means unrestricted access to all instances the user owns.
	MCPServerInstanceNames []string `json:"mcpServerInstanceNames,omitempty"`
}

// APIKeyCreateResponse is returned when creating an API key.
// This is the only time the full key is visible.
type APIKeyCreateResponse struct {
	APIKey
	Key string `json:"key"` // The full key, only shown once
}
