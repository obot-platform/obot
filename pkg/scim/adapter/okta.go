package adapter

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/obot-platform/obot/pkg/i18n"
)

const (
	oktaAdapterType      = "okta"
	oktaAuthProviderName = "okta-auth-provider"

	// maxOktaUserIDLength bounds externalId. Native Okta user IDs are 20 characters.
	maxOktaUserIDLength = 64

	// oktaGroupIDPrefix and oktaIDLength describe native Okta group IDs, such as 00g1a2b3c4d5e6f7g8h9.
	oktaGroupIDPrefix = "00g"
	oktaIDLength      = 20
)

var (
	// oktaOrgDomains are the domains of Okta-hosted orgs, whose admin console is served from the org's subdomain
	// with an "-admin" suffix. Orgs on a custom domain have no admin console address that the issuer reveals.
	oktaOrgDomains = []string{
		".okta.com",
		".oktapreview.com",
		".okta-emea.com",
		".okta-gov.com",
	}

	oktaDirectoryParameters = []DirectoryParameter{
		{
			Name:               "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
			SetupDescription:   i18n.Text("en", "okta_client_id_setup"),
			SetupTranslations:  i18n.Locales("okta_client_id_setup"),
			UnusedDescription:  i18n.Text("en", "okta_parameter_unused"),
			UnusedTranslations: i18n.Locales("okta_parameter_unused"),
		},
		{
			Name:               "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			SetupDescription:   i18n.Text("en", "okta_private_key_setup"),
			SetupTranslations:  i18n.Locales("okta_private_key_setup"),
			UnusedDescription:  i18n.Text("en", "okta_parameter_unused"),
			UnusedTranslations: i18n.Locales("okta_parameter_unused"),
		},
	}
)

// okta holds the rules for Okta's SCIM client.
//
// Okta sends the native Okta user ID as the SCIM externalId on every user create, and keeps it stable across renames,
// deactivation, and reactivation. The Okta auth provider already stores that ID as the identity's provider user ID,
// provider username, and group lookup ID, and as the Obot username.
type okta struct{}

func (okta) Type() string {
	return oktaAdapterType
}

func (okta) DirectoryParameters() []DirectoryParameter {
	return oktaDirectoryParameters
}

// SCIMChoiceParameter returns none: omitting the directory parameters chooses SCIM.
func (okta) SCIMChoiceParameter() SCIMChoiceParameter {
	return SCIMChoiceParameter{}
}

func (okta) IssuerConfigurationParameter() string {
	return "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL"
}

func (o okta) NativeUserID(user User) (string, error) {
	return o.NativeUserIDFromExternalID(user.ExternalID)
}

func (okta) NativeUserIDFromExternalID(externalID string) (string, error) {
	id := strings.TrimSpace(externalID)
	switch {
	case id == "":
		return "", &InvalidUserError{
			Message: "externalId is required: Okta sends the native Okta user ID as the externalId",
		}
	case len(id) > maxOktaUserIDLength || strings.ContainsFunc(id, func(r rune) bool { return unicode.IsSpace(r) || !unicode.IsPrint(r) }):
		return "", &InvalidUserError{
			Message: "externalId is not a native Okta user ID",
		}
	}
	return id, nil
}

// UserKey returns UserKeyProviderUserID: the native Okta user ID is the identity's provider user ID.
func (okta) UserKey() UserKey {
	return UserKeyProviderUserID
}

func (okta) NewUserIdentity(nativeUserID string) Identity {
	return Identity{
		Username:              nativeUserID,
		ProviderUsername:      nativeUserID,
		ProviderUserID:        nativeUserID,
		ProviderGroupLookupID: nativeUserID,
	}
}

// GroupKey returns GroupKeyDisplayName. Okta sends no externalId for a group.
func (okta) GroupKey() GroupKey {
	return GroupKeyDisplayName
}

// NativeGroupIDFromExternalID refuses every externalId, because Okta groups bind by name.
func (okta) NativeGroupIDFromExternalID(string) (string, error) {
	return "", &InvalidGroupError{
		Message: "Okta groups are matched by displayName, not externalId",
	}
}

func (okta) NewGroupID(groupIDPrefix, scimID, _ string) string {
	return groupIDPrefix + scimID
}

func (okta) NativeGroupID(groupIDPrefix, groupID string) string {
	id, ok := strings.CutPrefix(groupID, groupIDPrefix)
	if !ok || groupIDPrefix == "" || len(id) != oktaIDLength || !strings.HasPrefix(id, oktaGroupIDPrefix) ||
		strings.ContainsFunc(id, func(r rune) bool { return r > unicode.MaxASCII || !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		return ""
	}
	return id
}

func (okta) GroupConsoleURL(issuer, nativeGroupID string) string {
	if nativeGroupID == "" {
		return ""
	}
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "https" || u.Port() != "" {
		return ""
	}

	host := strings.ToLower(u.Hostname())
	for _, domain := range oktaOrgDomains {
		org, ok := strings.CutSuffix(host, domain)
		if !ok || org == "" || strings.Contains(org, ".") {
			continue
		}
		if !strings.HasSuffix(org, "-admin") {
			org += "-admin"
		}
		return (&url.URL{
			Scheme: "https",
			Host:   org + domain,
			Path:   "/admin/group/" + nativeGroupID,
		}).String()
	}
	return ""
}

// PatchRules returns the rules of Okta's PATCH requests, which follow RFC 7644.
func (okta) PatchRules() PatchRules {
	return PatchRules{}
}

// Rules returns the shared behavior: Okta deactivates users instead of deleting them.
func (okta) Rules() Rules {
	return Rules{}
}

// ErrorHints returns no hints, so that Okta's admins see the shared details. Okta deprovisions a user by setting active
// to false, as the shared detail of a refused user DELETE asks.
func (okta) ErrorHints() ErrorHints {
	return ErrorHints{}
}
