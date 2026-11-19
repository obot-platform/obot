package types

import (
	"encoding/json"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"gorm.io/datatypes"
)

const (
	// AdminAuditActorSystem is the actor of a change Obot made on its own, such as marking a grant invalid after the
	// provider refused to refresh it, or deleting the grant of a component an administrator removed.
	AdminAuditActorSystem = "system"

	AdminAuditResourceAgent             = "agent"
	AdminAuditResourceAPIKey            = "apiKey"
	AdminAuditResourceVMCP              = "vmcp"
	AdminAuditResourceVMCPProfile       = "vmcpProfile"
	AdminAuditResourceModelAccessPolicy = "modelAccessPolicy"
	AdminAuditResourceSkillAccessRule   = "skillAccessRule"

	AdminAuditActionAgentCreate  = "agent.create"
	AdminAuditActionAgentUpdate  = "agent.update"
	AdminAuditActionAgentDisable = "agent.disable"
	AdminAuditActionAgentEnable  = "agent.enable"
	AdminAuditActionAgentDelete  = "agent.delete"

	AdminAuditActionAgentKeyCreate = "agent.key.create"
	AdminAuditActionAgentKeyRevoke = "agent.key.revoke"

	AdminAuditActionModelAccessPolicySubjectsUpdate = "model_access_policy.subjects.update"
	AdminAuditActionSkillAccessRuleSubjectsUpdate   = "skill_access_rule.subjects.update"
	AdminAuditActionVMCPProfileSubjectsUpdate       = "vmcp.profile.subjects.update"

	// The agent OAuth grants of a vMCP are recorded against the vMCP (AdminAuditResourceVMCP), with the component in
	// the details.
	AdminAuditActionVMCPAgentOAuthAuthorize  = "vmcp.agent_oauth.authorize"
	AdminAuditActionVMCPAgentOAuthRevoke     = "vmcp.agent_oauth.revoke"
	AdminAuditActionVMCPAgentOAuthInvalidate = "vmcp.agent_oauth.invalidate"
	AdminAuditActionVMCPAgentOAuthDelete     = "vmcp.agent_oauth.delete"
)

// AdminAuditEvent records one change an administrator, or Obot on its own, made to something that decides what agents
// can do: agents, their API keys, the policies and profiles that name them, and the agent OAuth grants of vMCPs. The
// table is append-only, and an event outlives what it names: deleting an agent keeps its events.
//
// Details never carries a secret value: it names profile and component IDs, subject changes, scopes and hostnames,
// but never a configuration value, a token, an API key's secret or a configured URL.
type AdminAuditEvent struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt" gorm:"index"`
	// ActorUserID is the ID of the administrator who made the change, as the authenticator knows them (the decimal
	// user ID, or the bootstrap administrator's own ID), or AdminAuditActorSystem.
	ActorUserID string `json:"actorUserID" gorm:"index"`
	// Action is what was done, such as agent.create or vmcp.agent_oauth.authorize.
	Action string `json:"action"`
	// ResourceKind is one of the AdminAuditResource constants. ResourceID is its ID: an agent's, an API key's, a
	// vMCP's, a profile's, or a policy's name.
	ResourceKind string `json:"resourceKind" gorm:"index"`
	ResourceID   string `json:"resourceID" gorm:"index"`
	// ParentID is what holds the resource: the agent of an API key, or the vMCP of a profile.
	ParentID string         `json:"parentID,omitempty"`
	Details  datatypes.JSON `json:"details,omitempty"`
}

// ConvertAdminAuditEvent returns the API form of an event. actorName is the display name of the administrator, which is
// empty when it is not known; the system has none.
func ConvertAdminAuditEvent(event AdminAuditEvent, actorName string) types2.AdminAuditEvent {
	actor := types2.AuditLogActor{ActorType: types2.AuditLogActorTypeUser, ID: event.ActorUserID, DisplayName: actorName}
	if event.ActorUserID == AdminAuditActorSystem {
		actor = types2.AuditLogActor{ActorType: types2.AuditLogActorTypeSystem}
	}
	return types2.AdminAuditEvent{
		ID:           event.ID,
		CreatedAt:    *types2.NewTime(event.CreatedAt),
		Actor:        actor,
		Action:       event.Action,
		ResourceKind: event.ResourceKind,
		ResourceID:   event.ResourceID,
		ParentID:     event.ParentID,
		Details:      json.RawMessage(event.Details),
	}
}
