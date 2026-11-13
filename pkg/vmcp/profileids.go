package vmcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
)

// ProfileDataStore holds data that is keyed by vMCP profile ID and kept outside the vMCP manifest, such as profile
// configuration (ProfileConfigurationStore) and profile OAuth grants (ProfileOAuthGrantStore). Catalog-synced vMCPs
// keep such data too, because nothing about it is in the manifest that catalog sync replaces.
type ProfileDataStore interface {
	// ProfilesWithData returns the IDs, among profileIDs, of the vMCP's profiles that the store holds data for, so
	// that one call answers for many profiles. While any profile has data, the wildcard rule across profiles
	// (types.VMCPManifest.ValidateWildcardProfiles) applies to every change to the vMCP, with no legacy exception; and
	// an API update may not replace a profile that has data with a new profile of the same name
	// (CheckReplacedProfiles).
	ProfilesWithData(ctx context.Context, vmcpID string, profileIDs []string) ([]string, error)
	// DeleteProfileData deletes the data stored for the vMCP's profiles. The vMCP controller calls it with the
	// profiles removed from a vMCP, and with every profile of a vMCP being deleted. It must be idempotent and succeed
	// when there is nothing to delete: the controller calls it again with the same profiles after any failure,
	// including a failure to record the deletion after it succeeded.
	DeleteProfileData(ctx context.Context, vmcpID string, profileIDs []string) error
}

// ProfileDataStores combines the stores of profile data. Its zero value has none: no profile has data, and deleting
// does nothing.
type ProfileDataStores []ProfileDataStore

func (s ProfileDataStores) ProfilesWithData(ctx context.Context, vmcpID string, profileIDs []string) ([]string, error) {
	var result []string
	for _, store := range s {
		ids, err := store.ProfilesWithData(ctx, vmcpID, profileIDs)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if !slices.Contains(result, id) {
				result = append(result, id)
			}
		}
	}
	return result, nil
}

func (s ProfileDataStores) DeleteProfileData(ctx context.Context, vmcpID string, profileIDs []string) error {
	var errs []error
	for _, store := range s {
		errs = append(errs, store.DeleteProfileData(ctx, vmcpID, profileIDs))
	}
	return errors.Join(errs...)
}

// NewProfileID returns a random profile ID, for a profile created through the API.
func NewProfileID() string {
	id := make([]byte, 16)
	// crypto/rand.Read never fails; it crashes the program instead.
	_, _ = rand.Read(id)
	return types.VMCPProfileIDPrefix + hex.EncodeToString(id)
}

// DerivedProfileID returns the profile ID that catalog sync, migrations and the vMCP controller give a profile that
// has none. It depends only on the vMCP and profile names, so every sync of unchanged catalog YAML derives the same ID,
// and so does the controller for a vMCP stored before profiles had IDs.
func DerivedProfileID(vmcpName, profileName string) string {
	return types.VMCPProfileIDPrefix + utils.Digest([]string{vmcpName, profileName})[:32]
}

// InitializeProfileIDs assigns new IDs to every profile during vMCP creation through the API. Clients cannot choose
// IDs, because profile data is stored under them.
func InitializeProfileIDs(manifest *types.VMCPManifest) error {
	for index := range manifest.Profiles {
		if manifest.Profiles[index].ID != "" {
			return fmt.Errorf("profile %q ID is server-assigned and must be omitted", manifest.Profiles[index].Name)
		}
		manifest.Profiles[index].ID = NewProfileID()
	}
	return nil
}

// ReconcileProfileIDs preserves profile IDs during an update through the API. A profile sent with an ID keeps it,
// whatever its name, so renaming a profile keeps its data. A profile sent without one is new and gets a new ID, even if
// it has the name of a stored profile, so a new profile never inherits a removed one's data. An ID that no stored
// profile has is refused.
func ReconcileProfileIDs(existing types.VMCPManifest, desired *types.VMCPManifest) error {
	for index := range desired.Profiles {
		profile := &desired.Profiles[index]
		if profile.ID == "" {
			profile.ID = NewProfileID()
			continue
		}
		if _, ok := existing.Profile(profile.ID); !ok {
			return fmt.Errorf("profile %q has unknown ID %q", profile.Name, profile.ID)
		}
	}
	return nil
}

// AssignDerivedProfileIDs gives each profile without an ID its DerivedProfileID, and reports whether it assigned any.
// Catalog sync uses it for profiles whose YAML sets no ID, migrations for the vMCPs they create, and the vMCP
// controller for vMCPs stored before profiles had IDs.
func AssignDerivedProfileIDs(vmcpName string, manifest *types.VMCPManifest) bool {
	var assigned bool
	for index := range manifest.Profiles {
		if manifest.Profiles[index].ID == "" {
			manifest.Profiles[index].ID = DerivedProfileID(vmcpName, manifest.Profiles[index].Name)
			assigned = true
		}
	}
	return assigned
}

// ProfileIDs returns the IDs of the manifest's profiles, in order, leaving out profiles that have none yet.
func ProfileIDs(manifest types.VMCPManifest) []string {
	ids := make([]string, 0, len(manifest.Profiles))
	for _, profile := range manifest.Profiles {
		if profile.ID != "" {
			ids = append(ids, profile.ID)
		}
	}
	return ids
}

