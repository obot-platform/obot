package vmcp

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

var (
	groupSubject = types.Subject{Type: types.SubjectTypeGroup, ID: "engineering"}
	userSubject  = types.Subject{Type: types.SubjectTypeUser, ID: "1"}
)

func TestInitializeProfileIDs(t *testing.T) {
	manifest := types.VMCPManifest{Profiles: []types.VMCPProfile{{Name: "a"}, {Name: "b"}}}
	require.NoError(t, InitializeProfileIDs(&manifest))
	for _, profile := range manifest.Profiles {
		require.True(t, strings.HasPrefix(profile.ID, types.VMCPProfileIDPrefix), profile.ID)
		require.NoError(t, types.ValidateVMCPProfileID(profile.ID))
	}
	require.NotEqual(t, manifest.Profiles[0].ID, manifest.Profiles[1].ID)

	chosen := types.VMCPManifest{Profiles: []types.VMCPProfile{{ID: "mine", Name: "a"}}}
	require.ErrorContains(t, InitializeProfileIDs(&chosen), "server-assigned")
}

func TestReconcileProfileIDs(t *testing.T) {
	existing := types.VMCPManifest{Profiles: []types.VMCPProfile{
		{ID: "vp1a", Name: "a", Subjects: []types.Subject{groupSubject}},
		{ID: "vp1b", Name: "b", Subjects: []types.Subject{userSubject}},
	}}

	t.Run("rename, reorder and unrelated edits keep IDs", func(t *testing.T) {
		desired := types.VMCPManifest{Profiles: []types.VMCPProfile{
			{ID: "vp1b", Name: "b", Subjects: []types.Subject{userSubject, groupSubject}},
			{ID: "vp1a", Name: "renamed", Subjects: []types.Subject{groupSubject}},
		}}
		require.NoError(t, ReconcileProfileIDs(existing, &desired))
		require.Equal(t, "vp1b", desired.Profiles[0].ID)
		require.Equal(t, "vp1a", desired.Profiles[1].ID)
	})

	t.Run("a profile without an ID is new, even with a stored profile's name", func(t *testing.T) {
		desired := types.VMCPManifest{Profiles: []types.VMCPProfile{
			{Name: "a", Subjects: []types.Subject{groupSubject}},
			{ID: "vp1b", Name: "b", Subjects: []types.Subject{userSubject}},
		}}
		require.NoError(t, ReconcileProfileIDs(existing, &desired))
		require.NotEmpty(t, desired.Profiles[0].ID)
		require.NotEqual(t, "vp1a", desired.Profiles[0].ID)
		require.Equal(t, "vp1b", desired.Profiles[1].ID)
	})

	t.Run("an unknown ID is refused", func(t *testing.T) {
		desired := types.VMCPManifest{Profiles: []types.VMCPProfile{{ID: "vp1invented", Name: "a"}}}
		require.ErrorContains(t, ReconcileProfileIDs(existing, &desired), `unknown ID "vp1invented"`)
	})
}

func TestDerivedProfileIDs(t *testing.T) {
	id := DerivedProfileID("vmcp1one", "profile")
	require.Equal(t, id, DerivedProfileID("vmcp1one", "profile"))
	require.NoError(t, types.ValidateVMCPProfileID(id))
	require.True(t, strings.HasPrefix(id, types.VMCPProfileIDPrefix))
	require.NotEqual(t, id, DerivedProfileID("vmcp1two", "profile"))
	require.NotEqual(t, id, DerivedProfileID("vmcp1one", "renamed"))

	manifest := types.VMCPManifest{Profiles: []types.VMCPProfile{{ID: "explicit", Name: "a"}, {Name: "b"}}}
	require.True(t, AssignDerivedProfileIDs("vmcp1one", &manifest))
	require.Equal(t, "explicit", manifest.Profiles[0].ID)
	require.Equal(t, DerivedProfileID("vmcp1one", "b"), manifest.Profiles[1].ID)
	require.False(t, AssignDerivedProfileIDs("vmcp1one", &manifest))
}
