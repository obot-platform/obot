package adapter

import (
	"net/url"
	"strings"
	"unicode"
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
			Name: "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
			SetupDescription: "Client ID of an Okta API Services app, which Obot uses to fetch each user's groups from the " +
				"Okta Management API when they sign in. Leave this and the private key empty to provision users and groups " +
				"through SCIM instead.",
			UnusedDescription: "Not used: Okta provisions users and groups through SCIM, so Obot no longer calls the " +
				"Okta Management API. You can remove it, then delete the API Services app in Okta.",
		},
		{
			Name: "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			SetupDescription: "PEM-encoded RSA private key of the Okta API Services app. Leave this and the client ID " +
				"empty to provision users and groups through SCIM instead.",
			UnusedDescription: "Not used: Okta provisions users and groups through SCIM, so Obot no longer calls the " +
				"Okta Management API. You can remove it, then delete the API Services app in Okta.",
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

func (okta) IssuerConfigurationParameter() string {
	return "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL"
}

func (okta) NativeUserID(user User) (string, error) {
	id := strings.TrimSpace(user.ExternalID)
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

func (okta) NewUserIdentity(nativeUserID string) Identity {
	return Identity{
		Username:              nativeUserID,
		ProviderUsername:      nativeUserID,
		ProviderUserID:        nativeUserID,
		ProviderGroupLookupID: nativeUserID,
	}
}

func (okta) NewGroupID(groupIDPrefix, scimID string) string {
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
