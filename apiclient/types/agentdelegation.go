package types

const (
	// AgentDelegationIDPrefix starts the ID of every agent delegation.
	AgentDelegationIDPrefix = "adl1"
	// AgentDelegationRequestIDPrefix starts the ID of every agent delegation request.
	AgentDelegationRequestIDPrefix = "adr1"

	// AgentDelegationGrantCode redeems the code that a user's approval sent back to the agent
	// (AgentDelegationTokenRequest).
	AgentDelegationGrantCode = "code"
	// AgentDelegationGrantDelegation mints a new token from a delegation the agent already holds.
	AgentDelegationGrantDelegation = "delegation"
)

// AgentDelegationRequestCreate is the body with which an agent asks for a link a user can open to let it act on their
// behalf (POST /api/agent-delegations). The agent authenticates with its API key.
type AgentDelegationRequestCreate struct {
	// ReturnURL is where the user's browser is sent back to after approving or cancelling: on approval with the
	// requestID and code query parameters, and on cancellation with error=access_denied. It must use https and have
	// one of the agent's allowed return hosts (AgentIdentityManifest.AllowedReturnHosts).
	ReturnURL string `json:"returnURL"`
	// SecretHash is the S256 hash of a random secret that the agent keeps in the session of the user's browser: the
	// unpadded base64url SHA-256 digest of it, as a PKCE code challenge is. The secret is 43 to 128 of the characters
	// A-Z, a-z, 0-9, "-", ".", "_" and "~". Only the agent's backend, with the secret from the session of the browser
	// that comes back, can redeem the code, so an approval link that a user forwards to someone else is useless.
	SecretHash string `json:"secretHash"`
	// LoginHint is passed on to sign-in, as a convenience. It is never a check of who approves.
	LoginHint string `json:"loginHint,omitempty"`
	// DelegationID is the agent's delegation that the request asks to have approved again, if any. It must be the
	// agent's, and only its user can approve the request.
	DelegationID string `json:"delegationID,omitempty"`
}

// AgentDelegationRequestCreated is the response to AgentDelegationRequestCreate.
type AgentDelegationRequestCreated struct {
	RequestID string `json:"requestID"`
	// ApprovalURL is the page, in Obot, at which the user approves the request. It works once, and until ExpiresAt.
	ApprovalURL string `json:"approvalURL"`
	ExpiresAt   Time   `json:"expiresAt"`
}

// AgentDelegationApprovalRequest is an agent's request, as the signed-in user who can approve it sees it: the agent,
// and what it will be able to do for them.
type AgentDelegationApprovalRequest struct {
	ID        string               `json:"id"`
	Agent     AgentDelegationAgent `json:"agent"`
	ExpiresAt Time                 `json:"expiresAt"`
	// DelegationID is the user's existing delegation of the agent, if they approved it before. Approving again
	// keeps it.
	DelegationID string `json:"delegationID,omitempty"`
	// Access is what the agent may do for the user: its own grants.
	Access AgentDelegationAccess `json:"access"`
}

// AgentDelegationAgent is the agent of a delegation or of a request for one.
type AgentDelegationAgent struct {
	ID          string     `json:"id"`
	DisplayName string     `json:"displayName"`
	Description string     `json:"description,omitempty"`
	Status      UserStatus `json:"status"`
}

// AgentDelegationUser is the user of a delegation.
type AgentDelegationUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email,omitempty"`
	DisplayName string     `json:"displayName,omitempty"`
	Status      UserStatus `json:"status,omitempty"`
}

// AgentDelegationAccess is what an agent may do for a user one of its profiles matches: the agent's own grants, which
// are the same for every such user.
type AgentDelegationAccess struct {
	// VMCPs are the vMCPs, and the profiles of each that name the agent, that the agent may use for the user.
	VMCPs []AgentDelegationVMCPAccess `json:"vmcps"`
	// ModelAccessPolicies are the agent's LLM gateway policies, whose models and budgets apply to the agent for the
	// user.
	ModelAccessPolicies []AgentIdentityPolicyAccess `json:"modelAccessPolicies"`
	// SkillAccessRules are the agent's skill access rules, whose skills the agent may use for the user.
	SkillAccessRules []AgentIdentityPolicyAccess `json:"skillAccessRules"`
}

// AgentDelegationVMCPAccess is a vMCP that an agent may use for a user, through the profiles listed.
type AgentDelegationVMCPAccess struct {
	VMCPID          string                       `json:"vmcpID"`
	VMCPDisplayName string                       `json:"vmcpDisplayName,omitempty"`
	Profiles        []AgentDelegationVMCPProfile `json:"profiles"`
}

// AgentDelegationVMCPProfile is a profile of a vMCP.
type AgentDelegationVMCPProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AgentDelegationRedirect is the response to approving or cancelling a request: the URL to send the user's browser to.
// It is the agent's return URL, with the request ID and code on approval, or with error=access_denied on cancellation.
type AgentDelegationRedirect struct {
	RedirectURL string `json:"redirectURL"`
}

// AgentDelegationTokenRequest is the body with which an agent gets a token to act on behalf of a user
// (POST /api/agent-delegations/token). The agent authenticates with its API key, which becomes the token's minting
// key.
type AgentDelegationTokenRequest struct {
	// GrantType is AgentDelegationGrantCode or AgentDelegationGrantDelegation.
	GrantType string `json:"grantType"`
	// RequestID, Code and Secret redeem the code of an approved request (AgentDelegationGrantCode). The secret is the
	// one whose hash the request was created with, from the session of the browser the code came back with. A code is
	// used once: a wrong or missing secret uses it up too.
	RequestID string `json:"requestID,omitempty"`
	Code      string `json:"code,omitempty"`
	Secret    string `json:"secret,omitempty"`
	// DelegationID mints a new token from a delegation of the agent (AgentDelegationGrantDelegation).
	DelegationID string `json:"delegationID,omitempty"`
}

// AgentDelegationToken is a token with which an agent acts on behalf of a user. There is no refresh token: the agent
// mints a new token from the delegation when it needs one.
type AgentDelegationToken struct {
	DelegationID string `json:"delegationID"`
	AccessToken  string `json:"accessToken"`
	// TokenType is always "Bearer".
	TokenType string `json:"tokenType"`
	ExpiresAt Time   `json:"expiresAt"`
	// User is the user the agent acts for.
	User AgentDelegationUser `json:"user"`
}

// AgentDelegation is a user's approval for an agent to act on their behalf.
type AgentDelegation struct {
	ID              string               `json:"id"`
	Agent           AgentDelegationAgent `json:"agent"`
	User            AgentDelegationUser  `json:"user"`
	FirstApprovedAt Time                 `json:"firstApprovedAt"`
	LastApprovedAt  Time                 `json:"lastApprovedAt"`
	// LastUsedAt is when the agent last got a token from the delegation.
	LastUsedAt *Time `json:"lastUsedAt,omitempty"`
}

type AgentDelegationList List[AgentDelegation]
