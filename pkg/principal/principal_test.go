package principal

import (
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// A hosted agent's ID identifies an instance, not a row in the users table.
// Using it as an owner produces records pointing at a user that does not exist,
// which fails lookups and requeues forever in a controller.
func TestResourceOwnerIDResolvesAgentToItsOwner(t *testing.T) {
	agent := &kuser.DefaultInfo{
		Name:  "hosted-agent:hai1abc",
		UID:   "hosted-agent:hai1abc",
		Extra: map[string][]string{HostedAgentOwnerExtra: {"3"}},
	}

	if got := ResourceOwnerID(agent); got != "3" {
		t.Errorf("ResourceOwnerID = %q, want the owner 3", got)
	}
}

func TestResourceOwnerIDLeavesPeopleUnchanged(t *testing.T) {
	person := &kuser.DefaultInfo{Name: "someone@example.com", UID: "7"}

	if got := ResourceOwnerID(person); got != "7" {
		t.Errorf("ResourceOwnerID = %q, want 7", got)
	}
	if got := ResourceOwnerID(nil); got != "" {
		t.Errorf("ResourceOwnerID(nil) = %q, want empty", got)
	}
}

// An agent whose owner is somehow unrecorded must not silently resolve to
// something that looks like a different user.
func TestResourceOwnerIDFallsBackToTheCallerWhenOwnerMissing(t *testing.T) {
	agent := &kuser.DefaultInfo{
		UID:   "hosted-agent:hai1abc",
		Extra: map[string][]string{HostedAgentOwnerExtra: {""}},
	}

	if got := ResourceOwnerID(agent); got != "hosted-agent:hai1abc" {
		t.Errorf("ResourceOwnerID = %q, want the caller ID unchanged", got)
	}
}

func TestIsHostedAgent(t *testing.T) {
	agent := &kuser.DefaultInfo{
		Groups: []string{types.GroupAuthenticated, types.GroupMCP, types.GroupLLM},
		Extra:  map[string][]string{HostedAgentOwnerExtra: {"u1"}},
	}
	if !IsHostedAgent(agent) {
		t.Error("expected an agent principal to be recognised")
	}

	// The groups a sandbox carries are ordinary access groups a person can hold
	// too, so they must not be what identifies one.
	lookalike := &kuser.DefaultInfo{Groups: []string{types.GroupAuthenticated, types.GroupMCP, types.GroupLLM}}
	if IsHostedAgent(lookalike) {
		t.Error("a person holding the same access groups must not read as an agent")
	}

	// An empty owner is not an owner.
	blank := &kuser.DefaultInfo{Extra: map[string][]string{HostedAgentOwnerExtra: {""}}}
	if IsHostedAgent(blank) {
		t.Error("an empty owner must not read as an agent")
	}

	person := &kuser.DefaultInfo{Groups: []string{types.GroupAuthenticated, types.GroupAPI}}
	if IsHostedAgent(person) {
		t.Error("a user principal must not be mistaken for an agent")
	}
	if IsHostedAgent(nil) {
		t.Error("a nil principal is not an agent")
	}
}
