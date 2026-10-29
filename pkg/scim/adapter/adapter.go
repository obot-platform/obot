// Package adapter holds the provider-specific rules of SCIM connections.
//
// Shared SCIM code never infers provider behavior from names, prefixes, or identifier shapes. The registry maps an
// auth provider name to its adapter when a connection is created, and the connection persists the adapter's type,
// which selects the provider-specific rules from then on. Supporting another provider means registering another
// adapter.
package adapter

const (
	// pendingProviderUserIDPrefix starts the provider user ID of an identity that waits for its first sign-in.
	pendingProviderUserIDPrefix = "OBOT_SCIM_PENDING/"

	// UserKeyProviderUserID matches the identity's hashed provider user ID, which sign-in keys identities by.
	UserKeyProviderUserID UserKey = 0
	// UserKeyGroupLookupID matches the identity's hashed group lookup ID, for an identity provider whose sign-in
	// subject is not an ID that its SCIM client can send.
	UserKeyGroupLookupID UserKey = 1

	// GroupKeyDisplayName binds a pushed group to the unbound group with the same normalized name.
	GroupKeyDisplayName GroupKey = 0
	// GroupKeyExternalID binds a pushed group to the group whose ID carries the native group ID that its externalId
	// holds.
	GroupKeyExternalID GroupKey = 1
)

var (
	// byAuthProvider maps the name of each auth provider that supports SCIM to its adapter. Looking an auth provider
	// up here is the only place where shared code reads a provider name.
	byAuthProvider = map[string]Adapter{
		oktaAuthProviderName:     okta{},
		oneLoginAuthProviderName: oneLogin{},
		entraAuthProviderName:    entra{},
	}

	// byType maps each adapter's persisted type to the adapter.
	byType = map[string]Adapter{
		oktaAdapterType:     okta{},
		oneLoginAdapterType: oneLogin{},
		entraAdapterType:    entra{},
	}
)

// UserKey is which identity column a SCIM user's native ID matches.
type UserKey int

// GroupKey is how a pushed group finds an existing Obot group.
type GroupKey int

// Adapter holds the rules that differ between identity providers.
type Adapter interface {
	// Type is the adapter's persisted type.
	Type() string

	// DirectoryParameters are the auth provider configuration parameters that only login-time directory
	// synchronization uses.
	DirectoryParameters() []DirectoryParameter

	// SCIMChoiceParameter returns the parameter that chooses SCIM-first setup, for an auth provider without directory
	// parameters. Its Name is empty when omitting the directory parameters chooses SCIM.
	SCIMChoiceParameter() SCIMChoiceParameter

	// IssuerConfigurationParameter names the auth provider configuration parameter that holds the identity
	// provider's issuer URL.
	IssuerConfigurationParameter() string

	// NativeUserID returns the identity provider's immutable ID for a SCIM user. A user binds to the existing
	// identity of the connection's auth provider that stores the same ID.
	NativeUserID(user User) (string, error)

	// NativeUserIDFromExternalID returns the native user ID that a SCIM user's externalId holds, as NativeUserID would
	// for a user with that externalId, without the checks of other attributes. It answers externalId filters. It
	// returns an *InvalidUserError for an externalId that holds none.
	NativeUserIDFromExternalID(externalID string) (string, error)

	// UserKey returns which identity column a SCIM user's native ID matches.
	UserKey() UserKey

	// NewUserIdentity returns the Obot username and identity fields for a user that SCIM creates, so that the user's
	// later sign-ins find the same identity.
	NewUserIdentity(nativeUserID string) Identity

	// GroupKey returns how a pushed group finds an existing Obot group.
	GroupKey() GroupKey

	// NativeGroupIDFromExternalID validates a pushed group's externalId and returns the native group ID it holds, for
	// GroupKeyExternalID. It returns an *InvalidGroupError for an externalId that holds none.
	NativeGroupIDFromExternalID(externalID string) (string, error)

	// NewGroupID returns the Obot ID for a group that SCIM creates. nativeGroupID is the native group ID that
	// NativeGroupIDFromExternalID returned, for GroupKeyExternalID, and empty otherwise. For GroupKeyExternalID, the
	// ID depends on nativeGroupID alone, so that a pushed group's ID can be found from its externalId.
	NewGroupID(groupIDPrefix, scimID, nativeGroupID string) string

	// NativeGroupID returns the identity provider's own ID of the group with the given Obot ID, or an empty string
	// when the Obot ID does not carry one, such as the ID of a group that SCIM created.
	NativeGroupID(groupIDPrefix, groupID string) string

	// GroupConsoleURL returns the address of a group in the identity provider's admin console, or an empty string
	// when it cannot be built from the issuer.
	GroupConsoleURL(issuer, nativeGroupID string) string

	// PatchRules returns how the identity provider's PATCH requests depart from RFC 7644.
	PatchRules() PatchRules

	// Rules returns the identity provider's choices among shared behaviors.
	Rules() Rules

	// ErrorHints returns the provider-specific text that the details of some SCIM errors carry.
	ErrorHints() ErrorHints
}

