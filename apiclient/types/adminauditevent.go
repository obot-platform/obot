package types

import "encoding/json"

// AuditLogActorTypeSystem identifies Obot itself, for a change it made on its own, such as marking a vMCP's agent
// OAuth grant invalid after its provider refused to refresh it.
const AuditLogActorTypeSystem AuditLogActorType = "system"

// AdminAuditEvent is one change an administrator, or Obot on its own, made to an agent, an agent's API key, a policy
// or profile that names agents, or a vMCP's agent OAuth grants. It never carries a secret value.
type AdminAuditEvent struct {
	ID        uint `json:"id"`
	CreatedAt Time `json:"createdAt"`
	// Actor is the administrator, as a user, who made the change, or the system. DisplayName is the user's, when the
	// user can still be found.
	Actor AuditLogActor `json:"actor"`
	// Action is what was done, such as agent.create, agent.key.revoke or vmcp.agent_oauth.authorize.
	Action string `json:"action"`
	// ResourceKind is agent, apiKey, vmcp, vmcpProfile, modelAccessPolicy or skillAccessRule, and ResourceID the ID of
	// the resource: for a profile, its profile ID. A vMCP's agent OAuth grant is recorded against the vMCP, with the
	// component in Details.
	ResourceKind string `json:"resourceKind"`
	ResourceID   string `json:"resourceID"`
	// ParentID is what holds the resource: the agent of an API key, or the vMCP of a profile.
	ParentID string `json:"parentID,omitempty"`
	// Details depends on the action. It names what changed (fields, subjects, scopes, components), never
	// the values of secrets.
	Details json.RawMessage `json:"details,omitempty"`
}

// AdminAuditEventList is a page of admin audit events, newest first.
type AdminAuditEventList struct {
	Items []AdminAuditEvent `json:"items"`
	// Total is how many events match the filters, over all pages.
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}
