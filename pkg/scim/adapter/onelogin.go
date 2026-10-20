package adapter

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/pkg/i18n"
)

const (
	oneLoginAdapterType      = "onelogin"
	oneLoginAuthProviderName = "onelogin-auth-provider"

	// maxOneLoginIDLength bounds user and role IDs, which are positive integers, such as 263801284. The OneLogin auth
	// provider accepts IDs of up to 18 digits, which always fit in an int64, so a longer one could never sign in.
	maxOneLoginIDLength = 18

	// oneLoginDomain is the domain of OneLogin-hosted accounts, whose issuer and admin console are both served from
	// the account's subdomain.
	oneLoginDomain = ".onelogin.com"
)

var (
	oneLoginDirectoryParameters = []DirectoryParameter{
		{
			Name:               "OBOT_ONELOGIN_AUTH_PROVIDER_API_CLIENT_ID",
			SetupDescription:   i18n.Text("en", "onelogin_client_id_setup"),
			SetupTranslations:  i18n.Locales("onelogin_client_id_setup"),
			UnusedDescription:  i18n.Text("en", "onelogin_parameter_unused"),
			UnusedTranslations: i18n.Locales("onelogin_parameter_unused"),
		},
		{
			Name:               "OBOT_ONELOGIN_AUTH_PROVIDER_API_CLIENT_SECRET",
			SetupDescription:   i18n.Text("en", "onelogin_client_secret_setup"),
			SetupTranslations:  i18n.Locales("onelogin_client_secret_setup"),
			UnusedDescription:  i18n.Text("en", "onelogin_parameter_unused"),
			UnusedTranslations: i18n.Locales("onelogin_parameter_unused"),
		},
	}
)

// oneLogin holds the rules for OneLogin's "SCIM Provisioner with SAML (SCIM v2 Core)" connector.
//
// OneLogin sends only the attributes of the app's SCIM JSON Template, and the connector's default template has no
// externalId. With "externalId": "{$user.id}" added, every create carries the numeric OneLogin user ID as a JSON
// string, which is also the OIDC subject. The OneLogin auth provider stores that ID as the identity's provider user ID,
// provider username, and group lookup ID, and as the Obot username.
//
// Roles are pushed as groups. The OneLogin auth provider's groups are onelogin/<role ID>, so a group that SCIM creates
// binds to such a group by name, and keeps its ID.
type oneLogin struct{}

func (oneLogin) Type() string {
	return oneLoginAdapterType
}

func (oneLogin) DirectoryParameters() []DirectoryParameter {
	return oneLoginDirectoryParameters
}

func (oneLogin) IssuerConfigurationParameter() string {
	return "OBOT_ONELOGIN_AUTH_PROVIDER_ISSUER_URL"
}

// NativeUserID requires the numeric OneLogin user ID as both externalId and userName. OneLogin shows the message of
// a refusal to its admin after its own prefix, such as "User failed creating in app.", and never retries the request,
// so each message names the setting to change.
//
// Before it creates a user, OneLogin looks the user up by the app's SCIM Username parameter, and adopts any user the
// lookup finds: it writes the new person's attributes, including their externalId, to the found user. On the
// parameter's default, Username, a person who re-uses a deleted person's username is therefore matched to the deleted
// person's deactivated account. Obot refuses the changed externalId, which leaves the new person stuck until admins
// on both sides intervene. On OneLogin ID, which OneLogin never reuses, the lookup finds only the same person, such as
// a user whose app access was removed and then restored. Requiring userName to be that ID refuses a misconfigured
// app's first create, before any username can be re-used.
func (oneLogin) NativeUserID(user User) (string, error) {
	id := strings.TrimSpace(user.ExternalID)
	userName := strings.TrimSpace(user.UserName)
	switch {
	case !isOneLoginID(id) && !isOneLoginID(userName):
		// Both settings are wrong. Naming both saves the admin a second failed Retry.
		return "", &InvalidUserError{
			Message: `externalId and userName must both be the numeric OneLogin user ID. Make the SCIM JSON Template on ` +
				`the OneLogin app's Configuration tab send "externalId": "{$user.id}", and on the Parameters tab, set ` +
				`SCIM Username to OneLogin ID.`,
		}
	case id == "":
		return "", &InvalidUserError{
			Message: `externalId is required. Add "externalId": "{$user.id}" to the SCIM JSON Template on the OneLogin ` +
				`app's Configuration tab.`,
		}
	case !isOneLoginID(id):
		return "", &InvalidUserError{
			Message: `externalId must be the numeric OneLogin user ID. Set it to "{$user.id}" in the SCIM JSON Template ` +
				`on the OneLogin app's Configuration tab.`,
		}
	case userName != id:
		return "", &InvalidUserError{
			Message: "userName must be the OneLogin user ID, the same as externalId. On the OneLogin app's Parameters " +
				"tab, set SCIM Username to OneLogin ID. OneLogin finds existing users by SCIM Username, and a username " +
				"can be re-used by another person.",
		}
	}
	return id, nil
}

