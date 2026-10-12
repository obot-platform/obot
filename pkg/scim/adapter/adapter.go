// Package adapter holds the provider-specific rules of SCIM connections.
//
// Shared SCIM code never infers provider behavior from names, prefixes, or identifier shapes. The registry maps an
// auth provider name to its adapter when a connection is created, and the connection persists the adapter's type,
// which selects the provider-specific rules from then on. Supporting another provider means registering another
// adapter.
package adapter

var (
	// byAuthProvider maps the name of each auth provider that supports SCIM to its adapter. Looking an auth provider
	// up here is the only place where shared code reads a provider name.
	byAuthProvider = map[string]Adapter{
		oktaAuthProviderName: okta{},
	}

	// byType maps each adapter's persisted type to the adapter.
	byType = map[string]Adapter{
		oktaAdapterType: okta{},
	}
)

// Adapter holds the rules that differ between identity providers.
type Adapter interface {
	// Type is the adapter's persisted type.
	Type() string

	// DirectoryParameters are the auth provider configuration parameters that only login-time directory
	// synchronization uses.
	DirectoryParameters() []DirectoryParameter

	// IssuerConfigurationParameter names the auth provider configuration parameter that holds the identity
	// provider's issuer URL.
	IssuerConfigurationParameter() string

	// NativeUserID returns the identity provider's immutable ID for a SCIM user. A user binds to the existing
	// identity of the connection's auth provider that stores the same ID.
	NativeUserID(user User) (string, error)

	// NewUserIdentity returns the Obot username and identity fields for a user that SCIM creates, so that the user's
	// later sign-ins find the same identity.
	NewUserIdentity(nativeUserID string) Identity

	// NewGroupID returns the Obot ID for a group that SCIM creates.
	NewGroupID(groupIDPrefix, scimID string) string

	// NativeGroupID returns the identity provider's own ID of the group with the given Obot ID, or an empty string
	// when the Obot ID does not carry one, such as the ID of a group that SCIM created.
	NativeGroupID(groupIDPrefix, groupID string) string

	// GroupConsoleURL returns the address of a group in the identity provider's admin console, or an empty string
	// when it cannot be built from the issuer.
	GroupConsoleURL(issuer, nativeGroupID string) string

	// PatchRules returns how the identity provider's PATCH requests depart from RFC 7644.
	PatchRules() PatchRules
}

// DirectoryParameter is a configuration parameter that only login-time directory synchronization uses, with the
// descriptions the configuration form shows for it.
type DirectoryParameter struct {
	Name string
	// SetupDescription describes the parameter while the auth provider is being configured, when providing it
	// chooses directory synchronization and omitting it chooses SCIM.
	SetupDescription string
	// UnusedDescription describes the parameter once a SCIM connection has replaced directory synchronization.
	UnusedDescription string
}

// User holds the SCIM user attributes an adapter may read.
type User struct {
	UserName   string
	ExternalID string
}

// Identity holds the Obot user and identity fields for a user that SCIM creates.
type Identity struct {
	Username              string
	ProviderUsername      string
	ProviderUserID        string
	ProviderGroupLookupID string
}

// PatchRules holds how an identity provider's PATCH requests depart from RFC 7644. The SCIM endpoint accepts such a
// request only from the connections of that provider.
type PatchRules struct {
	// ReplaceAddsUnmatched makes a replace whose value filter selects no value do what an add does and create the
	// value, where RFC 7644 section 3.5.2.3 fails it with noTarget. Microsoft Entra ID sends a replace of
	// emails[type eq "work"].value for a user who has no work email.
	ReplaceAddsUnmatched bool
}

// InvalidUserError reports a SCIM user that lacks the attributes the adapter needs.
type InvalidUserError struct {
	Message string
}

func (e *InvalidUserError) Error() string {
	return e.Message
}

// ForAuthProvider returns the adapter of the named auth provider, and whether the provider supports SCIM.
func ForAuthProvider(authProviderName string) (Adapter, bool) {
	a, ok := byAuthProvider[authProviderName]
	return a, ok
}

// Lookup returns the adapter with the persisted type adapterType.
func Lookup(adapterType string) (Adapter, bool) {
	a, ok := byType[adapterType]
	return a, ok
}
