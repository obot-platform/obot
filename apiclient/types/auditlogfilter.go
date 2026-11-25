package types

// AuditLogAPIKeyFilterOption describes an API key that appears in the currently
// visible audit-log result set. Value is the stable filter value; the remaining
// fields are non-secret display context.
type AuditLogAPIKeyFilterOption struct {
	Value           string `json:"value"`
	Name            string `json:"name"`
	MaskedKey       string `json:"maskedKey"`
	UserID          string `json:"userID"`
	UserDisplayName string `json:"userDisplayName"`
	// AgentID and AgentDisplayName are set instead of UserID for the key of an agent, and are those of the agent's
	// record even after the agent is deleted.
	AgentID          string `json:"agentID,omitempty"`
	AgentDisplayName string `json:"agentDisplayName,omitempty"`
	Revoked          bool   `json:"revoked"`
}
