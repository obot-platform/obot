package principal

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	KindUser  = types.PrincipalKindUser
	KindAgent = types.PrincipalKindAgent

	// agentPrefix starts the storage encoding of an agent. A user is stored as their bare decimal ID.
	agentPrefix = "agent:"
)

// Kind is the kind of identity a Principal is. It is the API's principal kind, so a Principal's kind is reported as is.
type Kind = types.PrincipalKind

// Principal is a user or an agent that calls Obot or owns something in it.
type Principal struct {
	Kind Kind
	// ID is the decimal ID of the user or of the agent.
	ID string
}

// User returns the principal of the user with id.
func User(id uint) Principal {
	return Principal{Kind: KindUser, ID: strconv.FormatUint(uint64(id), 10)}
}

// Agent returns the principal of the agent with id.
func Agent(id uint) Principal {
	return Principal{Kind: KindAgent, ID: strconv.FormatUint(uint64(id), 10)}
}

// Parse decodes the storage encoding of a principal: a bare decimal ID for a user, or "agent:<decimal ID>" for an
// agent. Any other string is an error. The ID of the result is in canonical decimal form, so Parse("007") is User(7).
func Parse(s string) (Principal, error) {
	kind, rawID := KindUser, s
	if rest, ok := strings.CutPrefix(s, agentPrefix); ok {
		kind, rawID = KindAgent, rest
	}
	id, err := strconv.ParseUint(rawID, 10, 0)
	if err != nil {
		return Principal{}, fmt.Errorf("invalid principal %q: want a decimal user ID or %s<decimal agent ID>", s, agentPrefix)
	}
	return Principal{Kind: kind, ID: strconv.FormatUint(id, 10)}, nil
}

// String returns the storage encoding of the principal, which Parse decodes. It is empty for the zero Principal.
func (p Principal) String() string {
	switch p.Kind {
	case KindUser:
		return p.ID
	case KindAgent:
		return agentPrefix + p.ID
	default:
		return ""
	}
}

// UserID returns the ID of the user the principal is. It reports false for agents, so code that needs a user must
// reject them explicitly.
func (p Principal) UserID() (uint, bool) {
	return p.idOf(KindUser)
}

// AgentID returns the ID of the agent the principal is. It reports false for users, so code that needs an agent must
// reject them explicitly.
func (p Principal) AgentID() (uint, bool) {
	return p.idOf(KindAgent)
}

// idOf returns the principal's ID if it is of kind and the ID is a decimal number.
func (p Principal) idOf(kind Kind) (uint, bool) {
	if p.Kind != kind {
		return 0, false
	}
	id, err := strconv.ParseUint(p.ID, 10, 0)
	if err != nil {
		return 0, false
	}
	return uint(id), true
}

// Ref returns the API representation of the principal.
func (p Principal) Ref() types.PrincipalRef {
	return types.PrincipalRef{Kind: p.Kind, ID: p.ID}
}

// ClaimsAgent reports whether a caller presents itself as an agent in any way: its UID has the agent encoding's
// prefix, or it records the agent kind. Unlike FromUserInfo, it is also true for a malformed principal whose UID and
// recorded kind disagree, so that code confining agents to what they may do confines such a principal too, rather
// than treating it as a user. Use FromUserInfo to learn which agent a caller is.
func ClaimsAgent(u kuser.Info) bool {
	if u == nil {
		return false
	}
	return ClaimsAgentOwner(u.GetUID()) || slices.Contains(u.GetExtra()[KindExtra], string(KindAgent))
}

// ClaimsAgentOwner reports whether s, a principal's storage encoding (Principal.String) or a token's subject, presents
// itself as an agent's: it has the agent encoding's prefix, even if what follows is not a decimal ID. It is the
// lenient form of ParseAgent, for code that must refuse a malformed agent like any other agent.
func ClaimsAgentOwner(s string) bool {
	return strings.HasPrefix(s, agentPrefix)
}

// IsComponentHop reports whether u is the composite loopback reaching a component of a vMCP (types.GroupCompositeMCP),
// which acts for the caller of the vMCP.
func IsComponentHop(u kuser.Info) bool {
	return u != nil && slices.Contains(u.GetGroups(), types.GroupCompositeMCP)
}

// IsAgent reports whether u is a well-formed agent principal (FromUserInfo). Code that confines callers that claim to
// be agents, malformed ones included, uses ClaimsAgent instead.
func IsAgent(u kuser.Info) bool {
	p, ok := FromUserInfo(u)
	return ok && p.Kind == KindAgent
}

// ParseAgent returns the decimal ID of the agent that owner, a principal's storage encoding (Principal.String), names.
// It reports false for any other owner: a user's, and one that is not a well-formed principal.
func ParseAgent(owner string) (string, bool) {
	p, err := Parse(owner)
	if err != nil || p.Kind != KindAgent {
		return "", false
	}
	return p.ID, true
}

// IsAgentKind reports whether kind, a principal kind stored as a string, such as the principal_kind column of an
// activity row, is the agent kind.
func IsAgentKind(kind string) bool {
	return Kind(kind) == KindAgent
}

// FromUserInfo returns the principal an authenticated caller acts for. Only the authenticators of users and agents
// record a kind (RecordKind), so a caller without one, such as a system principal, is not a principal. It also
// reports false when the recorded kind does not match the encoding of the caller's UID: a decimal UID marked as an
// agent, or an agent UID marked as a user, is rejected rather than read as some other identity.
func FromUserInfo(u kuser.Info) (Principal, bool) {
	if u == nil {
		return Principal{}, false
	}
	marker := u.GetExtra()[KindExtra]
	if len(marker) != 1 {
		return Principal{}, false
	}
	p, err := Parse(u.GetUID())
	if err != nil || Kind(marker[0]) != p.Kind {
		return Principal{}, false
	}
	return p, true
}
