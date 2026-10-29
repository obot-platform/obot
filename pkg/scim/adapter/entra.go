package adapter

import (
	"strings"

	"github.com/obot-platform/obot/pkg/i18n"
)

const (
	entraAdapterType      = "entra"
	entraAuthProviderName = "entra-auth-provider"

	// entraSCIMChoiceParameterName chooses SCIM-first setup. The Entra auth provider reads users and groups from
	// Microsoft Graph with the same credentials it signs users in with, so no parameter is used by directory
	// synchronization alone, and omitting one cannot make the choice.
	entraSCIMChoiceParameterName = "OBOT_ENTRA_AUTH_PROVIDER_SCIM"

	// entraGroupConsoleURL is the address of a group's overview in the Microsoft Entra admin center, to which the
	// group's object ID is appended. It is the same for every tenant.
	entraGroupConsoleURL = "https://entra.microsoft.com/#view/Microsoft_AAD_IAM/GroupDetailsMenuBlade/~/Overview/groupId/"
)

// entra holds the rules for the Microsoft Entra ID provisioning service.
//
// The Entra auth provider keys identities by the OIDC subject, which is pairwise per application, and which the
// provisioning service cannot send. It stores the user's object ID as the identity's group lookup ID, so a user binds
// by that, and a user that SCIM creates waits for its first sign-in to learn its subject. The provisioning service
// sends the object ID as externalId only when the app's user mapping maps objectId to it; the default mapping sends
// mailNickname.
//
// Every group create carries the group's object ID as externalId, so a pushed group binds to, or is created as,
// entra/<object ID>, the ID that the Entra auth provider gives the group. Group names in Entra need not be unique.
//
// Entra sends a user DELETE when a user is permanently deleted, including the automatic permanent delete 30 days after
// a soft delete, with no deactivation before it. Entra expects the DELETE to remove the user, so Obot deletes the user.
type entra struct{}

func (entra) Type() string {
	return entraAdapterType
}

// DirectoryParameters returns none: directory synchronization differs from SCIM only in the Microsoft Graph
// application permissions granted in Entra.
func (entra) DirectoryParameters() []DirectoryParameter {
	return nil
}

func (entra) SCIMChoiceParameter() SCIMChoiceParameter {
	return SCIMChoiceParameter{
		Name:         entraSCIMChoiceParameterName,
		Description:  i18n.Text("en", "entra_scim_choice"),
		Translations: i18n.Locales("entra_scim_choice"),
	}
}

// IssuerConfigurationParameter returns the tenant ID parameter. The Entra auth provider has no issuer parameter, and
// derives its issuer from the tenant ID, which identifies the organization the bindings belong to just as well.
func (entra) IssuerConfigurationParameter() string {
	return "OBOT_ENTRA_AUTH_PROVIDER_TENANT_ID"
}

func (e entra) NativeUserID(user User) (string, error) {
	return e.NativeUserIDFromExternalID(user.ExternalID)
}

// NativeUserIDFromExternalID requires the user's object ID. Entra shows the message of a refusal to its admin verbatim
// in the provisioning logs, so it names the mapping to change. userName is not checked: it is the user principal
// name, which a soft delete changes.
func (entra) NativeUserIDFromExternalID(externalID string) (string, error) {
	id, ok := entraObjectID(externalID)
	if !ok {
		return "", &InvalidUserError{
			Message: "externalId must be the user's Entra object ID. In the enterprise app's Provisioning > Attribute " +
				"mapping > Provision Microsoft Entra ID Users, map objectId to externalId (the default maps mailNickname).",
		}
	}
	return id, nil
}

// UserKey returns UserKeyGroupLookupID: the object ID is the identity's group lookup ID, not its provider user ID.
func (entra) UserKey() UserKey {
	return UserKeyGroupLookupID
}

// NewUserIdentity returns a pending identity, which the user's first sign-in claims by object ID and re-keys to the
// subject.
func (entra) NewUserIdentity(nativeUserID string) Identity {
	return Identity{
		Username:              nativeUserID,
		ProviderUsername:      PendingProviderUserID(nativeUserID),
		ProviderUserID:        PendingProviderUserID(nativeUserID),
		ProviderGroupLookupID: nativeUserID,
	}
}

// GroupKey returns GroupKeyExternalID: every group create carries the group's object ID as externalId.
func (entra) GroupKey() GroupKey {
	return GroupKeyExternalID
}

func (entra) NativeGroupIDFromExternalID(externalID string) (string, error) {
	id, ok := entraObjectID(externalID)
	if !ok {
		return "", &InvalidGroupError{
			Message: "externalId must be the group's Entra object ID. In the enterprise app's Provisioning > Attribute " +
				"mapping > Provision Microsoft Entra ID Groups, map objectId to externalId.",
		}
	}
	return id, nil
}

// NewGroupID returns the ID that the Entra auth provider gives the group, so that a group that SCIM creates and a
// group that directory synchronization cached are the same group.
func (entra) NewGroupID(groupIDPrefix, _, nativeGroupID string) string {
	return groupIDPrefix + nativeGroupID
}

// NativeGroupID returns the object ID that every Entra group's ID carries.
func (entra) NativeGroupID(groupIDPrefix, groupID string) string {
	id, ok := strings.CutPrefix(groupID, groupIDPrefix)
	if !ok || groupIDPrefix == "" || !isCanonicalEntraObjectID(id) {
		return ""
	}
	return id
}

// GroupConsoleURL returns the group's overview in the Microsoft Entra admin center, which needs no issuer.
func (entra) GroupConsoleURL(_, nativeGroupID string) string {
	if !isCanonicalEntraObjectID(nativeGroupID) {
		return ""
	}
	return entraGroupConsoleURL + nativeGroupID
}

// PatchRules returns the rules of Entra's PATCH requests, which replace emails[type eq "work"].value for a user who
// has no work email.
func (entra) PatchRules() PatchRules {
	return PatchRules{
		ReplaceAddsUnmatched: true,
	}
}

// Rules returns that a user DELETE deletes the user. Admins who want to keep a user's data clear Delete under the
// user mapping's Target object actions, and Entra then sends nothing.
func (entra) Rules() Rules {
	return Rules{
		DeletesUsers: true,
	}
}

// ErrorHints returns what an Entra admin must do after each failure. Entra shows a failure's detail in the
// provisioning logs, and retries failed changes in a later cycle. A cycle whose probe fails twice quarantines the job,
// which then waits for an admin.
func (entra) ErrorHints() ErrorHints {
	return ErrorHints{
		Retry: "Entra retries failed changes in a later cycle. If the provisioning job shows Quarantine, select Restart " +
			"provisioning on the enterprise app's Provisioning page after Obot is reachable again.",
		ExternalIDChanged: "The externalId mapping changed. Map objectId to externalId, then restart provisioning.",
	}
}

// entraObjectID returns the canonical form of an Entra object ID: a GUID in its 8-4-4-4-12 hexadecimal form, in lower
// case, as Microsoft Graph and the Entra auth provider write it. Surrounding whitespace is ignored.
func entraObjectID(value string) (string, bool) {
	id := strings.ToLower(strings.TrimSpace(value))
	return id, isCanonicalEntraObjectID(id)
}

// isCanonicalEntraObjectID reports whether id is a lower-case GUID in its 8-4-4-4-12 hexadecimal form, without braces
// or a urn:uuid: prefix.
func isCanonicalEntraObjectID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return false
			}
		}
	}
	return true
}
