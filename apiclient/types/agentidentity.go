package types

// AgentIdentityManifest is the metadata an admin sets on an agent identity. It is all of an agent's configuration
// that an admin writes: whether an agent is enabled, and what it is granted, are separate.
type AgentIdentityManifest struct {
	// DisplayName names the agent for people. It is required, and need not be unique: an agent is identified by its ID.
	DisplayName string `json:"displayName"`
	Description string `json:"description,omitempty"`
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
// again once it is enabled. Whether the agent can use a vMCP is the readiness API's (AgentVMCPReadiness). The summary only
// reports, as Problems, scopes of the agent's keys that none of these grants anything for.
type AgentIdentityAccess struct {
	// AgentID is the decimal ID of the agent.
	AgentID string `json:"agentID"`
	// VMCPProfiles are the matching profiles of shared vMCPs.
	VMCPProfiles []AgentIdentityVMCPProfileAccess `json:"vmcpProfiles"`
	// ModelAccessPolicies are the matching model access policies (LLM gateway profiles).
	ModelAccessPolicies []AgentIdentityPolicyAccess `json:"modelAccessPolicies"`
	// SkillAccessRules are the matching skill access rules.
	SkillAccessRules []AgentIdentityPolicyAccess `json:"skillAccessRules"`
	// Problems are scopes of the agent's active API keys that nothing grants access to: the LLM gateway without a model
	// access policy, and skills without a skill access rule.
	Problems []AgentIdentityAccessProblem `json:"problems"`
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

const (
	// AgentReadinessConfigurationRequired is a reason an agent cannot use a vMCP: a component enabled for it requires a
	// configuration value that the agent's profiles do not supply. Field names the key.
	AgentReadinessConfigurationRequired = "configuration_required"
	// AgentReadinessOAuthRequired is a reason an agent cannot use a vMCP: a component enabled for it requires OAuth, and
	// the agent's profiles hold no grant for it.
	AgentReadinessOAuthRequired = "oauth_required"
	// AgentReadinessOAuthInvalid is a reason an agent cannot use a vMCP: the grant its profile holds for a component no
	// longer works, and must be authorized again.
	AgentReadinessOAuthInvalid = "oauth_invalid"
	// AgentReadinessSelectionAmbiguous is a reason an agent cannot use a vMCP: more than one profile names every agent,
	// so the profile an agent falls back to is unknown. Without a component, it is about the whole vMCP.
	AgentReadinessSelectionAmbiguous = "selection_ambiguous"

	// AgentSourceConfiguration is the kind of an AgentReadinessSource for a configuration value.
	AgentSourceConfiguration = "configuration"
	// AgentSourceOAuth is the kind of an AgentReadinessSource for an OAuth grant.
	AgentSourceOAuth = "oauth"
)

// AgentReadinessReason is one reason an agent cannot use a vMCP. It never carries a configuration value.
type AgentReadinessReason struct {
	// ComponentID is the component the reason is about. It is empty for a reason about the whole vMCP.
	ComponentID string `json:"componentID,omitempty"`
	// Code is one of the AgentReadiness constants.
	Code string `json:"code"`
	// Field is the configuration key that is missing, never its value.
	Field string `json:"field,omitempty"`
	// ProfileID is the profile where an administrator fixes the reason: the agent's explicit profile if it has one,
	// otherwise the wildcard profile it falls back to. It is empty for AgentReadinessSelectionAmbiguous. For
	// AgentReadinessOAuthInvalid, it is the profile that holds the grant.
	ProfileID string `json:"profileID,omitempty"`
	// URLProfileID is, for an invalid grant that was authorized for another upstream URL than the one the agent's
	// connection reaches, the profile whose configuration sets the URL the agent reaches.
	URLProfileID string `json:"urlProfileID,omitempty"`
}

// AgentReadinessSource is the profile that supplies something an agent uses on a vMCP. It names the key, never the
// value, and never a token.
type AgentReadinessSource struct {
	// Kind is AgentSourceConfiguration or AgentSourceOAuth.
	Kind        string `json:"kind"`
	ComponentID string `json:"componentID"`
	// Field is the configuration key. It is empty for an OAuth grant.
	Field     string `json:"field,omitempty"`
	ProfileID string `json:"profileID"`
	// Fallback reports that it comes from the profile that names every agent, which the agent falls back to, rather
	// than from the profile that names the agent itself.
	Fallback bool `json:"fallback,omitempty"`
}

// AgentVMCPReadiness is whether an agent can use a vMCP, computed when it is asked for, with the same checks that
// decide whether the agent can connect. It carries no configuration value and no token.
type AgentVMCPReadiness struct {
	VMCPID          string `json:"vmcpID"`
	VMCPDisplayName string `json:"vmcpDisplayName,omitempty"`
	// Ready reports that the agent can connect to the vMCP. A disabled agent can be ready: it is refused at admission
	// until it is enabled.
	Ready bool `json:"ready"`
	// Disabled reports that the agent is disabled, so it cannot connect whatever its readiness is.
	Disabled bool `json:"disabled"`
	// ExplicitProfileID is the profile that names the agent by ID, if any.
	ExplicitProfileID string `json:"explicitProfileID,omitempty"`
	// WildcardProfileID is the profile that names every agent, if exactly one does.
	WildcardProfileID string `json:"wildcardProfileID,omitempty"`
	// Reasons are why the agent cannot use the vMCP. It is empty when it can.
	Reasons []AgentReadinessReason `json:"reasons"`
	// Sources are the profiles that supply the agent's configuration values and OAuth grants. The list of an agent's
	// vMCPs leaves it out.
	Sources []AgentReadinessSource `json:"sources,omitempty"`
	// PendingSync reports that the agent has connected before, and its connection has not yet received the
	// configuration its profiles now resolve to. The next connect waits for it.
	PendingSync bool `json:"pendingSync,omitempty"`
}

type AgentVMCPReadinessList List[AgentVMCPReadiness]

const (
	// AgentProblemNoModelAccessPolicy is the problem of an agent whose key can use the LLM gateway, when no model access
	// policy names the agent: every LLM request is refused.
	AgentProblemNoModelAccessPolicy = "no_model_access_policy"
	// AgentProblemNoSkillAccessRule is the problem of an agent whose key can use skills, when no skill access rule names
	// the agent: it sees no skills.
	AgentProblemNoSkillAccessRule = "no_skill_access_rule"
)

// AgentIdentityAccessProblem is something that keeps an agent from using what one of its active API keys is scoped
// for.
type AgentIdentityAccessProblem struct {
	// Code is AgentProblemNoModelAccessPolicy or AgentProblemNoSkillAccessRule.
	Code string `json:"code"`
}
