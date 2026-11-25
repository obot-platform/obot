//nolint:revive
package types

import (
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
)

// APIKey represents an API key for a user or an agent to access Obot.
// The key format is ok1-<user_id>-<key_id>-<secret> for a user's key, and oak1-<agent_id>-<key_id>-<secret> for an
// agent's key. Lookups are done by key ID and owner (extracted from the token), then bcrypt.CompareHashAndPassword
// is used to verify the secret portion.
type APIKey struct {
	APIKeyScopes `json:",inline"`

	ID uint `json:"id" gorm:"primaryKey;autoIncrement"`
	// UserID is set on a user's key, and AgentID on an agent's key. The chk_api_keys_owner constraint requires
	// exactly one of them, so a key never has two owners or none. Use Owner rather than reading them directly.
	UserID  *uint `json:"userId,omitempty" gorm:"index"`
	AgentID *uint `json:"agentId,omitempty" gorm:"index;check:chk_api_keys_owner,(user_id IS NULL) <> (agent_id IS NULL)"`

	// ClaudeEnvironment is set only by the server when issuing runner credentials.
	ClaudeEnvironment bool `json:"-"`

	Name         string     `json:"name"`                  // User-provided name for the key
	Description  string     `json:"description,omitempty"` // Optional description
	HashedSecret string     `json:"-"`                     // bcrypt hash of the secret portion only
	CreatedAt    time.Time  `json:"createdAt"`
	LastUsedAt   *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"` // nil means no expiration
	RevokedAt    *time.Time `json:"revokedAt,omitempty" gorm:"index"`

	// OwnerStatus is the lifecycle status of the key's user or agent, read during validation.
	OwnerStatus types.UserStatus `json:"-" gorm:"-"`
}

type APIKeyScopes struct {
	CanAccessAPI         bool `json:"canAccessAPI" gorm:"default:false;not null"`
	CanAccessSkills      bool `json:"canAccessSkills" gorm:"default:false;not null"`
	CanAccessLLMProxy    bool `json:"canAccessLLMProxy" gorm:"default:false;not null"`
	CanAccessDeviceScans bool `json:"canAccessDeviceScans" gorm:"default:false;not null"`

	// MCPServerIDs contains resource names of MCPServers or VMCPs this key can access.
	// Supports all server types: single-user, multi-user, remote, and composite.
	// Use "*" as a wildcard to grant access to all servers the user can access.
	// This may be empty for skills-only API keys.
	MCPServerIDs []string `json:"mcpServerIds,omitempty" gorm:"serializer:json"`
}

// APIKeyOwnerLifecycle is the lifecycle state of an API key's user or agent, read with the key.
type APIKeyOwnerLifecycle struct {
	OwnerID         *uint
	OwnerDeletedAt  *time.Time
	OwnerDisabledAt *time.Time
}

// AgentAPIKeyCreateRequest describes an API key to create for an agent. Its scopes work as those of a user's key do:
// each one that is omitted is not granted, and the key must have at least one. An agent's key can only narrow what
// the agent is granted, which is computed whenever the key is used. It never has CanAccessAPI or
// CanAccessDeviceScans, so the request has no field for them.
type AgentAPIKeyCreateRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"` // nil means no expiration

	CanAccessSkills   bool `json:"canAccessSkills,omitempty"`
	CanAccessLLMProxy bool `json:"canAccessLLMProxy,omitempty"`
	// MCPServerIDs are the vMCPs the key can connect to, or "*" for every vMCP the agent is granted.
	MCPServerIDs []string `json:"mcpServerIds,omitempty"`
}

// APIKeyCreateResponse is returned when creating an API key.
// This is the only time the full key is visible.
type APIKeyCreateResponse struct {
	APIKey
	Key string `json:"key"` // The full key, only shown once
}

// Owner returns the user or agent that owns the key. A key with both owners or none, which the database refuses to
// store, has the zero Principal, which is neither a user nor an agent.
func (k APIKey) Owner() principal.Principal {
	switch {
	case k.UserID != nil && k.AgentID == nil:
		return principal.User(*k.UserID)
	case k.AgentID != nil && k.UserID == nil:
		return principal.Agent(*k.AgentID)
	default:
		return principal.Principal{}
	}
}

// Scopes returns the scopes of the key r describes.
func (r AgentAPIKeyCreateRequest) Scopes() APIKeyScopes {
	return APIKeyScopes{
		CanAccessSkills:   r.CanAccessSkills,
		CanAccessLLMProxy: r.CanAccessLLMProxy,
		MCPServerIDs:      r.MCPServerIDs,
	}
}

// Status returns the owner's lifecycle status. An owner with no user or agent row is treated as deleted.
func (o APIKeyOwnerLifecycle) Status() types.UserStatus {
	switch {
	case o.OwnerID == nil || o.OwnerDeletedAt != nil:
		return types.UserStatusDeleted
	case o.OwnerDisabledAt != nil:
		return types.UserStatusDisabled
	default:
		return types.UserStatusActive
	}
}

func (as APIKeyScopes) Groups(u *User) []string {
	groups := make([]string, 0, 7)
	if as.CanAccessAPI {
		groups = append(groups, types.GroupAPI)
		if u != nil {
			groups = append(groups, u.Role.RoleGroups()...)
		}
	}

	if as.CanAccessSkills {
		groups = append(groups, types.GroupSkills)
	}
	if as.CanAccessLLMProxy {
		groups = append(groups, types.GroupLLM)
	}
	if as.CanAccessDeviceScans {
		groups = append(groups, types.GroupDeviceScans)
	}
	if len(as.MCPServerIDs) != 0 {
		groups = append(groups, types.GroupMCP)
	}
	return append(groups, types.GroupAuthenticated)
}

func (as APIKeyScopes) HasSomeScope() bool {
	return as.CanAccessAPI || as.CanAccessSkills || as.CanAccessLLMProxy || as.CanAccessDeviceScans || len(as.MCPServerIDs) != 0
}
