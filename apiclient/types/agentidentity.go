package types

// AgentIdentityManifest is the metadata an admin sets on an agent identity. It is all of an agent's configuration
// that an admin writes: whether an agent is enabled, and what it is granted, are separate.
type AgentIdentityManifest struct {
	// DisplayName names the agent for people. It is required, and need not be unique: an agent is identified by its ID.
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
	// OwnerUserIDs are the IDs of the active users who are the agent's administrative contacts. Ownership is
	// metadata only: it grants an owner no rights over the agent.
	OwnerUserIDs []string `json:"ownerUserIDs,omitempty"`
}

// AgentIdentity is a non-human identity that calls Obot with its own API keys. It is not a hosted agent or an agent on
// a device; the path of its API is /api/agents.
type AgentIdentity struct {
	// Metadata.ID is the decimal ID of the agent.
	Metadata
	AgentIdentityManifest `json:",inline"`

	// CreatorUserID is the ID of the admin who created the agent.
	CreatorUserID string `json:"creatorUserID,omitempty"`
	// Status is active, disabled, or deleted. A deleted agent is only listed with includeDeleted.
	Status UserStatus `json:"status"`
	// DisabledAt and DisabledReason are set only while the agent is disabled.
	DisabledAt     *Time  `json:"disabledAt,omitempty"`
	DisabledReason string `json:"disabledReason,omitempty"`
	// ActiveAPIKeyCount is how many of the agent's API keys neither are revoked nor have expired. It is always
	// present, so a zero is shown as 0.
	ActiveAPIKeyCount int `json:"activeAPIKeyCount"`
}

type AgentIdentityList List[AgentIdentity]

// AgentIdentityDisableRequest is the body of a request to disable an agent.
type AgentIdentityDisableRequest struct {
	// Reason is why the agent is disabled. It is optional, and is shown with the agent.
	Reason string `json:"reason,omitempty"`
}

// AgentAPIKeyCreateRequest is the body of a request to create an API key for an agent. Its scopes work as those of a
// user's API key do: a scope that is omitted is not granted, and the key must have at least one. An agent's key never
// grants access to the API or to device scans, so the request has no field for either, and a request that names one is
// refused.
type AgentAPIKeyCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// ExpiresAt is when the key stops working. A key with none does not expire.
	ExpiresAt *Time `json:"expiresAt,omitempty"`

	CanAccessSkills   bool `json:"canAccessSkills,omitempty"`
	CanAccessLLMProxy bool `json:"canAccessLLMProxy,omitempty"`
	// MCPServerIDs are the IDs of the vMCPs the key can connect to, or "*" for every vMCP the agent is granted.
	MCPServerIDs []string `json:"mcpServerIds,omitempty"`
}

// AgentAPIKey is an API key of an agent, without its secret. It has its own type rather than the type of a user's
// key, which has the API and device scan scopes that an agent's key never has.
type AgentAPIKey struct {
	ID uint `json:"id"`
	// AgentID is the decimal ID of the agent the key belongs to.
	AgentID     string `json:"agentId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	CanAccessSkills   bool `json:"canAccessSkills"`
	CanAccessLLMProxy bool `json:"canAccessLLMProxy"`
	// MCPServerIDs is always present, so that a key with no vMCPs shows an empty list rather than none.
	MCPServerIDs []string `json:"mcpServerIds"`

	CreatedAt  Time  `json:"createdAt"`
	LastUsedAt *Time `json:"lastUsedAt,omitempty"`
	ExpiresAt  *Time `json:"expiresAt,omitempty"`
	RevokedAt  *Time `json:"revokedAt,omitempty"`
}

type AgentAPIKeyList List[AgentAPIKey]

// AgentAPIKeyCreateResponse is the response to the creation of an agent's API key.
type AgentAPIKeyCreateResponse struct {
	AgentAPIKey `json:",inline"`
	// Key is the full API key. It is only ever shown in this response.
	Key string `json:"key"`
}

// SubjectMatch is how a policy's subjects name an identity.
type SubjectMatch string

const (
	// SubjectMatchExplicit is a subject that names the identity itself, such as {agent, "7"}.
	SubjectMatchExplicit SubjectMatch = "explicit"
	// SubjectMatchKindWildcard is the wildcard of the identity's kind: agents:* ({agent, "*"}) for an agent.
	SubjectMatchKindWildcard SubjectMatch = "kindWildcard"
	// SubjectMatchWildcard is the wildcard for everyone, subjects:* ({selector, "*"}).
	SubjectMatchWildcard SubjectMatch = "wildcard"
)

// AgentIdentityAccess is what the policies that can name agents grant an agent: the vMCP profiles, model access
// policies and skill access rules whose subjects match it, and how each matches. A policy that several of its
// subjects match is listed once, with its most specific match. It lists grants of a disabled agent too, which apply
// again once it is enabled. Whether the agent can use them, such as whether a vMCP is ready for it, is not part of it.
type AgentIdentityAccess struct {
	// AgentID is the decimal ID of the agent.
	AgentID string `json:"agentID"`
	// VMCPProfiles are the matching profiles of shared vMCPs.
	VMCPProfiles []AgentIdentityVMCPProfileAccess `json:"vmcpProfiles"`
	// ModelAccessPolicies are the matching model access policies (LLM gateway profiles).
	ModelAccessPolicies []AgentIdentityPolicyAccess `json:"modelAccessPolicies"`
	// SkillAccessRules are the matching skill access rules.
	SkillAccessRules []AgentIdentityPolicyAccess `json:"skillAccessRules"`
}

// AgentIdentityVMCPProfileAccess is a vMCP profile whose subjects match an agent.
type AgentIdentityVMCPProfileAccess struct {
	VMCPID          string       `json:"vmcpID"`
	VMCPDisplayName string       `json:"vmcpDisplayName,omitempty"`
	ProfileID       string       `json:"profileID,omitempty"`
	ProfileName     string       `json:"profileName"`
	Match           SubjectMatch `json:"match"`
}

// AgentIdentityPolicyAccess is a model access policy or skill access rule whose subjects match an agent.
type AgentIdentityPolicyAccess struct {
	ID          string       `json:"id"`
	DisplayName string       `json:"displayName,omitempty"`
	Match       SubjectMatch `json:"match"`
}
