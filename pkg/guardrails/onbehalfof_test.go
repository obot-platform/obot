package guardrails

import (
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// TestGuardrailFiltersAccumulateOnBehalfOfAUser checks that a guardrail filter applies to an agent acting on behalf of a
// user when its subjects name the agent or the user, by ID, group or everyone, and that one for some other user, group
// or agent does not.
func TestGuardrailFiltersAccumulateOnBehalfOfAUser(t *testing.T) {
	info := &kuser.DefaultInfo{UID: "agent:5", Name: "agent:5", Extra: map[string][]string{}}
	principal.RecordPrincipal(info.Extra, principal.KindAgent, apitypes.UserStatusActive)
	principal.RecordOnBehalfOf(info.Extra, 7)
	require.NoError(t, agentprofile.RecordDelegation(info.Extra, agentprofile.Delegation{
		ID: "adl1test",
		User: subjects.Caller{
			Principal: principal.User(7),
			Groups:    subjects.Groups{AuthProvider: map[string]struct{}{"eng": {}}},
		},
	}))

	identity, admin, err := ResolveIdentity(t.Context(), nil, info)
	require.NoError(t, err)
	require.False(t, admin)
	require.Equal(t, "agent:5", identity.UserID)
	require.Equal(t, principal.Agent(5), identity.Principal)
	require.Empty(t, identity.Groups, "the user's groups are never the agent's")
	require.Equal(t, principal.User(7), identity.OnBehalfOf.Principal)

	subject := func(typ, id string) []apitypes.MCPGuardrailFilterSubject {
		return []apitypes.MCPGuardrailFilterSubject{{Type: typ, ID: id}}
	}
	for _, tt := range []struct {
		name     string
		subjects []apitypes.MCPGuardrailFilterSubject
		want     bool
	}{
		{
			name: "no subjects is everyone, which covers the user",
			want: true,
		},
		{
			name:     "everyone",
			subjects: subject(apitypes.MCPGuardrailSubjectEveryone, ""),
			want:     true,
		},
		{
			name:     "the user",
			subjects: subject(apitypes.MCPGuardrailSubjectUser, "7"),
			want:     true,
		},
		{
			name:     "the user's group",
			subjects: subject(apitypes.MCPGuardrailSubjectGroup, "eng"),
			want:     true,
		},
		{
			name:     "the agent",
			subjects: subject(apitypes.MCPGuardrailSubjectAgent, "5"),
			want:     true,
		},
		{
			name:     "another user",
			subjects: subject(apitypes.MCPGuardrailSubjectUser, "8"),
		},
		{
			name:     "the user with the agent's ID",
			subjects: subject(apitypes.MCPGuardrailSubjectUser, "5"),
		},
		{
			name:     "another group",
			subjects: subject(apitypes.MCPGuardrailSubjectGroup, "sales"),
		},
		{
			name:     "another agent",
			subjects: subject(apitypes.MCPGuardrailSubjectAgent, "7"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, subjectsMatch(filterSubjects(tt.subjects), identity))
		})
	}

	// The agent acting for itself is covered only by filters that name it.
	agent := &kuser.DefaultInfo{UID: "agent:5", Name: "agent:5", Extra: map[string][]string{}}
	principal.RecordPrincipal(agent.Extra, principal.KindAgent, apitypes.UserStatusActive)
	own, _, err := ResolveIdentity(t.Context(), nil, agent)
	require.NoError(t, err)
	require.Equal(t, subjects.Caller{}, own.OnBehalfOf)
	require.False(t, subjectsMatch(filterSubjects(subject(apitypes.MCPGuardrailSubjectEveryone, "")), own))
	require.False(t, subjectsMatch(filterSubjects(subject(apitypes.MCPGuardrailSubjectUser, "7")), own))

	// Conditions see the user the agent acts for.
	ok, err := evalCondition(types.GuardrailTargetMCP, `principal.on_behalf_of == "7" && principal.kind == "agent"`, identity, nil)
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = evalCondition(types.GuardrailTargetMCP, `principal.on_behalf_of == ""`, own, nil)
	require.NoError(t, err)
	require.True(t, ok)
}
