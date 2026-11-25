package agentprofile

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

var (
	everyone = []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}
)

func TestValidate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []types.AgentProfile
		want    string
	}{
		{
			name: "no entries",
		},
		{
			name: "entries with users, groups, Obot groups and All Obot Users",
			entries: []types.AgentProfile{
				{
					Name:     "everyone",
					Subjects: everyone,
				},
				{
					Name:     "alice",
					Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "5"}},
				},
				{
					Name:     "groups",
					Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "eng"}, {Type: types.SubjectTypeObotGroup, ID: types.GroupAdmin}},
				},
			},
		},
		{
			name: "an entry that is not valid on its own",
			entries: []types.AgentProfile{
				{
					Name:     "agent",
					Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
				},
			},
			want: `agent profile "agent": invalid subject: agent subjects are not supported`,
		},
		{
			name: "duplicate names",
			entries: []types.AgentProfile{
				{
					Name:     "same",
					Subjects: everyone,
				},
				{
					Name:     " same ",
					Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "5"}},
				},
			},
			want: `duplicate agent profile name "same"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.entries)
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
		})
	}
}

func TestInitializeIDs(t *testing.T) {
	entries := []types.AgentProfile{{Name: "a"}, {Name: "b"}}
	require.NoError(t, InitializeIDs(entries))
	require.True(t, strings.HasPrefix(entries[0].ID, types.AgentProfileIDPrefix))
	require.Len(t, entries[0].ID, len(types.AgentProfileIDPrefix)+32)
	require.NotEqual(t, entries[0].ID, entries[1].ID)

	require.ErrorContains(t, InitializeIDs([]types.AgentProfile{{Name: "a", ID: "ap1chosen"}}), "server-assigned")
}

func TestReconcileIDs(t *testing.T) {
	existing := []types.AgentProfile{{ID: "ap1a", Name: "a"}, {ID: "ap1b", Name: "b"}}

	desired := []types.AgentProfile{
		// Renamed, and keeps its ID.
		{ID: "ap1b", Name: "renamed"},
		// New, with the name of a stored entry, and gets a new ID.
		{Name: "a"},
	}
	require.NoError(t, ReconcileIDs(existing, desired))
	require.Equal(t, "ap1b", desired[0].ID)
	require.True(t, strings.HasPrefix(desired[1].ID, types.AgentProfileIDPrefix))
	require.NotEqual(t, "ap1a", desired[1].ID)

	require.ErrorContains(t, ReconcileIDs(existing, []types.AgentProfile{{ID: "ap1c", Name: "c"}}), `unknown id "ap1c"`)
	require.ErrorContains(t, ReconcileIDs(existing, []types.AgentProfile{{ID: "ap1a", Name: "a"}, {ID: "ap1a", Name: "copy"}}), "of another entry")
	require.NoError(t, ReconcileIDs(existing, nil))
}
