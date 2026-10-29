//nolint:revive
package types

import (
	"time"

	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	// ProvenanceSettingsID is the ID of the single settings row.
	ProvenanceSettingsID uint = 1
)

// The provenance rows persist the cross-surface session graph and the Obot
// Policy engine's records (pkg/provenance). Content never lands here: the
// payloads are the engine's content-free structs, serialized as JSON, with the
// fields a list view filters or sorts on lifted into indexed columns.

// ProvenanceSession is one agent session across every surface.
type ProvenanceSession struct {
	ID             string     `json:"id" gorm:"primaryKey"`
	UserID         string     `json:"userID" gorm:"index"`
	Email          string     `json:"email"`
	Agent          string     `json:"agent" gorm:"index"`
	Title          string     `json:"title"`
	StartedAt      time.Time  `json:"startedAt" gorm:"index"`
	LastActivityAt time.Time  `json:"lastActivityAt" gorm:"index"`
	EndedAt        *time.Time `json:"endedAt,omitempty"`
	Risk           int        `json:"risk" gorm:"index"`
	Killed         bool       `json:"killed" gorm:"index"`
	// LastSummary is the most recent event summary, lifted for text search.
	LastSummary string `json:"lastSummary"`
	// Labels and Surfaces are lifted from State for filtering.
	Labels   provenance.Labels    `json:"labels" gorm:"serializer:json"`
	Surfaces []provenance.Surface `json:"surfaces" gorm:"serializer:json"`
	Actor    provenance.Actor     `json:"actor" gorm:"serializer:json"`
	// State is the whole SessionState, fingerprints included. Fingerprints can
	// run to tens of thousands of entries, so they stay in this blob.
	State provenance.SessionState `json:"state" gorm:"serializer:json"`
}

// ProvenanceSessionLink maps a stitching key (a client session ID, a device
// and agent, an environment) to the session it points at.
type ProvenanceSessionLink struct {
	Key       string    `json:"key" gorm:"primaryKey"`
	SessionID string    `json:"sessionID" gorm:"index"`
	UpdatedAt time.Time `json:"updatedAt" gorm:"index"`
}

// ProvenanceEvent is one recorded event in a session. Payload is the full
// Event minus the action's content, which is never serialized. Seq orders
// events that share a timestamp in the order they were appended.
type ProvenanceEvent struct {
	Seq       uint      `json:"seq" gorm:"primaryKey;autoIncrement"`
	ID        string    `json:"id" gorm:"uniqueIndex;not null"`
	SessionID string    `json:"sessionID" gorm:"index"`
	UserID    string    `json:"userID" gorm:"index"`
	Surface   string    `json:"surface" gorm:"index"`
	Kind      string    `json:"kind" gorm:"index"`
	At        time.Time `json:"at" gorm:"index"`
	ParentID  string    `json:"parentID"`
	// Allowed is nil for events without a decision.
	Allowed  *bool            `json:"allowed,omitempty" gorm:"index"`
	Effect   string           `json:"effect" gorm:"index"`
	PolicyID string           `json:"policyID" gorm:"index"`
	Payload  provenance.Event `json:"payload" gorm:"serializer:json"`
}

// ProvenancePolicy is one Obot Policy rule.
type ProvenancePolicy struct {
	ID        string            `json:"id" gorm:"primaryKey"`
	Name      string            `json:"name"`
	Effect    string            `json:"effect" gorm:"index"`
	Mode      string            `json:"mode" gorm:"index"`
	Enabled   bool              `json:"enabled" gorm:"index"`
	BuiltIn   bool              `json:"builtIn"`
	Priority  int               `json:"priority" gorm:"index"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Payload   provenance.Policy `json:"payload" gorm:"serializer:json"`
}

// ProvenanceApproval is a request for a person to allow a held action.
type ProvenanceApproval struct {
	ID        string              `json:"id" gorm:"primaryKey"`
	Status    string              `json:"status" gorm:"index"`
	UserID    string              `json:"userID" gorm:"index"`
	SessionID string              `json:"sessionID" gorm:"index"`
	EventID   string              `json:"eventID"`
	PolicyID  string              `json:"policyID"`
	CreatedAt time.Time           `json:"createdAt" gorm:"index"`
	ExpiresAt time.Time           `json:"expiresAt"`
	Payload   provenance.Approval `json:"payload" gorm:"serializer:json"`
}

// ProvenanceKill is an active kill switch at some scope.
type ProvenanceKill struct {
	ID        string          `json:"id" gorm:"primaryKey"`
	Scope     string          `json:"scope" gorm:"index"`
	Target    string          `json:"target" gorm:"index"`
	CreatedAt time.Time       `json:"createdAt"`
	ExpiresAt *time.Time      `json:"expiresAt,omitempty"`
	Payload   provenance.Kill `json:"payload" gorm:"serializer:json"`
}

// ProvenanceHoneytoken is a planted canary. The value's hash lives in the
// payload: it is an unsigned 64-bit value, which a signed bigint column would
// not hold.
type ProvenanceHoneytoken struct {
	ID        string                `json:"id" gorm:"primaryKey"`
	Kind      string                `json:"kind" gorm:"index"`
	Name      string                `json:"name"`
	CreatedAt time.Time             `json:"createdAt"`
	Payload   provenance.Honeytoken `json:"payload" gorm:"serializer:json"`
}

// ProvenanceToolLabel is the capability labelling of one tool on one server.
type ProvenanceToolLabel struct {
	ID        uint              `json:"id" gorm:"primaryKey;autoIncrement"`
	Server    string            `json:"server" gorm:"uniqueIndex:idx_provenance_tool_labels_server_tool;not null"`
	Tool      string            `json:"tool" gorm:"uniqueIndex:idx_provenance_tool_labels_server_tool;not null"`
	Source    string            `json:"source" gorm:"index"`
	Rationale string            `json:"rationale"`
	UpdatedAt time.Time         `json:"updatedAt"`
	Labels    provenance.Labels `json:"labels" gorm:"serializer:json"`
}

// ProvenanceSettings is the single row (ID 1) of organization-wide settings.
type ProvenanceSettings struct {
	ID        uint                `json:"id" gorm:"primaryKey"`
	UpdatedAt time.Time           `json:"updatedAt"`
	Payload   provenance.Settings `json:"payload" gorm:"serializer:json"`
}
