package client

import (
	"encoding/json"
	"strings"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

// TestAgentProfilesAreStoredWithIDs checks that the profiles of an agent are written whole by create and update,
// that create assigns every entry an ID, and that update keeps the IDs of stored entries, gives new entries new IDs, and
// refuses IDs the agent does not have.
func TestAgentProfilesAreStoredWithIDs(t *testing.T) {
	for _, database := range grantTestDatabases() {
		t.Run(database.name, func(t *testing.T) {
			c := database.open(t)
			ctx := t.Context()
			everyone := []apitypes.Subject{{Type: apitypes.SubjectTypeSelector, ID: apitypes.SubjectWildcardID}}

			_, err := c.CreateAgent(ctx, types.Agent{
				DisplayName: "builder",
				Profiles:    []apitypes.AgentProfile{{ID: "ap1chosen", Name: "chosen", Subjects: everyone}},
			})
			require.ErrorIs(t, err, ErrInvalidAgentMetadata)
			require.ErrorContains(t, err, "server-assigned")

			input := []apitypes.AgentProfile{
				{
					Name:     "everyone",
					Subjects: everyone,
				},
				{
					Name:     "eng",
					Subjects: []apitypes.Subject{{Type: apitypes.SubjectTypeGroup, ID: "eng"}},
				},
			}
			agent, err := c.CreateAgent(ctx, types.Agent{DisplayName: "builder", Profiles: input})
			require.NoError(t, err)
			// The caller's entries are not changed.
			require.Empty(t, input[0].ID)

			stored := storedAgent(t, c, agent.ID).Profiles
			require.Len(t, stored, 2)
			for _, entry := range stored {
				require.True(t, strings.HasPrefix(entry.ID, apitypes.AgentProfileIDPrefix), entry.ID)
			}
			require.NotEqual(t, stored[0].ID, stored[1].ID)
			require.Equal(t, agent.Profiles, stored)
			require.Equal(t, everyone, stored[0].Subjects)

			// The first entry is renamed and keeps its ID, the second is dropped, and a new one is added, with the name
			// the dropped one had.
			renamed := stored[0]
			renamed.Name = "all users"
			updated, err := c.UpdateAgent(ctx, types.Agent{
				ID:          agent.ID,
				DisplayName: "builder",
				Profiles: []apitypes.AgentProfile{
					renamed,
					{Name: "eng", Subjects: []apitypes.Subject{{Type: apitypes.SubjectTypeGroup, ID: "eng"}}},
				},
			})
			require.NoError(t, err)
			after := storedAgent(t, c, agent.ID).Profiles
			require.Equal(t, updated.Profiles, after)
			require.Len(t, after, 2)
			require.Equal(t, stored[0].ID, after[0].ID)
			require.Equal(t, "all users", after[0].Name)
			require.NotEqual(t, stored[1].ID, after[1].ID)
			require.NotEmpty(t, after[1].ID)

			for _, tt := range []struct {
				name    string
				entries []apitypes.AgentProfile
				want    string
			}{
				{
					name:    "unknown ID",
					entries: []apitypes.AgentProfile{{ID: "ap1unknown", Name: "x", Subjects: everyone}},
					want:    "unknown id",
				},
				{
					// The ID of an entry the update removed is no longer the agent's.
					name:    "ID of a removed entry",
					entries: []apitypes.AgentProfile{{ID: stored[1].ID, Name: "x", Subjects: everyone}},
					want:    "unknown id",
				},
				{
					name: "the same ID twice",
					entries: []apitypes.AgentProfile{
						{ID: after[0].ID, Name: "x", Subjects: everyone},
						{ID: after[0].ID, Name: "y", Subjects: everyone},
					},
					want: "of another entry",
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					_, err := c.UpdateAgent(ctx, types.Agent{ID: agent.ID, DisplayName: "renamed", Profiles: tt.entries})
					require.ErrorIs(t, err, ErrInvalidAgentMetadata)
					require.ErrorContains(t, err, tt.want)
					// A refused update changes nothing.
					require.Equal(t, after, storedAgent(t, c, agent.ID).Profiles)
					require.Equal(t, "builder", storedAgent(t, c, agent.ID).DisplayName)
				})
			}

			// An update without profiles removes every entry.
			_, err = c.UpdateAgent(ctx, types.Agent{ID: agent.ID, DisplayName: "builder"})
			require.NoError(t, err)
			require.Empty(t, storedAgent(t, c, agent.ID).Profiles)
		})
	}
}

