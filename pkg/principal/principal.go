// Package principal interprets who an authenticated caller is.
//
// Obot authenticates people and hosted agents. They are not interchangeable: an
// agent authenticates as itself so that a compromised sandbox reaches only what
// that one instance was configured with, which means its identity is an
// instance rather than a row in the users table. Code that needs "the user who
// owns this" therefore cannot use the caller's ID directly.
package principal

import (
	"slices"

	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// HostedAgentOwnerExtra carries the user a hosted agent was created by. It is
// set on the principal at authentication time.
const HostedAgentOwnerExtra = "hosted_agent_owner_id"

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
func IsHostedAgent(user kuser.Info) bool {
	return user != nil && slices.Contains(user.GetGroups(), types.GroupHostedAgent)
}
