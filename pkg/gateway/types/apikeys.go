//nolint:revive
package types

import (
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

// APIKey represents an API key for a user to access the Obot API.
// The key format is: ok1-<user_id>-<key_id>-<secret>
// Lookups are done by key ID (extracted from the token), then bcrypt.CompareHashAndPassword
// is used to verify the secret portion.
type APIKey struct {
	APIKeyScopes `json:",inline"`

	ID           uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID       uint       `json:"userId" gorm:"index"`
	Name         string     `json:"name"`                  // User-provided name for the key
	Description  string     `json:"description,omitempty"` // Optional description
	HashedSecret string     `json:"-"`                     // bcrypt hash of the secret portion only
	CreatedAt    time.Time  `json:"createdAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"` // nil means no expiration
}

type APIKeyScopes struct {
	CanAccessSkills             bool `json:"canAccessSkills" gorm:"default:false;not null"`
	CanAccessAPI                bool `json:"canAccessAPI" gorm:"default:false;not null"`
	CanAccessLLMProxy           bool `json:"canAccessLLMProxy" gorm:"default:false;not null"`
	CanAccessPublishedArtifacts bool `json:"canAccessPublishedArtifacts" gorm:"default:false;not null"`

	// MCPServerIDs contains Kubernetes resource names of MCPServers this key can access.
	// Supports all server types: single-user, multi-user, remote, and composite.
	// Use "*" as a wildcard to grant access to all servers the user can access.
	// This may be empty for skills-only API keys.
	MCPServerIDs []string `json:"mcpServerIds,omitempty" gorm:"serializer:json"`
}

func (as APIKeyScopes) Groups(u *User) []string {
	if as.CanAccessAPI {
		if u == nil {
			return nil
		}
		return u.Role.Groups()
	}

	groups := make([]string, 0, 5)
	if as.CanAccessSkills {
		groups = append(groups, types.GroupSkills)
	}
	if as.CanAccessLLMProxy {
		groups = append(groups, types.GroupLLM)
	}
	if as.CanAccessPublishedArtifacts {
		groups = append(groups, types.GroupPublishedArtifacts)
	}
	if len(as.MCPServerIDs) != 0 {
		groups = append(groups, types.GroupMCP)
	}
	return append(groups, types.GroupAuthenticated)
}

// APIKeyCreateResponse is returned when creating an API key.
// This is the only time the full key is visible.
type APIKeyCreateResponse struct {
	APIKey
	Key string `json:"key"` // The full key, only shown once
}
