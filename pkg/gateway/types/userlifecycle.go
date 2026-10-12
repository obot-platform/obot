package types

import (
	"time"
)

const (
	// UserLifecycleEventDisabled removes what a disabled user must not keep: their MCP OAuth refresh tokens.
	UserLifecycleEventDisabled UserLifecycleEventType = "disabled"
	// UserLifecycleEventReconcile recomputes what depends on a user's roles and, when the user left a group,
	// the resources that group granted.
	UserLifecycleEventReconcile UserLifecycleEventType = "reconcile"
)

// UserLifecycleEventType is the kind of work a UserLifecycleEvent records.
type UserLifecycleEventType string

// UserLifecycleEvent is an outbox entry for work that must follow a committed change to a user, in a store the
// gateway database cannot share a transaction with. It is written in the same transaction as the change, and
// delivered after the commit until delivery succeeds.
type UserLifecycleEvent struct {
	// ID orders the events. idx_user_lifecycle_events_pending serves delivery, which reads the pending events in order.
	ID        uint                   `json:"id" gorm:"primaryKey;index:idx_user_lifecycle_events_pending,where:delivered_at IS NULL"`
	CreatedAt time.Time              `json:"createdAt"`
	UserID    uint                   `json:"userID" gorm:"index"`
	Type      UserLifecycleEventType `json:"type"`
	// GroupsRemoved is set on a reconcile event when the user left at least one group.
	GroupsRemoved bool `json:"groupsRemoved" gorm:"not null;default:false"`

	// ClaimedUntil is when a replica's claim to deliver the event expires, so that replicas rarely deliver the
	// same event at once. Delivery is idempotent, so an expired claim is safe to take over.
	ClaimedUntil *time.Time `json:"claimedUntil,omitempty"`
	DeliveredAt  *time.Time `json:"deliveredAt,omitempty" gorm:"index"`
	Attempts     int        `json:"attempts" gorm:"not null;default:0"`
	LastError    string     `json:"lastError,omitempty"`
}
