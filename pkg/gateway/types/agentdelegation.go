package types

import (
	"time"
)

const (
	// AgentDelegationRequestPending is a request that waits for a user's approval.
	AgentDelegationRequestPending AgentDelegationRequestState = "pending"
	// AgentDelegationRequestApproved is a request that a user approved, whose code the agent has yet to redeem.
	AgentDelegationRequestApproved AgentDelegationRequestState = "approved"
	// AgentDelegationRequestRedeemed is a request whose code was used up: redeemed, or presented with the wrong secret.
	AgentDelegationRequestRedeemed AgentDelegationRequestState = "redeemed"
	// AgentDelegationRequestCancelled is a request that the user turned down.
	AgentDelegationRequestCancelled AgentDelegationRequestState = "cancelled"
)

// AgentDelegationRequestState is where an AgentDelegationRequest is in the approval protocol.
type AgentDelegationRequestState string

// AgentDelegationRequest is an agent's request that a user let it act on their behalf. The agent creates it with its
// API key, and the user approves or cancels it in their browser. Approval sends the browser back to the agent with a
// one-time code, which the agent redeems with its key and the secret it kept in the user's session with it. Only
// digests of the code and of the secret are stored.
type AgentDelegationRequest struct {
	ID string `json:"id" gorm:"primaryKey"`
	// AgentID is the agent that asked. idx_agent_delegation_requests_agent_state serves counting its pending requests.
	AgentID uint                        `json:"agentID" gorm:"not null;index:idx_agent_delegation_requests_agent_state,priority:1"`
	State   AgentDelegationRequestState `json:"state" gorm:"not null;index:idx_agent_delegation_requests_agent_state,priority:2"`
	// APIKeyID is the agent's key that created the request.
	APIKeyID uint `json:"apiKeyID" gorm:"not null"`
	// ReturnURL is where the user's browser is sent after approving or cancelling. It was on the agent's allowed return
	// hosts when the request was created.
	ReturnURL string `json:"returnURL" gorm:"not null"`
	// SecretHash is the S256 hash (unpadded base64url SHA-256) of the secret the agent kept in the user's session.
	SecretHash string `json:"-" gorm:"not null"`
	// LoginHint is passed on to sign-in. It is a convenience, never a check of who approves.
	LoginHint string `json:"loginHint,omitempty"`
	// DelegationID is, until approval, the delegation the agent asked to have approved again, if any, and after it,
	// the delegation the approval created or updated.
	DelegationID string `json:"delegationID,omitempty" gorm:"index"`
	// ApprovedUserID is the user who approved the request, and 0 until one has.
	ApprovedUserID uint       `json:"approvedUserID,omitempty" gorm:"not null;default:0;index"`
	ApprovedAt     *time.Time `json:"approvedAt,omitempty"`
	// CodeHash is the hex SHA-256 digest of the code issued at approval. It is cleared once the code is used up.
	CodeHash      string     `json:"-"`
	CodeExpiresAt *time.Time `json:"codeExpiresAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	// ExpiresAt is when the request can no longer be approved or cancelled. A code issued before then expires on its
	// own schedule (CodeExpiresAt).
	ExpiresAt time.Time `json:"expiresAt" gorm:"not null;index"`
}

// AgentDelegation is a user's approval for an agent to act on their behalf. There is at most one for each agent and
// user: approving again updates it and keeps its ID, so tokens minted from it keep working. It belongs to the agent,
// not to a key: any active key of the agent can mint tokens from it.
type AgentDelegation struct {
	ID string `json:"id" gorm:"primaryKey"`
	// AgentID and UserID are unique together. idx_agent_delegations_agent_user serves the agent's delegations, and
	// the index on UserID the user's.
	AgentID uint `json:"agentID" gorm:"not null;uniqueIndex:idx_agent_delegations_agent_user,priority:1"`
	UserID  uint `json:"userID" gorm:"not null;uniqueIndex:idx_agent_delegations_agent_user,priority:2;index"`
	// APIKeyID is the agent's key that created the request the user approved last. It is recorded for audit only.
	APIKeyID        uint       `json:"apiKeyID"`
	FirstApprovedAt time.Time  `json:"firstApprovedAt"`
	LastApprovedAt  time.Time  `json:"lastApprovedAt"`
	LastUsedAt      *time.Time `json:"lastUsedAt,omitempty"`
}
