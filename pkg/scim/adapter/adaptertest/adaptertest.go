// Package adaptertest provides a SCIM adapter for tests of shared code that need rules no registered adapter has.
package adaptertest

import (
	"strings"

	"github.com/obot-platform/obot/pkg/scim/adapter"
)

const (
	// adapterType is the persisted type of Adapter.
	adapterType = "test"
)

// Adapter is a SCIM adapter whose rules are Okta's, apart from those its fields choose. With
// adapter.UserKeyGroupLookupID, a user that SCIM creates has a pending identity, and with adapter.GroupKeyExternalID, a
// group's ID is the prefix and its externalId. Register it with adapter.RegisterForTest.
type Adapter struct {
	Users       adapter.UserKey
	Groups      adapter.GroupKey
	SharedRules adapter.Rules
}

// okta returns the registered Okta adapter, whose rules Adapter uses by default.
func okta() adapter.Adapter {
	a, ok := adapter.Lookup("okta")
	if !ok {
		panic("the okta adapter is not registered")
	}
	return a
}

func (Adapter) Type() string {
	return adapterType
}

func (Adapter) DirectoryParameters() []adapter.DirectoryParameter {
	return okta().DirectoryParameters()
}

func (Adapter) SCIMChoiceParameter() adapter.SCIMChoiceParameter {
	return adapter.SCIMChoiceParameter{}
}

func (Adapter) IssuerConfigurationParameter() string {
	return okta().IssuerConfigurationParameter()
}

func (Adapter) NativeUserID(user adapter.User) (string, error) {
	return okta().NativeUserID(user)
}

func (Adapter) NativeUserIDFromExternalID(externalID string) (string, error) {
	return okta().NativeUserIDFromExternalID(externalID)
}

func (a Adapter) UserKey() adapter.UserKey {
	return a.Users
}

func (a Adapter) NewUserIdentity(nativeUserID string) adapter.Identity {
	if a.Users != adapter.UserKeyGroupLookupID {
		return okta().NewUserIdentity(nativeUserID)
	}
	return adapter.Identity{
		Username:              nativeUserID,
		ProviderUsername:      adapter.PendingProviderUserID(nativeUserID),
		ProviderUserID:        adapter.PendingProviderUserID(nativeUserID),
		ProviderGroupLookupID: nativeUserID,
	}
}

func (a Adapter) GroupKey() adapter.GroupKey {
	return a.Groups
}

func (a Adapter) NativeGroupIDFromExternalID(externalID string) (string, error) {
	id := strings.TrimSpace(externalID)
	if a.Groups != adapter.GroupKeyExternalID || id == "" {
		return "", &adapter.InvalidGroupError{
			Message: "externalId must be the group's native ID",
		}
	}
	return id, nil
}

func (a Adapter) NewGroupID(groupIDPrefix, scimID, nativeGroupID string) string {
	if a.Groups == adapter.GroupKeyExternalID {
		return groupIDPrefix + nativeGroupID
	}
	return groupIDPrefix + scimID
}

func (a Adapter) NativeGroupID(groupIDPrefix, groupID string) string {
	if a.Groups != adapter.GroupKeyExternalID {
		return ""
	}
	if id, ok := strings.CutPrefix(groupID, groupIDPrefix); ok && groupIDPrefix != "" {
		return id
	}
	return ""
}

func (Adapter) GroupConsoleURL(string, string) string {
	return ""
}

func (Adapter) PatchRules() adapter.PatchRules {
	return adapter.PatchRules{}
}

func (a Adapter) Rules() adapter.Rules {
	return a.SharedRules
}

func (Adapter) ErrorHints() adapter.ErrorHints {
	return adapter.ErrorHints{}
}
