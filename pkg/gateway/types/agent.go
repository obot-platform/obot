package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"gorm.io/gorm"
)

const (
	// AgentLifecycleEventDisabled records that an agent lost access.
	AgentLifecycleEventDisabled AgentLifecycleEventType = "disabled"
	// AgentLifecycleEventEnabled records that a disabled agent regained access.
	AgentLifecycleEventEnabled AgentLifecycleEventType = "enabled"
	// AgentLifecycleEventDeleted records that an agent was deleted, so that what it owns is removed.
	AgentLifecycleEventDeleted AgentLifecycleEventType = "deleted"
)

var (
	// AgentLifecycleColumns are the agent columns that only lifecycle operations write. Every other agent write
	// omits them, so that a stale copy of an agent can neither disable, re-enable, nor restore it.
	AgentLifecycleColumns = []string{"disabled_at", "disabled_reason", "deleted_at"}
)

// Agent is a non-human identity that calls Obot with its own credentials. Its ID is its identity: display names
// need not be unique. An agent has no role, groups, or login.
type Agent struct {
	ID          uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	DisplayName string `json:"displayName" gorm:"not null"`
	Description string `json:"description"`
	// OwnerUserIDs are the agent's administrative contacts. They grant nothing, and changing or deleting an owner
	// has no effect on the agent.
	OwnerUserIDs  []uint    `json:"ownerUserIDs" gorm:"serializer:json"`
	CreatorUserID uint      `json:"creatorUserID"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`

	// Lifecycle fields. A disabled agent keeps its keys and assignments but is denied access. A deleted agent's row
	// is kept so that audit history can still show it. Only lifecycle operations write these fields; see
	// AgentLifecycleColumns.
	DisabledAt     *time.Time     `json:"disabledAt,omitempty"`
	DisabledReason string         `json:"disabledReason,omitempty" gorm:"not null;default:''"`
	DeletedAt      gorm.DeletedAt `json:"deletedAt,omitzero" gorm:"index"`
}

// AgentLifecycleEventType is the kind of change an AgentLifecycleEvent records.
type AgentLifecycleEventType string

// AgentLifecycleEvent is an outbox entry for work that must follow a committed change to an agent's lifecycle, in a
// store the gateway database cannot share a transaction with. It is written in the same transaction as the change,
// and delivered after the commit until delivery succeeds, as UserLifecycleEvent is.
type AgentLifecycleEvent struct {
	// ID orders the events. idx_agent_lifecycle_events_pending serves delivery, which reads the pending events in
	// order.
	ID        uint                    `json:"id" gorm:"primaryKey;index:idx_agent_lifecycle_events_pending,where:delivered_at IS NULL"`
	CreatedAt time.Time               `json:"createdAt"`
	AgentID   uint                    `json:"agentID" gorm:"index"`
	Type      AgentLifecycleEventType `json:"type"`

	// ClaimedUntil is when a replica's claim to deliver the event expires, so that replicas rarely deliver the
	// same event at once. Delivery is idempotent, so an expired claim is safe to take over.
	ClaimedUntil *time.Time `json:"claimedUntil,omitempty"`
	DeliveredAt  *time.Time `json:"deliveredAt,omitempty" gorm:"index"`
	Attempts     int        `json:"attempts" gorm:"not null;default:0"`
	LastError    string     `json:"lastError,omitempty"`
}

// Status returns the agent's lifecycle status. Agents share the lifecycle statuses of users.
func (a Agent) Status() types2.UserStatus {
	switch {
	case a.DeletedAt.Valid:
		return types2.UserStatusDeleted
	case a.DisabledAt != nil:
		return types2.UserStatusDisabled
	default:
		return types2.UserStatusActive
	}
}
