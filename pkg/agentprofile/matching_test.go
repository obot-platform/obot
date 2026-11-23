package agentprofile

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/stretchr/testify/require"
)

func TestMatches(t *testing.T) {
	entries := []types.AgentProfile{
		{Name: "everyone", Subjects: everyone},
		{Name: "alice", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}},
		{Name: "eng", Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}}},
		{Name: "admins", Subjects: []types.Subject{{Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin}}},
	}
	user := func(id uint, authProviderGroups, obotGroups []string) subjects.Caller {
		caller := subjects.Caller{
			Principal: principal.User(id),
			Groups: subjects.Groups{
				AuthProvider: map[string]struct{}{},
				Obot:         map[string]struct{}{},
			},
		}
		for _, group := range authProviderGroups {
			caller.Groups.AuthProvider[group] = struct{}{}
		}
		for _, group := range obotGroups {
			caller.Groups.Obot[group] = struct{}{}
		}
		return caller
	}

	for _, tt := range []struct {
		name    string
		entries []types.AgentProfile
		caller  subjects.Caller
		want    bool
	}{
		{
			name:    "a user named by ID",
			entries: entries[1:],
			caller:  user(7, nil, nil),
			want:    true,
		},
		{
			name:    "a user in a group",
			entries: entries[1:],
			caller:  user(8, []string{"eng"}, nil),
			want:    true,
		},
		{
			name:    "a user in an Obot group",
			entries: entries[1:],
			caller:  user(8, nil, []string{types.GroupAdmin}),
			want:    true,
		},
		{
			name:    "a user no entry names but every user",
			entries: entries,
			caller:  user(9, []string{"sales"}, nil),
			want:    true,
		},
		{
			name:    "a user no entry names",
			entries: entries[1:],
			caller:  user(9, nil, nil),
		},
		{
			// An agent is never acted for, even one whose numeric ID is a user's that an entry names.
			name:    "an agent",
			entries: append(entries, types.AgentProfile{Name: "agent", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}}),
			caller:  subjects.Caller{Principal: principal.Agent(7)},
		},
		{
			name:    "no entries",
			entries: nil,
			caller:  user(7, nil, nil),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, Matches(tt.entries, tt.caller))
		})
	}
}
