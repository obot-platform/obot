// Package principal interprets who an authenticated caller is.
//
// Obot authenticates people and hosted agents. They are not interchangeable: an
// agent authenticates as itself so that a compromised sandbox reaches only what
// that one instance was configured with, which means its identity is an
// instance rather than a row in the users table. Code that needs "the user who
// owns this" therefore cannot use the caller's ID directly.
package principal

import (
	"fmt"
	"strconv"

	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// HostedAgentOwnerExtra carries the user a hosted agent was created by. It is
// set on the principal at authentication time.
const HostedAgentOwnerExtra = "hosted_agent_owner_id"

// AuthorizedModelIDsExtra carries the Model resource names a hosted agent was
// configured with, or "*" for every model. It is the agent's whole authority
// over models: an agent is not re-evaluated against access policies, which
// describe people.
const AuthorizedModelIDsExtra = "authorized_model_ids"

const (
	// APIKeyIDExtra and APIKeyNameExtra carry the non-secret credential
	// attribution established by the API-key authenticator. Downstream code
	// must use APIKeyAttributionFromUser instead of interpreting these values.
	APIKeyIDExtra   = "api_key_id"
	APIKeyNameExtra = "api_key_name"
)

// APIKeyAttribution identifies the API key that authenticated a request.
type APIKeyAttribution struct {
	ID   uint
	Name string
}

// NewAPIKeyAttribution creates event-time display attribution for an API key.
// Unnamed keys use the reconstructable, non-secret masked key identifier so
// audit consumers never need to join back to the API-key table to label them.
func NewAPIKeyAttribution(id, ownerUserID uint, name string) APIKeyAttribution {
	if name == "" {
		name = fmt.Sprintf("ok1-%d-%d-*****", ownerUserID, id)
	}
	return APIKeyAttribution{ID: id, Name: name}
}

// APIKeyAttributionFromUser returns the API key that authenticated a caller.
// A missing or malformed ID means the caller did not authenticate with an API
// key. The authenticator supplies a display name, including the masked fallback
// for an unnamed key; an empty name indicates a legacy or malformed principal.
func APIKeyAttributionFromUser(user kuser.Info) (APIKeyAttribution, bool) {
	if user == nil {
		return APIKeyAttribution{}, false
	}
	extra := user.GetExtra()
	ids := extra[APIKeyIDExtra]
	if len(ids) == 0 {
		return APIKeyAttribution{}, false
	}
	id, err := strconv.ParseUint(ids[0], 10, 0)
	if err != nil || id == 0 {
		return APIKeyAttribution{}, false
	}
	attribution := APIKeyAttribution{ID: uint(id)}
	if names := extra[APIKeyNameExtra]; len(names) > 0 {
		attribution.Name = names[0]
	}
	return attribution, true
}

// AuthorizedModelIDs returns the models a hosted agent may use, and whether the
// caller is an agent at all.
//
// Every caller that carries the extra is an agent, but the reverse does not
// hold: an agent configured with no models has none, which is an empty list and
// not the absence of one. Callers must distinguish those, because an agent
// authorized for nothing may use nothing, while a person is governed by policy
// instead.
func AuthorizedModelIDs(user kuser.Info) ([]string, bool) {
	if user == nil {
		return nil, false
	}
	ids, ok := user.GetExtra()[AuthorizedModelIDsExtra]
	return ids, ok || IsHostedAgent(user)
}

// ResourceOwnerID returns the user that owns what a caller creates, and that
// usage is attributed to.
//
// For a person this is their own ID. For a hosted agent it is the user who
// created it, because the resources an agent touches belong to that person and
// the usage it incurs is theirs. Using the agent's own ID here produces records
// pointing at a user that does not exist, which fails lookups and, in a
// controller, requeues forever.
//
// This is distinct from authorization, which uses the caller's real identity:
// an agent may only reach what its own configuration allows, regardless of what
// its owner can reach.
func ResourceOwnerID(user kuser.Info) string {
	if user == nil {
		return ""
	}
	if owner := user.GetExtra()[HostedAgentOwnerExtra]; len(owner) > 0 && owner[0] != "" {
		return owner[0]
	}
	return user.GetUID()
}

// IsHostedAgent reports whether a caller is a sandbox rather than a person.
//
// The owner extra is the marker rather than a group of its own. A sandbox's
// groups say what it may reach -- models, MCP servers, skills -- and those are
// the same groups a person can hold, so they cannot answer what it is. Only a
// sandbox is issued on behalf of someone else, which is exactly what this extra
// records, and ResourceOwnerID already relies on it being set.
func IsHostedAgent(user kuser.Info) bool {
	if user == nil {
		return false
	}
	owner := user.GetExtra()[HostedAgentOwnerExtra]
	return len(owner) > 0 && owner[0] != ""
}
