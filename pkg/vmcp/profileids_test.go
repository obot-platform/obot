package vmcp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

var (
	selectorWildcard = types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}
	groupSubject     = types.Subject{Type: types.SubjectTypeGroup, ID: "engineering"}
	userSubject      = types.Subject{Type: types.SubjectTypeUser, ID: "1"}
)

// fakeProfileData records the deletions it is asked for and the profiles it is asked about, and reports data for the
// profiles in has.
type fakeProfileData struct {
	has     map[string]bool
	err     error
	deleted [][]string
	asked   [][]string
}

func (f *fakeProfileData) ProfilesWithData(_ context.Context, _ string, profileIDs []string) ([]string, error) {
	f.asked = append(f.asked, profileIDs)
	if f.err != nil {
		return nil, f.err
	}
	var result []string
	for _, id := range profileIDs {
		if f.has[id] {
			result = append(result, id)
		}
	}
	return result, nil
}

func (f *fakeProfileData) DeleteProfileData(_ context.Context, _ string, profileIDs []string) error {
	f.deleted = append(f.deleted, profileIDs)
	return f.err
}

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

func TestProfileSubjectsChanged(t *testing.T) {
	stored := types.VMCPManifest{Profiles: []types.VMCPProfile{
		{ID: "vp1a", Name: "a", Subjects: []types.Subject{selectorWildcard, groupSubject}},
		{ID: "vp1b", Name: "b", Subjects: []types.Subject{selectorWildcard}},
		{Name: "legacy", Subjects: []types.Subject{userSubject}},
	}}
	for _, tc := range []struct {
		name     string
		profiles []types.VMCPProfile
		want     bool
	}{
		{
			name:     "unchanged",
			profiles: stored.Profiles,
		},
		{
			name: "renamed, reordered subjects and profiles",
			profiles: []types.VMCPProfile{
				{ID: "vp1b", Name: "b2", Subjects: []types.Subject{selectorWildcard}},
				{ID: "vp1a", Name: "a", Subjects: []types.Subject{groupSubject, selectorWildcard}},
			},
		},
		{
			name: "profile removed",
			profiles: []types.VMCPProfile{
				{ID: "vp1a", Name: "a", Subjects: []types.Subject{selectorWildcard, groupSubject}},
			},
		},
		{
			name: "stored profile without an ID matched by name",
			profiles: []types.VMCPProfile{
				{ID: "vp1new", Name: "legacy", Subjects: []types.Subject{userSubject}},
			},
		},
		{
			name: "subject added",
			profiles: []types.VMCPProfile{
				{ID: "vp1b", Name: "b", Subjects: []types.Subject{selectorWildcard, userSubject}},
			},
			want: true,
		},
		{
			name: "subject removed",
			profiles: []types.VMCPProfile{
				{ID: "vp1a", Name: "a", Subjects: []types.Subject{selectorWildcard}},
			},
			want: true,
		},
		{
			name: "profile added",
			profiles: []types.VMCPProfile{
				{ID: "vp1c", Name: "c", Subjects: []types.Subject{groupSubject}},
			},
			want: true,
		},
		{
			name: "new profile with a stored profile's name",
			profiles: []types.VMCPProfile{
				{ID: "vp1other", Name: "b", Subjects: []types.Subject{selectorWildcard}},
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, ProfileSubjectsChanged(stored, types.VMCPManifest{Profiles: tc.profiles}))
		})
	}
}

func TestCheckWildcardProfiles(t *testing.T) {
	legacy := types.VMCPManifest{Profiles: []types.VMCPProfile{
		{ID: "vp1a", Name: "a", Subjects: []types.Subject{selectorWildcard}},
		{ID: "vp1b", Name: "b", Subjects: []types.Subject{selectorWildcard}},
	}}
	renamed := types.VMCPManifest{Profiles: []types.VMCPProfile{
		{ID: "vp1a", Name: "a2", Subjects: []types.Subject{selectorWildcard}},
		{ID: "vp1b", Name: "b", Subjects: []types.Subject{selectorWildcard}},
	}}
	changed := types.VMCPManifest{Profiles: []types.VMCPProfile{
		{ID: "vp1a", Name: "a", Subjects: []types.Subject{selectorWildcard, groupSubject}},
		{ID: "vp1b", Name: "b", Subjects: []types.Subject{selectorWildcard}},
	}}
	fixed := types.VMCPManifest{Profiles: []types.VMCPProfile{
		{ID: "vp1a", Name: "a", Subjects: []types.Subject{selectorWildcard}},
		{ID: "vp1b", Name: "b", Subjects: []types.Subject{groupSubject}},
	}}
	for _, tc := range []struct {
		name          string
		store         ProfileDataStore
		stored        *types.VMCPManifest
		desired       types.VMCPManifest
		wantViolation bool
		wantErr       bool
	}{
		{
			name:          "new vMCP must follow the rule",
			desired:       legacy,
			wantViolation: true,
		},
		{
			name:    "legacy vMCP keeps working on unrelated edits",
			store:   &fakeProfileData{},
			stored:  &legacy,
			desired: renamed,
		},
		{
			name:    "legacy vMCP without stores",
			stored:  &legacy,
			desired: legacy,
		},
		{
			name:          "changing subjects applies the rule",
			store:         &fakeProfileData{},
			stored:        &legacy,
			desired:       changed,
			wantViolation: true,
		},
		{
			name:    "a change that fixes the vMCP is allowed",
			store:   &fakeProfileData{has: map[string]bool{"vp1a": true}},
			stored:  &legacy,
			desired: fixed,
		},
		{
			name:          "profile data applies the rule",
			store:         ProfileDataStores{&fakeProfileData{}, &fakeProfileData{has: map[string]bool{"vp1b": true}}},
			stored:        &legacy,
			desired:       renamed,
			wantViolation: true,
		},
		{
			name:    "a failure to check profile data",
			store:   &fakeProfileData{err: errors.New("unavailable")},
			stored:  &legacy,
			desired: renamed,
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			violation, err := CheckWildcardProfiles(t.Context(), tc.store, "vmcp1", tc.stored, tc.desired)
			require.Equal(t, tc.wantErr, err != nil, "err = %v", err)
			require.Equal(t, tc.wantViolation, violation != nil, "violation = %v", violation)
		})
	}
}