// TestAdminAuditOfAgentProfiles checks that a change to an agent's profiles is recorded as an update of the
// agent, with its agent profiles before and after, and that saving the same profiles records nothing.
func TestAdminAuditOfAgentProfiles(t *testing.T) {
	c := newTestClient(t)
	admin := WithAdminActor(t.Context(), "42")
	everyone := []apitypes.Subject{{Type: apitypes.SubjectTypeSelector, ID: apitypes.SubjectWildcardID}}

	agent, err := c.CreateAgent(admin, types.Agent{
		DisplayName: "builder",
		Profiles:    []apitypes.AgentProfile{{Name: "everyone", Subjects: everyone}},
	})
	require.NoError(t, err)
	created := agent.Profiles

	changed := []apitypes.AgentProfile{created[0]}
	changed[0].Subjects = []apitypes.Subject{{Type: apitypes.SubjectTypeGroup, ID: "eng"}}
	_, err = c.UpdateAgent(admin, types.Agent{ID: agent.ID, DisplayName: "builder", Profiles: changed})
	require.NoError(t, err)
	// The same profiles again, with an empty list where the stored one has none, changes nothing.
	same := []apitypes.AgentProfile{changed[0]}
	same[0].Subjects = append([]apitypes.Subject{}, same[0].Subjects...)
	_, err = c.UpdateAgent(admin, types.Agent{ID: agent.ID, DisplayName: "builder", Profiles: same})
	require.NoError(t, err)
	// A change of the description alone records no profiles.
	_, err = c.UpdateAgent(admin, types.Agent{ID: agent.ID, DisplayName: "builder", Description: "builds", Profiles: same})
	require.NoError(t, err)
	// Removing every entry is a change.
	_, err = c.UpdateAgent(admin, types.Agent{ID: agent.ID, DisplayName: "builder", Description: "builds"})
	require.NoError(t, err)

	events := adminAuditEvents(t, c)
	require.Len(t, events, 4)
	for i, action := range []string{"agent.create", "agent.update", "agent.update", "agent.update"} {
		require.Equal(t, action, events[i].Action)
	}

	// profiles decodes the before and after of the agent profiles an event records.
	profiles := func(event types.AdminAuditEvent) (before, after []apitypes.AgentProfile) {
		t.Helper()
		var details struct {
			Profiles *struct {
				Before []apitypes.AgentProfile `json:"before"`
				After  []apitypes.AgentProfile `json:"after"`
			} `json:"profiles"`
		}
		require.NoError(t, json.Unmarshal(event.Details, &details))
		require.NotNil(t, details.Profiles, string(event.Details))
		require.NotNil(t, details.Profiles.Before)
		require.NotNil(t, details.Profiles.After)
		return details.Profiles.Before, details.Profiles.After
	}

	before, after := profiles(events[0])
	require.Empty(t, before)
	require.Equal(t, created, after)

	require.Equal(t, []any{types.AdminAuditAgentProfiles}, adminAuditDetails(t, events[1])["changedFields"])
	before, after = profiles(events[1])
	require.Equal(t, created, before)
	require.Equal(t, changed, after)

	require.Equal(t, map[string]any{"changedFields": []any{"description"}}, adminAuditDetails(t, events[2]))

	require.Equal(t, []any{types.AdminAuditAgentProfiles}, adminAuditDetails(t, events[3])["changedFields"])
	before, after = profiles(events[3])
	require.Equal(t, changed, before)
	require.Empty(t, after)
}
