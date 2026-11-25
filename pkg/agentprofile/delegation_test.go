package agentprofile

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// actingFor returns the principal of agent 3 acting on behalf of user 7, as the authenticator of delegated tokens
// records it.
func actingFor(t *testing.T) *kuser.DefaultInfo {
	t.Helper()

	extra := map[string][]string{}
	principal.RecordPrincipal(extra, principal.KindAgent, types.UserStatusActive)
	principal.RecordOnBehalfOf(extra, 7)
	require.NoError(t, RecordDelegation(extra, Delegation{
		ID: "adl1test",
		User: subjects.Caller{
			Principal: principal.User(7),
			Groups: subjects.Groups{
				AuthProvider: map[string]struct{}{"eng": {}},
				Obot:         map[string]struct{}{types.GroupBasic: {}},
			},
		},
		Email:       "ann@example.com",
		DisplayName: "Ann",
	}))
	uid := principal.Agent(3).String()
	return &kuser.DefaultInfo{Name: uid, UID: uid, Extra: extra}
}

func agentPrincipal() *kuser.DefaultInfo {
	extra := map[string][]string{}
	principal.RecordPrincipal(extra, principal.KindAgent, types.UserStatusActive)
	uid := principal.Agent(3).String()
	return &kuser.DefaultInfo{Name: uid, UID: uid, Extra: extra}
}

func TestDelegationOfRoundTrips(t *testing.T) {
	d, ok := DelegationOf(actingFor(t))
	require.True(t, ok)
	require.Equal(t, "adl1test", d.ID)
	require.Equal(t, principal.User(7), d.User.Principal)
	require.Equal(t, map[string]struct{}{"eng": {}}, d.User.Groups.AuthProvider)
	require.Equal(t, map[string]struct{}{types.GroupBasic: {}}, d.User.Groups.Obot)
	require.Equal(t, "ann@example.com", d.Email)
	require.Equal(t, "Ann", d.DisplayName)
}

func TestDelegationOfFailsClosed(t *testing.T) {
	_, ok := DelegationOf(agentPrincipal())
	require.False(t, ok, "an agent acting for itself has no delegation")

	// A user that carries the record acts for no one.
	user := actingFor(t)
	user.UID = "7"
	principal.RecordPrincipal(user.Extra, principal.KindUser, types.UserStatusActive)
	_, ok = DelegationOf(user)
	require.False(t, ok, "a user's principal has no delegation")

	for _, tt := range []struct {
		name   string
		record []string
	}{
		{
			name: "no record",
		},
		{
			name:   "a malformed record",
			record: []string{"{"},
		},
		{
			name:   "a record of another user",
			record: []string{`{"id":"adl1test","userID":"8"}`},
		},
		{
			name:   "two records",
			record: []string{`{"id":"adl1test","userID":"7"}`, `{"id":"adl1test","userID":"7"}`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := actingFor(t)
			if tt.record == nil {
				delete(info.Extra, principal.DelegationExtra)
			} else {
				info.Extra[principal.DelegationExtra] = tt.record
			}
			d, ok := DelegationOf(info)
			require.True(t, ok, "the agent still acts on behalf of the user")
			require.Equal(t, principal.User(7), d.User.Principal)
			// Without an ID, the authorizer refuses the caller everything.
			require.Empty(t, d.ID)
			require.Empty(t, d.User.Groups.AuthProvider)
			require.Empty(t, d.Email)
		})
	}
}