// RecordedProfileIDs returns, once each, the profiles that vmcp's status records data for: those recorded before any
// data was stored for them (v1.VMCPStatus.ProfileIDs), and those that a configuration revision or an OAuth grant is
// recorded for, so that a profile with either is cleaned up even if it was somehow never recorded.
func RecordedProfileIDs(vmcp v1.VMCP) []string {
	var ids []string
	for _, id := range slices.Concat(vmcp.Status.ProfileIDs, slices.Sorted(maps.Keys(vmcp.Status.ProfileConfigurationRevisions)), ProfileOAuthGrantProfileIDs(vmcp)) {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// ProfileSubjectsChanged reports whether desired adds a profile or changes the subjects of a stored one, in any way
// other than their order. Profiles are matched by ID, or by name for a stored profile that has no ID yet. Removing
// profiles is not a change to their subjects.
func ProfileSubjectsChanged(stored, desired types.VMCPManifest) bool {
	for _, profile := range desired.Profiles {
		index := slices.IndexFunc(stored.Profiles, func(candidate types.VMCPProfile) bool {
			return candidate.ID != "" && candidate.ID == profile.ID || candidate.ID == "" && candidate.Name == profile.Name
		})
		if index < 0 || !sameSubjects(stored.Profiles[index].Subjects, profile.Subjects) {
			return true
		}
	}
	return false
}

// sameSubjects reports whether a and b hold the same subjects, ignoring order and repeats.
func sameSubjects(a, b []types.Subject) bool {
	for _, subject := range a {
		if !slices.Contains(b, subject) {
			return false
		}
	}
	for _, subject := range b {
		if !slices.Contains(a, subject) {
			return false
		}
	}
	return true
}

// CheckWildcardProfiles applies the wildcard rule across profiles (types.VMCPManifest.ValidateWildcardProfiles) to a
// vMCP being saved, and returns the violation, if any, as violation. err is a failure to check.
//
// stored is the vMCP as stored, or nil for a new one, which always follows the rule. A stored vMCP may predate the
// rule, and keeps working for users, so the rule applies to it only when desired adds a profile or changes a profile's
// subjects (ProfileSubjectsChanged), or when store holds data for any of its profiles. Nothing new is stored to make
// this exception: it compares with the stored manifest.
func CheckWildcardProfiles(ctx context.Context, store ProfileDataStore, vmcpID string, stored *types.VMCPManifest, desired types.VMCPManifest) (violation error, err error) {
	violation = desired.ValidateWildcardProfiles()
	if violation == nil || stored == nil || ProfileSubjectsChanged(*stored, desired) {
		return violation, nil
	}
	if store == nil {
		return nil, nil
	}
	withData, err := store.ProfilesWithData(ctx, vmcpID, ProfileIDs(*stored))
	if err != nil {
		return nil, fmt.Errorf("check vMCP %q for profile data: %w", vmcpID, err)
	}
	if len(withData) > 0 {
		return fmt.Errorf("%w; this vMCP has profile configuration or OAuth grants, so it must follow this rule", violation), nil
	}
	return nil, nil
}

// CheckReplacedProfiles refuses an API update that would replace a stored profile that has data with a new profile of
// the same name, and returns the refusal as violation. err is a failure to check. It must see the request before
// ReconcileProfileIDs gives new profiles their IDs.
//
// A profile sent without an ID is new (ReconcileProfileIDs), so a client that does not send IDs back would otherwise
// silently drop each profile's configuration and OAuth grants, and a new profile would look like the removed one it is
// named after. Such an update is refused when a profile without an ID has the name of a stored profile that has data
// and whose ID the request does not send. Removing a profile by leaving it out, and adding a profile named after a
// stored one that has no data, are still allowed. The store is asked once, for every such stored profile.
func CheckReplacedProfiles(ctx context.Context, store ProfileDataStore, vmcpID string, stored, request types.VMCPManifest) (violation error, err error) {
	if store == nil {
		return nil, nil
	}
	sent := make(map[string]struct{}, len(request.Profiles))
	for _, profile := range request.Profiles {
		if profile.ID != "" {
			sent[profile.ID] = struct{}{}
		}
	}
	// replaced maps each new profile's name to the ID of the stored profile of that name that the request leaves out.
	replaced := map[string]string{}
	var candidates []string
	for _, profile := range request.Profiles {
		if profile.ID != "" {
			continue
		}
		index := slices.IndexFunc(stored.Profiles, func(candidate types.VMCPProfile) bool { return candidate.Name == profile.Name })
		if index < 0 {
			continue
		}
		// A stored profile without an ID predates profile IDs, so nothing can be stored for it.
		id := stored.Profiles[index].ID
		if _, ok := sent[id]; id == "" || ok {
			continue
		}
		replaced[profile.Name] = id
		candidates = append(candidates, id)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	withData, err := store.ProfilesWithData(ctx, vmcpID, candidates)
	if err != nil {
		return nil, fmt.Errorf("check vMCP %q for profile data: %w", vmcpID, err)
	}
	for _, profile := range request.Profiles {
		if id, ok := replaced[profile.Name]; ok && profile.ID == "" && slices.Contains(withData, id) {
			return fmt.Errorf("profile %q has stored configuration or OAuth grants; send its id (%q) to keep it, or remove the profile first", profile.Name, id), nil
		}
	}
	return nil, nil
}
