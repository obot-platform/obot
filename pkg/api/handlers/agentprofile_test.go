package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

// profileFixture is an agent whose profiles the tests write.
type profileFixture struct {
	*agentIdentityFixture
	agent types.AgentIdentity
	path  string
}

func newProfileFixture(t *testing.T) *profileFixture {
	t.Helper()

	f := newAgentIdentityFixture(t)
	agent := f.createAgent(types.AgentIdentityManifest{DisplayName: "assistant"})
	return &profileFixture{
		agentIdentityFixture: f,
		agent:                agent,
		path:                 "/api/agents/" + agent.ID,
	}
}

// get returns the agent. It decodes into a new value each time: decoding into one that holds entries would merge
// into them.
func (f *profileFixture) get() types.AgentIdentity {
	f.t.Helper()
	var agent types.AgentIdentity
	f.call(http.StatusOK, http.MethodGet, f.path, nil, &agent)
	return agent
}

// put replaces the agent's profiles, which must succeed, and returns the agent.
func (f *profileFixture) put(profiles []types.AgentProfile) types.AgentIdentity {
	f.t.Helper()
	var agent types.AgentIdentity
	f.call(http.StatusOK, http.MethodPut, f.path, types.AgentIdentityManifest{DisplayName: "assistant", Profiles: profiles}, &agent)
	return agent
}

func everyUser() []types.Subject {
	return []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}
}

func TestAgentProfileRoundTripOverHTTP(t *testing.T) {
	f := newProfileFixture(t)

	require.Empty(t, f.get().Profiles)

	var created types.AgentIdentity
	f.call(http.StatusCreated, http.MethodPost, "/api/agents", types.AgentIdentityManifest{
		DisplayName: "new",
		Profiles: []types.AgentProfile{
			{Name: " everyone ", Subjects: everyUser()},
		},
	}, &created)
	require.Len(t, created.Profiles, 1)
	require.True(t, strings.HasPrefix(created.Profiles[0].ID, types.AgentProfileIDPrefix))
	require.Equal(t, "everyone", created.Profiles[0].Name, "names are trimmed")
	require.Equal(t, everyUser(), created.Profiles[0].Subjects)

	// Create refuses an ID.
	f.call(http.StatusBadRequest, http.MethodPost, "/api/agents", types.AgentIdentityManifest{
		DisplayName: "new",
		Profiles:    []types.AgentProfile{{ID: "ap1chosen", Name: "everyone", Subjects: everyUser()}},
	}, nil)

	entries := []types.AgentProfile{
		{
			Name:     "everyone",
			Subjects: everyUser(),
		},
		{
			Name: "engineering",
			Subjects: []types.Subject{
				{Type: types.SubjectTypeGroup, ID: "eng"},
				{Type: types.SubjectTypeUser, ID: "2"},
				{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin},
			},
		},
	}
	updated := f.put(entries)
	require.Len(t, updated.Profiles, 2)
	for i, entry := range updated.Profiles {
		require.True(t, strings.HasPrefix(entry.ID, types.AgentProfileIDPrefix))
		entries[i].ID = entry.ID
	}
	require.Equal(t, entries, updated.Profiles)

	require.Equal(t, entries, f.get().Profiles)
	var list types.AgentIdentityList
	f.call(http.StatusOK, http.MethodGet, "/api/agents", nil, &list)
	require.Equal(t, entries, list.Items[0].Profiles)

	// IDs carry over, a new entry gets one, and an unknown one is refused.
	entries[1].Name = "eng"
	again := f.put([]types.AgentProfile{entries[1], {Name: "new", Subjects: everyUser()}})
	require.Equal(t, entries[1], again.Profiles[0])
	require.NotEmpty(t, again.Profiles[1].ID)
	require.NotEqual(t, entries[0].ID, again.Profiles[1].ID)

	f.call(http.StatusBadRequest, http.MethodPut, f.path, types.AgentIdentityManifest{
		DisplayName: "assistant",
		Profiles:    []types.AgentProfile{{ID: entries[0].ID, Name: "removed", Subjects: everyUser()}},
	}, nil)
	f.call(http.StatusBadRequest, http.MethodPut, f.path, types.AgentIdentityManifest{
		DisplayName: "assistant",
		Profiles:    []types.AgentProfile{{ID: "ap1unknown", Name: "unknown", Subjects: everyUser()}},
	}, nil)
	require.Equal(t, again.Profiles, f.get().Profiles, "a refused update changes nothing")

	// An update that leaves profiles out removes them, as it does every field it leaves out.
	var cleared types.AgentIdentity
	f.call(http.StatusOK, http.MethodPut, f.path, types.AgentIdentityManifest{DisplayName: "renamed"}, &cleared)
	require.Empty(t, cleared.Profiles)
}

func TestAgentProfileValidationOverHTTP(t *testing.T) {
	f := newProfileFixture(t)

	for _, tt := range []struct {
		name   string
		access []types.AgentProfile
		want   string
	}{
		{
			name:   "agent subject",
			access: []types.AgentProfile{{Name: "x", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: f.agent.ID}}}},
			want:   "agent subjects are not supported",
		},
		{
			name:   "duplicate names",
			access: []types.AgentProfile{{Name: "x", Subjects: everyUser()}, {Name: "x", Subjects: everyUser()}},
			want:   "duplicate agent profile name",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec := f.do(http.MethodPut, f.path, types.AgentIdentityManifest{DisplayName: "assistant", Profiles: tt.access})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), tt.want)
		})
	}

	require.Empty(t, f.get().Profiles, "refused updates change nothing")

	// Unknown fields of an entry are refused, as they are of the agent. A profile only says which users the agent may
	// act for, so it has no field for what the agent may use.
	for _, field := range []string{`"vmcps":{"off":true}`, `"llm":{"ids":["m1"]}`, `"skills":{}`, `"tools":[]`} {
		f.call(http.StatusBadRequest, http.MethodPut, f.path, `{"displayName":"assistant","profiles":[{"name":"x","subjects":[{"type":"selector","id":"*"}],`+field+`}]}`, nil)
	}
	require.Empty(t, f.get().Profiles)
}
