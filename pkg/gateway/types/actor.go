package types

import (
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// Actor says who an activity row is attributed to: the principal that made the request, and the legacy user ID
// column that existing queries, exports and dashboards read.
//
// Rows are attributed with two columns, PrincipalKind and PrincipalID, rather than ActorKind and ActorID: the MCP
// audit table already has an actor_id column for the actor of a local-agent tool call, who may be a device, and the
// two are different things. PrincipalKind and PrincipalID are the identity that called Obot.
//
// A user's row, including one written before agents existed, has PrincipalKind "user" (the column's default) and no
// PrincipalID, and is read as the user in UserID; see Ref.
type Actor struct {
	// PrincipalKind is "user" or "agent". It is empty for a caller that is neither, such as a system principal; the
	// column's default then reads it as a user, with no ID.
	PrincipalKind string
	// PrincipalID is the decimal ID of the agent, and empty for a user, who is in UserID.
	PrincipalID string
	// UserID is the user's ID for a user, and empty for an agent, so that nothing that reads a row's user mistakes
	// an agent for one. A caller that is not a principal keeps its UID here, as it was always recorded.
	UserID string
	// OnBehalfOf is the decimal ID of the user an agent acted for (principal.OnBehalfOf), and empty for any other
	// request. The agent's row keeps UserID empty even then, so that nothing that reads a user's rows, such as the
	// user's own budgets and daily token limits, counts the agent's work for them. Readers that include it ask to.
	OnBehalfOf string
}

// ActorForCaller returns the attribution of the request of an authenticated caller. It uses the kind its authenticator
// recorded, and never reads one from the shape of the UID. An agent that acts for a user is attributed to the agent,
// with the user as OnBehalfOf.
func ActorForCaller(caller kuser.Info) Actor {
	if p, ok := principal.FromUserInfo(caller); ok {
		actor := actorOf(p)
		actor.OnBehalfOf, _ = principal.OnBehalfOf(caller)
		return actor
	}
	if caller == nil {
		return Actor{}
	}
	// Not a user or an agent, or malformed: keep what was always recorded.
	return Actor{UserID: caller.GetUID()}
}

// ActorForOwner returns the attribution of an owner in its storage encoding (Principal.String), for the writers that
// are handed the encoding rather than the caller, such as audit entries sent by an MCP server. An owner that is not an
// encoded principal keeps being recorded as the user ID it always was.
func ActorForOwner(owner string) Actor {
	if p, err := principal.Parse(owner); err == nil {
		return actorOf(p)
	}
	return Actor{UserID: owner}
}

func actorOf(p principal.Principal) Actor {
	if p.Kind == principal.KindAgent {
		return Actor{PrincipalKind: string(p.Kind), PrincipalID: p.ID}
	}
	return Actor{PrincipalKind: string(p.Kind), UserID: p.ID}
}

// Ref returns the typed principal of a row's actor: its agent, or otherwise the user in its UserID. The result is the
// zero PrincipalRef when the row has no principal at all.
func (a Actor) Ref() apitypes.PrincipalRef {
	if principal.IsAgentKind(a.PrincipalKind) {
		return apitypes.PrincipalRef{Kind: apitypes.PrincipalKindAgent, ID: a.PrincipalID}
	}
	if a.UserID == "" {
		return apitypes.PrincipalRef{}
	}
	return apitypes.PrincipalRef{Kind: apitypes.PrincipalKindUser, ID: a.UserID}
}

// OnBehalfOfRef returns the typed principal of the user a row's agent acted for, and the zero PrincipalRef when it acted
// for no one.
func (a Actor) OnBehalfOfRef() apitypes.PrincipalRef {
	if a.OnBehalfOf == "" {
		return apitypes.PrincipalRef{}
	}
	return apitypes.PrincipalRef{Kind: apitypes.PrincipalKindUser, ID: a.OnBehalfOf}
}

// IsAgent reports whether a row's actor is an agent that it names.
func (a Actor) IsAgent() bool {
	return principal.IsAgentKind(a.PrincipalKind) && a.PrincipalID != ""
}

// Owner returns the storage encoding (Principal.String) of a row's actor: "agent:<id>" for an agent, and otherwise its
// UserID. It is the inverse of ActorForOwner.
func (a Actor) Owner() string {
	if a.IsAgent() {
		return principal.Principal{Kind: principal.KindAgent, ID: a.PrincipalID}.String()
	}
	return a.UserID
}
