package principal

import (
	"strconv"
	"strings"

	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// OnBehalfOfExtra carries the ID of the user that an agent caller acts for, as its authenticator determined it.
	// Code must use RecordOnBehalfOf and OnBehalfOf instead of this key.
	OnBehalfOfExtra = "obot_on_behalf_of"
	// DelegationExtra carries what the authenticator of an on-behalf-of caller established about the user: the
	// delegation, and the user's groups and identity (pkg/agentprofile.RecordDelegation). It is stripped with
	// OnBehalfOfExtra, so that no provider can supply it. Code must use pkg/agentprofile instead of this key.
	DelegationExtra = "obot_delegation"

	// delegatedOwnerUserPart separates the agent and the user in a DelegatedOwner's encoding.
	delegatedOwnerUserPart = "/user:"
)

// DelegatedOwner is an agent that acts for a user, as the owner of what the user sets up through the agent, such as
// instances, credentials and OAuth tokens (DelegatedOwnerOf returns it for a caller). It keeps that setup apart from the user's own (owned by "<userID>") and from the agent's shared setup (owned
// by "agent:<agentID>"). Its storage encoding (String) is "agent:<agentID>/user:<userID>".
//
// It is an owner, never a principal. Parse refuses its encoding, so FromUserInfo never reads a caller with it as a user
// or an agent, no authenticator builds one, the admission check refuses any caller that carries it
// (IsDelegatedOwner), and no token is issued for it.
type DelegatedOwner struct {
	AgentID uint
	UserID  uint
}

// RecordOnBehalfOf records that a principal acts for the user with userID. Like RecordKind, it always replaces any
// value already in extra. Only an agent acts for a user: OnBehalfOf reports nothing for any other kind of caller, and
// FromUserInfo refuses one that carries it. RecordKind removes it, so an authenticator records it after the kind.
func RecordOnBehalfOf(extra map[string][]string, userID uint) {
	extra[OnBehalfOfExtra] = []string{strconv.FormatUint(uint64(userID), 10)}
}

// OnBehalfOf returns the decimal ID of the user that a caller acts for (RecordOnBehalfOf). It reports false for a
// caller that acts for no one, and for any caller that is not a well-formed agent principal (FromUserInfo).
func OnBehalfOf(u kuser.Info) (string, bool) {
	owner, ok := DelegatedOwnerOf(u)
	if !ok {
		return "", false
	}
	return strconv.FormatUint(uint64(owner.UserID), 10), true
}

// HasInvalidOnBehalfOf reports whether u carries an on-behalf-of user that OnBehalfOf does not accept: one on a caller
// that is not a well-formed agent, or one that is not a single user ID. No authenticator builds such a caller, and the
// admission check refuses one rather than let it act as itself.
func HasInvalidOnBehalfOf(u kuser.Info) bool {
	if u == nil {
		return false
	}
	if _, present := u.GetExtra()[OnBehalfOfExtra]; !present {
		return false
	}
	_, ok := OnBehalfOf(u)
	return !ok
}

// onBehalfOfUser returns the user that the recorded values of OnBehalfOfExtra name. It reports false unless they are a
// single user ID.
func onBehalfOfUser(values []string) (uint, bool) {
	if len(values) != 1 {
		return 0, false
	}
	p, err := Parse(values[0])
	if err != nil {
		return 0, false
	}
	return p.UserID()
}

// DelegatedOwnerOf returns the agent that a caller is and the user it acts for, which own what the user sets up
// through the agent. It reports false for a caller that acts for no one, and for any caller that is not a well-formed
// agent principal (FromUserInfo).
func DelegatedOwnerOf(u kuser.Info) (DelegatedOwner, bool) {
	// A caller that is not a well-formed principal is the zero Principal, which is no agent.
	p, _ := FromUserInfo(u)
	agentID, ok := p.AgentID()
	if !ok {
		return DelegatedOwner{}, false
	}
	userID, ok := onBehalfOfUser(u.GetExtra()[OnBehalfOfExtra])
	if !ok {
		return DelegatedOwner{}, false
	}
	return DelegatedOwner{AgentID: agentID, UserID: userID}, true
}

// String returns the storage encoding of the owner, which ParseDelegatedOwner decodes.
func (o DelegatedOwner) String() string {
	return agentPrefix + strconv.FormatUint(uint64(o.AgentID), 10) + delegatedOwnerUserPart + strconv.FormatUint(uint64(o.UserID), 10)
}

// ParseDelegatedOwner decodes the storage encoding of a DelegatedOwner (DelegatedOwner.String). It reports false for
// anything else, including an encoding with a zero ID or one that is not in canonical form, so that an owner has only
// one encoding.
func ParseDelegatedOwner(s string) (DelegatedOwner, bool) {
	rest, ok := strings.CutPrefix(s, agentPrefix)
	if !ok {
		return DelegatedOwner{}, false
	}
	agentPart, userPart, ok := strings.Cut(rest, delegatedOwnerUserPart)
	if !ok {
		return DelegatedOwner{}, false
	}
	agentID, err := strconv.ParseUint(agentPart, 10, 0)
	if err != nil || agentID == 0 {
		return DelegatedOwner{}, false
	}
	userID, err := strconv.ParseUint(userPart, 10, 0)
	if err != nil || userID == 0 {
		return DelegatedOwner{}, false
	}
	owner := DelegatedOwner{AgentID: uint(agentID), UserID: uint(userID)}
	if owner.String() != s {
		return DelegatedOwner{}, false
	}
	return owner, true
}

// IsDelegatedOwner reports whether owner has the shape of a DelegatedOwner's encoding, an agent's prefix followed by
// more than an ID, whether or not it is well formed. Code that must never act as such an owner, such as
// authentication, refuses any such owner.
func IsDelegatedOwner(owner string) bool {
	rest, ok := strings.CutPrefix(owner, agentPrefix)
	return ok && strings.Contains(rest, "/")
}

// OwnedByCaller reports whether what owner owns, such as an instance, a component server or connection, or an upstream
// OAuth token, is the caller u's to use: owner is u's own encoded owner (its UID), or, for an agent acting on behalf of
// a user, the DelegatedOwner of that agent and that user, which owns what the user set up through the agent. An agent
// acting for one user never uses what it was set up with for another, or what the user set up for themselves, and an
// agent acting for itself, a user, or anyone else never uses what any user set up through an agent.
func OwnedByCaller(u kuser.Info, owner string) bool {
	if u == nil || owner == "" {
		return false
	}
	if owner == u.GetUID() {
		return !IsDelegatedOwner(owner)
	}
	delegated, ok := DelegatedOwnerOf(u)
	return ok && delegated.String() == owner
}
