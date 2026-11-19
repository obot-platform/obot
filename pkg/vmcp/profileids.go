package vmcp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/utils"
)

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
// IDs, because audit records name profiles by them.
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
// whatever its name, so renaming a profile keeps its identity in audit records. A profile sent without one is new and
// gets a new ID, even if it has the name of a stored profile, so a new profile is never recorded as a removed one. An
// ID that no stored profile has is refused.
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

// ProfileIDs returns the IDs of profiles, in order, leaving out profiles that have none yet.
func ProfileIDs(profiles []types.VMCPProfile) []string {
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if profile.ID != "" {
			ids = append(ids, profile.ID)
		}
	}
	return ids
}