func (oneLogin) NewUserIdentity(nativeUserID string) Identity {
	return Identity{
		Username:              nativeUserID,
		ProviderUsername:      nativeUserID,
		ProviderUserID:        nativeUserID,
		ProviderGroupLookupID: nativeUserID,
	}
}

func (oneLogin) NewGroupID(groupIDPrefix, scimID string) string {
	return groupIDPrefix + scimID
}

// NativeGroupID returns the role ID of a group that the OneLogin auth provider cached. Groups that SCIM created have
// UUIDs, which are not role IDs.
func (oneLogin) NativeGroupID(groupIDPrefix, groupID string) string {
	id, ok := strings.CutPrefix(groupID, groupIDPrefix)
	if !ok || groupIDPrefix == "" || !isOneLoginID(id) {
		return ""
	}
	return id
}

// GroupConsoleURL returns the role's page in the admin console, which is served from the issuer's host, as in
// https://acme.onelogin.com/roles/1113742/edit for the issuer https://acme.onelogin.com/oidc/2. Only an issuer on a
// OneLogin-hosted subdomain reveals where the admin console is.
func (oneLogin) GroupConsoleURL(issuer, nativeGroupID string) string {
	if !isOneLoginID(nativeGroupID) {
		return ""
	}
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme != "https" || u.Port() != "" || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	account, ok := strings.CutSuffix(host, oneLoginDomain)
	if !ok || account == "" || strings.Contains(account, ".") {
		return ""
	}
	return (&url.URL{
		Scheme: "https",
		Host:   host,
		Path:   "/roles/" + nativeGroupID + "/edit",
	}).String()
}

// PatchRules returns the rules of the Core connector's PATCH requests, which follow RFC 7644: one add or remove of a
// value list on members.
func (oneLogin) PatchRules() PatchRules {
	return PatchRules{}
}

// ErrorHints returns what a OneLogin admin must do after each failure. OneLogin shows a failure's detail in its event
// verbatim, after its own prefix, such as "User failed deleting in app.", and retries nothing but a 429. Every other
// failure of a user's task waits for an admin to click Retry on the user, on the app's Users tab or the user's
// Applications tab.
func (oneLogin) ErrorHints() ErrorHints {
	return ErrorHints{
		// Requests that are not a user's task, such as the connection test and an entitlements refresh, have no
		// Retry button, so they are run again instead.
		Retry: "In OneLogin, click Retry on the user in the app's Users tab, or run the connection test or " +
			"entitlements refresh again.",
		// OneLogin sends a user DELETE only when the action for deleted users and removed app access is Delete. The
		// action for suspended users offers only Suspend and Do Nothing, so the hint doesn't name it. With Suspend,
		// removing a user's access and deleting the user each send active false, and restoring access reactivates
		// the same user. Retry uses the app's current action, so after the change it deactivates the user.
		UserDelete: `Obot does not delete users. On the OneLogin app's Provisioning tab, set "When users are ` +
			`deleted in OneLogin, or the user's app access is removed" to Suspend, then click Retry on this user.`,
		// OneLogin's "SCIM2 PATCH for Groups" connector adds a member with a filtered path and no value. An app's
		// connector is chosen when the app is created.
		ValuelessFilteredAdd: "This app uses a connector Obot does not support. Create a OneLogin app with the " +
			"'SCIM Provisioner with SAML (SCIM v2 Core)' connector and grant users access to it instead.",
		// OneLogin writes a user only to an account its lookup by SCIM Username found, and NativeUserID refuses a
		// userName that is not the user's own ID. A changed externalId is therefore not expected, and says only
		// what it means.
		ExternalIDChanged: "This OneLogin user's ID does not match the account the request was sent to, so that " +
			"account belongs to another OneLogin user.",
	}
}

// isOneLoginID reports whether id is a OneLogin user or role ID: a positive integer, in canonical decimal form with
// no leading zeros, of at most maxOneLoginIDLength digits, as the OneLogin auth provider parses it.
func isOneLoginID(id string) bool {
	if id == "" || len(id) > maxOneLoginIDLength {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
}
