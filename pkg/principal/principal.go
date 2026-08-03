// Package principal interprets who an authenticated caller is.
//
// Obot authenticates people and hosted agents. They are not interchangeable: an
// agent authenticates as itself so that a compromised sandbox reaches only what
// that one instance was configured with, which means its identity is an
// instance rather than a row in the users table. Code that needs "the user who
// owns this" therefore cannot use the caller's ID directly.
package principal

import (
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
