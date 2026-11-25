package types

// PrincipalKind is the kind of identity that acts in or owns something in Obot.
type PrincipalKind string

const (
	PrincipalKindUser  PrincipalKind = "user"
	PrincipalKindAgent PrincipalKind = "agent"
)

// PrincipalRef identifies a user or an agent in API responses and audit records.
type PrincipalRef struct {
	Kind PrincipalKind `json:"kind"`
	// ID is the decimal ID of the user or of the agent.
	ID string `json:"id"`
}