// DirectoryParameter is a configuration parameter that only login-time directory synchronization uses, with the
// descriptions the configuration form shows for it.
type DirectoryParameter struct {
	Name string
	// SetupDescription describes the parameter while the auth provider is being configured, when providing it
	// chooses directory synchronization and omitting it chooses SCIM.
	SetupDescription  string
	SetupTranslations map[string]string
	// UnusedDescription describes the parameter once a SCIM connection has replaced directory synchronization.
	UnusedDescription  string
	UnusedTranslations map[string]string
}

// SCIMChoiceParameter is a configuration parameter that chooses SCIM-first setup when it is "true", with the
// description the configuration form shows for it. Obot reads it and drops it, so it is never stored, and the auth
// provider never sees it.
type SCIMChoiceParameter struct {
	Name         string
	Description  string
	Translations map[string]string
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

// Rules holds an identity provider's choices among shared behaviors, which take no arguments.
type Rules struct {
	// DeletesUsers makes DELETE /Users/{id} delete the user and their data, instead of refusing it with 501.
	DeletesUsers bool
}

// ErrorHints holds provider-specific text for SCIM error details. An identity provider that shows a failed request's
// detail to its admin, and never retries the request on its own, needs the detail to say what to change in the
// identity provider and how to recover. An empty field leaves the shared detail as it is. A failure whose cause the
// adapter itself finds, such as a user without the attributes it needs, says so in its InvalidUserError instead.
type ErrorHints struct {
	// Retry is appended to the detail of a failure that may succeed when the request is retried, such as a request
	// that took too long or arrived while the endpoint was unavailable.
	Retry string
	// UserDelete replaces the detail of the refusal of a user DELETE, for an adapter whose Rules do not delete users.
	// The shared detail asks for a deactivation instead. An identity provider that sends a DELETE unless it is set up
	// otherwise names the setting here.
	UserDelete string
	// ValuelessFilteredAdd is appended to the detail of the refusal of an add whose path has a value filter and that
	// has no value, as in {"op":"add","path":"members[value eq \"id\"]"}.
	ValuelessFilteredAdd string
	// ExternalIDChanged is appended to the detail of the refusal of a change to a provisioned user's externalId, which
	// the user is bound by.
	ExternalIDChanged string
}

// InvalidUserError reports a SCIM user that lacks the attributes the adapter needs.
type InvalidUserError struct {
	Message string
}

// InvalidGroupError reports a pushed group whose externalId holds no native group ID.
type InvalidGroupError struct {
	Message string
}

func (e *InvalidUserError) Error() string {
	return e.Message
}

func (e *InvalidGroupError) Error() string {
	return e.Message
}

// PendingProviderUserID returns the provider user ID of an identity that SCIM creates for an adapter with
// UserKeyGroupLookupID, whose sign-in subject SCIM does not know. The user's first sign-in finds the identity by it, and
// replaces it with the subject.
func PendingProviderUserID(nativeUserID string) string {
	return pendingProviderUserIDPrefix + nativeUserID
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
