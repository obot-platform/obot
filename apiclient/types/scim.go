package types

// SCIMConnection connects an auth provider to an identity provider's SCIM client.
type SCIMConnection struct {
	ID          string `json:"id"`
	AdapterType string `json:"adapterType"`
	// Origin is "scim_first" for a connection created when its auth provider was configured without directory
	// credentials, or "migrated" for one that replaced login-time directory synchronization.
	Origin                  string `json:"origin"`
	AuthProviderNamespace   string `json:"authProviderNamespace"`
	AuthProviderName        string `json:"authProviderName"`
	AuthProviderDisplayName string `json:"authProviderDisplayName"`
	// State is "connected" or "enforced".
	State string `json:"state"`
	// BaseURL is the SCIM base URL to configure in the identity provider.
	BaseURL    string `json:"baseURL"`
	Issuer     string `json:"issuer,omitempty"`
	EnabledAt  Time   `json:"enabledAt"`
	EnforcedAt *Time  `json:"enforcedAt,omitempty"`
	// HasToken is false until the first bearer token is issued. Every request to a connection without one fails.
	HasToken bool `json:"hasToken"`
	// TokenIssuedAt is when the current bearer token was issued, and TokenExpiresAt when it stops being accepted,
	// a year later. A token must be rotated before it expires.
	TokenIssuedAt  *Time `json:"tokenIssuedAt,omitempty"`
	TokenExpiresAt *Time `json:"tokenExpiresAt,omitempty"`
	// PreviousTokenAccepted is true while the token that the last rotation replaced is still accepted, which it
	// is until PreviousTokenExpiresAt unless it is revoked first.
	PreviousTokenAccepted  bool  `json:"previousTokenAccepted"`
	PreviousTokenExpiresAt *Time `json:"previousTokenExpiresAt,omitempty"`
	// AuthProviderConfigured is false while the connection's auth provider is not the one serving sign-ins,
	// such as while it is only staged. The SCIM endpoint then answers 503, and no token is issued.
	AuthProviderConfigured bool `json:"authProviderConfigured"`
	// Token is the bearer token. It is set only in the response that issued it, and cannot be retrieved again.
	Token string `json:"token,omitempty"`
}

type SCIMConnectionList List[SCIMConnection]

// GroupReference is an object that references a group, which makes the group carry authorization.
type GroupReference struct {
	// Kind is accessControlRule, modelAccessPolicy, skillAccessRule, messagePolicy, hostedAgentAccessRule,
	// publishedArtifact, groupRoleAssignment, or vmcpProfile.
	Kind string `json:"kind"`
	// ID is the object's ID, or the group ID for a group role assignment.
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	// Detail says where in the object the reference is, such as a published artifact's version, a virtual MCP
	// server's profile, or the role a group role assignment grants.
	Detail string `json:"detail,omitempty"`
}

// SCIMSetupGroup is a group of an auth provider that SCIM manages or will manage.
type SCIMSetupGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// NativeID is the identity provider's own ID of the group, when the group ID carries one.
	NativeID string `json:"nativeID,omitempty"`
	// ConsoleURL is the address of the group in the identity provider's admin console, when it can be built.
	ConsoleURL string `json:"consoleURL,omitempty"`
	// SCIMID is the ID of the group's SCIM binding. It is empty while the group is unbound.
	SCIMID     string           `json:"scimID,omitempty"`
	References []GroupReference `json:"references,omitempty"`
}

// SCIMSetupGroupPage is one page of a list of groups.
type SCIMSetupGroupPage struct {
	Items []SCIMSetupGroup `json:"items"`
	Total int64            `json:"total"`
}

// SCIMSetupWarning is a finding that does not block SCIM, but needs the administrator's attention.
type SCIMSetupWarning struct {
	// Type is "everyoneGroup" for a referenced group that the identity provider cannot push, "missingGroup" for
	// references to a group ID of the auth provider that no group has, "duplicateName" for a name that more than one
	// unbound referenced group has, or "unreferencedNamesake" for an unreferenced group with the name of an unbound
	// referenced group. A group pushed under the name of either of the last two binds to no group.
	Type       string           `json:"type"`
	Message    string           `json:"message"`
	GroupID    string           `json:"groupID"`
	GroupName  string           `json:"groupName,omitempty"`
	References []GroupReference `json:"references,omitempty"`
}