func TestProfileDataStoresDeleteFromEveryStore(t *testing.T) {
	first, second := &fakeProfileData{}, &fakeProfileData{err: errors.New("unavailable")}
	err := ProfileDataStores{first, second}.DeleteProfileData(t.Context(), "vmcp1", []string{"vp1a"})
	require.ErrorContains(t, err, "unavailable")
	require.Equal(t, [][]string{{"vp1a"}}, first.deleted)
	require.Equal(t, [][]string{{"vp1a"}}, second.deleted)

	require.NoError(t, ProfileDataStores(nil).DeleteProfileData(t.Context(), "vmcp1", []string{"vp1a"}))
	withData, err := ProfileDataStores(nil).ProfilesWithData(t.Context(), "vmcp1", []string{"vp1a"})
	require.NoError(t, err)
	require.Empty(t, withData)

	withData, err = ProfileDataStores{
		&fakeProfileData{has: map[string]bool{"vp1a": true}},
		&fakeProfileData{has: map[string]bool{"vp1a": true, "vp1b": true}},
	}.ProfilesWithData(t.Context(), "vmcp1", []string{"vp1a", "vp1b", "vp1c"})
	require.NoError(t, err)
	require.Equal(t, []string{"vp1a", "vp1b"}, withData)
}

func TestCheckReplacedProfiles(t *testing.T) {
	stored := types.VMCPManifest{Profiles: []types.VMCPProfile{
		{ID: "vp1ops", Name: "ops", Subjects: []types.Subject{groupSubject}},
		{ID: "vp1dev", Name: "dev", Subjects: []types.Subject{userSubject}},
		{ID: "vp1empty", Name: "empty", Subjects: []types.Subject{userSubject}},
		{Name: "legacy", Subjects: []types.Subject{userSubject}},
	}}
	data := map[string]bool{"vp1ops": true, "vp1dev": true}
	for _, tc := range []struct {
		name      string
		profiles  []types.VMCPProfile
		wantError string
		wantAsked [][]string
	}{
		{
			name: "every profile sent with its ID",
			profiles: []types.VMCPProfile{
				{ID: "vp1ops", Name: "ops"},
				{ID: "vp1dev", Name: "renamed"},
			},
		},
		{
			name: "profiles with data removed by leaving them out",
			profiles: []types.VMCPProfile{
				{ID: "vp1empty", Name: "empty"},
			},
		},
		{
			name: "a new profile named after a stored profile without data",
			profiles: []types.VMCPProfile{
				{ID: "vp1ops", Name: "ops"},
				{ID: "vp1dev", Name: "dev"},
				{Name: "empty"},
			},
			wantAsked: [][]string{{"vp1empty"}},
		},
		{
			name: "a new profile named after a stored profile without an ID",
			profiles: []types.VMCPProfile{
				{Name: "legacy"},
			},
		},
		{
			name: "a new profile named after a stored profile whose ID moved to another profile",
			profiles: []types.VMCPProfile{
				{ID: "vp1ops", Name: "operations"},
				{Name: "ops"},
			},
		},
		{
			name: "profiles sent without IDs replace profiles with data",
			profiles: []types.VMCPProfile{
				{Name: "empty"},
				{Name: "ops"},
				{Name: "dev"},
			},
			wantError: `profile "ops" has stored configuration or OAuth grants; send its id ("vp1ops") to keep it, or remove the profile first`,
			wantAsked: [][]string{{"vp1empty", "vp1ops", "vp1dev"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeProfileData{has: data}
			violation, err := CheckReplacedProfiles(t.Context(), store, "vmcp1", stored, types.VMCPManifest{Profiles: tc.profiles})
			require.NoError(t, err)
			if tc.wantError == "" {
				require.NoError(t, violation)
			} else {
				require.EqualError(t, violation, tc.wantError)
			}
			require.Equal(t, tc.wantAsked, store.asked)
		})
	}

	violation, err := CheckReplacedProfiles(t.Context(), &fakeProfileData{err: errors.New("unavailable")}, "vmcp1", stored, types.VMCPManifest{Profiles: []types.VMCPProfile{{Name: "ops"}}})
	require.ErrorContains(t, err, "unavailable")
	require.NoError(t, violation)

	violation, err = CheckReplacedProfiles(t.Context(), nil, "vmcp1", stored, types.VMCPManifest{Profiles: []types.VMCPProfile{{Name: "ops"}}})
	require.NoError(t, err)
	require.NoError(t, violation)
}
