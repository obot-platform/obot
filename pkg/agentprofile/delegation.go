package agentprofile

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// Delegation is what authenticating a token with which an agent acts on behalf of a user established about that user.
// The authenticator computes it once per request, after it checks that a profile of the agent matches the user, and
// records it on the agent's principal (RecordDelegation); everything else reads it from there (DelegationOf). It
// grants the agent nothing: the agent acting for the user has its own grants.
type Delegation struct {
	// ID is the delegation the token was minted from.
	ID string
	// User is the user the agent acts for, with the groups they are in now. The groups are for matching subjects only,
	// such as which guardrails cover the user. They are never the caller's groups, so no rule that grants a user's
	// group anything ever grants it to the agent.
	User subjects.Caller
	// Email and DisplayName are the user's, for the identity Obot passes on to MCP servers.
	Email       string
	DisplayName string
}

// recordedDelegation is the encoding of a Delegation in principal.DelegationExtra.
type recordedDelegation struct {
	ID                 string   `json:"id"`
	UserID             string   `json:"userID"`
	AuthProviderGroups []string `json:"authProviderGroups,omitempty"`
	ObotGroups         []string `json:"obotGroups,omitempty"`
	Email              string   `json:"email,omitempty"`
	DisplayName        string   `json:"displayName,omitempty"`
}

// RecordDelegation records d on the principal of the agent that acts for d's user, whose extra is extra. Only the
// authenticator of tokens with which an agent acts on behalf of a user calls it, after principal.RecordOnBehalfOf.
// Like RecordOnBehalfOf, it replaces whatever extra holds.
func RecordDelegation(extra map[string][]string, d Delegation) error {
	data, err := json.Marshal(recordedDelegation{
		ID:                 d.ID,
		UserID:             d.User.Principal.ID,
		AuthProviderGroups: slices.Sorted(maps.Keys(d.User.Groups.AuthProvider)),
		ObotGroups:         slices.Sorted(maps.Keys(d.User.Groups.Obot)),
		Email:              d.Email,
		DisplayName:        d.DisplayName,
	})
	if err != nil {
		return err
	}
	extra[principal.DelegationExtra] = []string{string(data)}
	return nil
}

// DelegationOf returns what authentication established about the user that u, an agent's principal, acts for. It
// reports false for a caller that acts for no one (principal.OnBehalfOf).
//
// For a caller that acts for a user without a well-formed record of the same user, it returns only the user, with no
// ID and no groups. The authenticator always records one, so such a caller was not built by it, and the authorizer
// refuses it everything (pkg/api/authz).
func DelegationOf(u kuser.Info) (Delegation, bool) {
	userID, ok := principal.OnBehalfOf(u)
	if !ok {
		return Delegation{}, false
	}
	d := Delegation{User: subjects.Caller{Principal: principal.Principal{Kind: principal.KindUser, ID: userID}}}

	values := u.GetExtra()[principal.DelegationExtra]
	if len(values) != 1 {
		return d, true
	}
	var record recordedDelegation
	if err := json.Unmarshal([]byte(values[0]), &record); err != nil || record.UserID != userID {
		return d, true
	}
	d.ID = record.ID
	d.Email = record.Email
	d.DisplayName = record.DisplayName
	d.User.Groups = subjects.Groups{
		AuthProvider: setOf(record.AuthProviderGroups),
		Obot:         setOf(record.ObotGroups),
	}
	return d, true
}

func setOf(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