// SCIMSetupUser is a user of a SCIM connection's auth provider.
type SCIMSetupUser struct {
	ID          string `json:"id"`
	Username    string `json:"username,omitempty"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	// Status is active or disabled.
	Status         UserStatus `json:"status"`
	DisabledReason string     `json:"disabledReason,omitempty"`
	// SCIMID is the ID of the user's SCIM binding. It is empty while the user is unprovisioned.
	SCIMID string `json:"scimID,omitempty"`
	// Active is the provisioned state the identity provider last sent. It is only set for provisioned users.
	Active bool `json:"active,omitempty"`
	// SignedIn is true once the user has signed in through the auth provider.
	SignedIn bool `json:"signedIn,omitempty"`
}

// SCIMSetupUserPage is one page of a list of users.
type SCIMSetupUserPage struct {
	Items []SCIMSetupUser `json:"items"`
	Total int64           `json:"total"`
}

// SCIMRequestFailure is an authenticated SCIM request that failed.
type SCIMRequestFailure struct {
	Time   Time   `json:"time"`
	Method string `json:"method"`
	// Resource is the request path below the SCIM base URL.
	Resource string `json:"resource"`
	Status   int    `json:"status"`
	SCIMType string `json:"scimType,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// SCIMRequestFailurePage is one page of a connection's recent failed requests, newest first.
type SCIMRequestFailurePage struct {
	Items []SCIMRequestFailure `json:"items"`
	Total int64                `json:"total"`
}

// SCIMConnectionActivity shows the SCIM requests a connection received. Activity is not proof that the identity
// provider and Obot are synchronized.
type SCIMConnectionActivity struct {
	// LastRequestAt is when the last authenticated request arrived, and LastSuccessAt when the last one
	// succeeded. Both are recorded at most every few seconds.
	LastRequestAt  *Time                  `json:"lastRequestAt,omitempty"`
	LastSuccessAt  *Time                  `json:"lastSuccessAt,omitempty"`
	RecentFailures SCIMRequestFailurePage `json:"recentFailures"`
}

// SCIMConnectionReview is the state of a SCIM connection, and what enforcing it would do. Each list holds its first
// page; the connection's list routes serve the others.
type SCIMConnectionReview struct {
	Connection SCIMConnection `json:"connection"`
	// ProvisionedUsers are the users SCIM has provisioned.
	ProvisionedUsers SCIMSetupUserPage `json:"provisionedUsers"`
	// UnprovisionedUsers are the users of the auth provider that SCIM has not provisioned. Enforcing disables them.
	UnprovisionedUsers SCIMSetupUserPage `json:"unprovisionedUsers"`
	// BoundGroups are the groups that SCIM manages.
	BoundGroups SCIMSetupGroupPage `json:"boundGroups"`
	// UnboundReferencedGroups are referenced groups that the identity provider has not pushed. They block
	// enforcing.
	UnboundReferencedGroups SCIMSetupGroupPage `json:"unboundReferencedGroups"`
	// UnreferencedGroups are unbound groups that nothing references. Enforcing deletes them.
	UnreferencedGroups SCIMSetupGroupPage `json:"unreferencedGroups"`
	Warnings           []SCIMSetupWarning `json:"warnings"`
	// EnforceBlockers are the reasons the requesting user cannot enforce SCIM now, other than
	// UnboundReferencedGroups, which also block it. They are empty once SCIM is enforced.
	EnforceBlockers []string               `json:"enforceBlockers"`
	Activity        SCIMConnectionActivity `json:"activity"`
}

// SCIMDuplicateGroupName is a name that more than one referenced group of an auth provider has, after
// normalization. The identity provider pushes groups by name, so a pushed group binds to neither.
type SCIMDuplicateGroupName struct {
	Name   string           `json:"name"`
	Groups []SCIMSetupGroup `json:"groups"`
}

// SCIMEnablePreview names the configured auth provider that SCIM can be enabled for, and what blocks enabling it.
// Enabling applies only to a configured auth provider that supports SCIM and synchronizes its directory at sign-in.
type SCIMEnablePreview struct {
	// AuthProviderNamespace, AuthProviderName, and AuthProviderDisplayName name the configured auth provider. They
	// are empty when no auth provider is configured, or the configured one does not support SCIM.
	AuthProviderNamespace   string `json:"authProviderNamespace,omitempty"`
	AuthProviderName        string `json:"authProviderName,omitempty"`
	AuthProviderDisplayName string `json:"authProviderDisplayName,omitempty"`
	// Blockers are the reasons SCIM cannot be enabled now, including one for each duplicate group name.
	Blockers []string `json:"blockers"`
	// DuplicateGroupNames are the names that more than one referenced group has. They block enabling.
	DuplicateGroupNames []SCIMDuplicateGroupName `json:"duplicateGroupNames"`
}

// SCIMEnableResult is what enabling SCIM did.
type SCIMEnableResult struct {
	// Connection carries the bearer token, which is shown only in this response.
	Connection SCIMConnection `json:"connection"`
	// DeletedGroupCount is the number of groups deleted because nothing referenced them.
	DeletedGroupCount int `json:"deletedGroupCount"`
	// DeletionError is set when the groups that nothing references could not be deleted. SCIM is enabled
	// regardless, and the deletion can be retried.
	DeletionError string `json:"deletionError,omitempty"`
}

// SCIMGroupDeletionResult is what a deletion of the unbound groups that nothing references did.
type SCIMGroupDeletionResult struct {
	// DeletedGroupCount is the number of groups deleted because nothing referenced them.
	DeletedGroupCount int `json:"deletedGroupCount"`
}

// SCIMEnforceResult is what enforcing SCIM did.
type SCIMEnforceResult struct {
	Connection SCIMConnection `json:"connection"`
	// DisabledUserCount is the number of users disabled because SCIM never provisioned them.
	DisabledUserCount int `json:"disabledUserCount"`
	// DeletedGroupCount is the number of unbound groups deleted because nothing referenced them.
	DeletedGroupCount int `json:"deletedGroupCount"`
}

// ResidualGroupData is what remains of an auth provider's groups from an earlier configuration. It blocks
// configuring the provider without directory credentials until the provider's auth provider cleanup removes it.
type ResidualGroupData struct {
	// Groups are the provider's groups, and the group IDs with its group ID prefix that no group has but something
	// references, each with the objects that reference it. A group ID that no group has has no name.
	Groups []SCIMSetupGroup `json:"groups"`
	// MembershipCount is the number of memberships in groups with the provider's group ID prefix.
	MembershipCount int64 `json:"membershipCount"`
}
