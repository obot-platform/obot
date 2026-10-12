package types

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/text/secure/precis"
)

const (
	// SCIMConnectionStateConnected means SCIM provisions users and groups, and has replaced login-time directory
	// synchronization, but users that SCIM has not provisioned can still sign in.
	SCIMConnectionStateConnected SCIMConnectionState = "connected"
	// SCIMConnectionStateEnforced means sign-in requires a SCIM binding.
	SCIMConnectionStateEnforced SCIMConnectionState = "enforced"

	// SCIMConnectionOriginSCIMFirst means the connection was created when its auth provider was configured without
	// directory credentials, so the provider never synchronized its directory at sign-in.
	SCIMConnectionOriginSCIMFirst SCIMConnectionOrigin = "scim_first"
	// SCIMConnectionOriginMigrated means the connection replaced the login-time directory synchronization of an auth
	// provider that was already configured.
	SCIMConnectionOriginMigrated SCIMConnectionOrigin = "migrated"

	// SCIMGroupBindingOriginExisting means the binding took over a group that existed before the identity provider
	// pushed it, keeping its ID.
	SCIMGroupBindingOriginExisting SCIMGroupBindingOrigin = "existing"
	// SCIMGroupBindingOriginCreated means SCIM created the group when the identity provider pushed it.
	SCIMGroupBindingOriginCreated SCIMGroupBindingOrigin = "created"

	SCIMResourceTypeUser  SCIMResourceType = "User"
	SCIMResourceTypeGroup SCIMResourceType = "Group"

	// SCIMTokenLifetime is how long a bearer token is accepted after it is issued. The identity provider keeps the
	// token for as long as provisioning runs, so this bounds how long a leaked token that nobody revoked keeps
	// working. A token must be rotated before it expires.
	SCIMTokenLifetime = 365 * 24 * time.Hour
)

// SCIMConnectionState is how far a SCIM connection has taken over its auth provider's directory. Both transitions,
// from no connection to connected and from connected to enforced, are permanent.
type SCIMConnectionState string

// SCIMConnectionOrigin is how a SCIM connection came to exist.
type SCIMConnectionOrigin string

// SCIMGroupBindingOrigin is whether a group binding took over an existing group or created one.
type SCIMGroupBindingOrigin string

// SCIMResourceType is the kind of resource a SCIM ID identifies.
type SCIMResourceType string

// SCIMConnection connects one auth provider to an identity provider's SCIM client. Once it exists, SCIM replaces
// login-time directory synchronization for that provider, permanently. The installation has at most one.
type SCIMConnection struct {
	// ID is the server-issued UUID of the connection, and the UID of its SCIM principal.
	ID        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// AdapterType selects the provider-specific SCIM rules. It is recorded when the connection is created and never
	// changes.
	AdapterType string               `json:"adapterType" gorm:"not null"`
	Origin      SCIMConnectionOrigin `json:"origin" gorm:"not null"`

	// AuthProviderNamespace and AuthProviderName are the exact auth provider whose users and groups this connection
	// manages.
	AuthProviderNamespace string `json:"authProviderNamespace" gorm:"not null;uniqueIndex:idx_scim_connections_auth_provider"`
	AuthProviderName      string `json:"authProviderName" gorm:"not null;uniqueIndex:idx_scim_connections_auth_provider"`

	// GroupIDPrefix is the auth provider's group ID prefix, which groups created through SCIM keep.
	GroupIDPrefix string `json:"groupIDPrefix" gorm:"not null"`

	// Issuer is the identity provider's issuer URL, recorded when the connection was created.
	Issuer string `json:"issuer"`

	State SCIMConnectionState `json:"state" gorm:"not null"`

	// TokenVerifier is a hash of the current bearer token, and PreviousTokenVerifier a hash of the token it replaced,
	// which is still accepted until PreviousTokenExpiresAt or until it is revoked. Both are empty for a connection
	// that has no token yet. Tokens themselves are never stored.
	TokenVerifier          string     `json:"-"`
	TokenIssuedAt          *time.Time `json:"tokenIssuedAt,omitempty"`
	PreviousTokenVerifier  string     `json:"-"`
	PreviousTokenExpiresAt *time.Time `json:"previousTokenExpiresAt,omitempty"`

	// EnabledAt is when SCIM replaced login-time directory synchronization for the provider, and EnforcedAt when
	// sign-in started requiring a SCIM binding.
	EnabledAt  time.Time  `json:"enabledAt"`
	EnforcedAt *time.Time `json:"enforcedAt,omitempty"`

	// LastRequestAt is when the last authenticated SCIM request arrived, and LastSuccessAt when the last one
	// succeeded. They are recorded at most every few seconds, and show activity, not that the identity provider and
	// Obot are synchronized.
	LastRequestAt *time.Time `json:"lastRequestAt,omitempty"`
	LastSuccessAt *time.Time `json:"lastSuccessAt,omitempty"`
}

// SCIMUserBinding binds a SCIM user to an Obot user. Its ID is a random (version 4) UUID, like every SCIM ID this
// server issues, so SCIM IDs are unique across resource types. A retired binding stays as a tombstone, so that
// group writes that still mention it can recognize it.
//
// Among a connection's unretired bindings, at most one has a given native user ID, at most one refers to a given
// Obot user, and at most one has a given case-folded userName.
//
// ExternalID, UserName, and Profile hold data from the identity provider and are encrypted like users are.
// HashedNativeUserID and HashedUserName support equality lookups without decrypting.
type SCIMUserBinding struct {
	// ID is the server-issued SCIM user ID. idx_scim_user_bindings_list serves the pages of a connection's users,
	// which are ordered by creation.
	ID           string `gorm:"primaryKey;index:idx_scim_user_bindings_list,priority:3,where:retired_at IS NULL"`
	ConnectionID string `gorm:"not null;index:idx_scim_user_bindings_native,unique,priority:1,where:retired_at IS NULL;index:idx_scim_user_bindings_user,unique,priority:1,where:retired_at IS NULL;index:idx_scim_user_bindings_user_name,unique,priority:1,where:retired_at IS NULL;index:idx_scim_user_bindings_list,priority:1"`
	UserID       uint   `gorm:"not null;index:idx_scim_user_bindings_user,unique,priority:2;index"`

	// HashedNativeUserID is the hash of the identity provider's immutable user ID, which is also the hashed provider
	// user ID of the user's identity.
	HashedNativeUserID string `gorm:"not null;index:idx_scim_user_bindings_native,unique,priority:2"`
	// HashedUserName is the hash of the SCIM userName's comparison key, which SCIMUserNameKey returns.
	HashedUserName string `gorm:"not null;index:idx_scim_user_bindings_user_name,unique,priority:2"`

	ExternalID string
	UserName   string
	// Profile is the JSON-encoded SCIMUserProfile.
	Profile   string
	Encrypted bool `gorm:"not null;default:false"`

	// Active is the provisioned state the identity provider last sent.
	Active bool `gorm:"not null;default:false"`

	// Revision counts the changes to the binding.
	Revision  int64     `gorm:"not null;default:0"`
	CreatedAt time.Time `gorm:"index:idx_scim_user_bindings_list,priority:2"`
	UpdatedAt time.Time
	RetiredAt *time.Time `gorm:"index"`
}

// SCIMGroupBinding binds a SCIM group to an Obot group. Its ID is a random (version 4) UUID. A retired binding stays
// as a tombstone. The Obot group itself, and every reference to it, outlives the binding.
//
// Each group has at most one unretired binding, and the normalized display names of a connection's unretired
// bindings are unique.
type SCIMGroupBinding struct {
	// ID is the server-issued SCIM group ID.
	ID           string `gorm:"primaryKey"`
	ConnectionID string `gorm:"not null;index:idx_scim_group_bindings_name,unique,priority:1,where:retired_at IS NULL;index"`
	// GroupID is the ID of the Obot group, which never changes.
	GroupID string `gorm:"not null;index:idx_scim_group_bindings_group,unique,where:retired_at IS NULL;index"`
	// NormalizedDisplayName is the trimmed, case-folded display name.
	NormalizedDisplayName string                 `gorm:"not null;index:idx_scim_group_bindings_name,unique,priority:2"`
	Origin                SCIMGroupBindingOrigin `gorm:"not null"`

	// Revision counts the changes to the group and its memberships made through SCIM.
	Revision  int64 `gorm:"not null;default:0"`
	CreatedAt time.Time
	UpdatedAt time.Time
	RetiredAt *time.Time `gorm:"index"`
}

// SCIMPendingGroupDeletion marks a group that was selected for deletion because nothing references it. The mark is
// committed before the group is deleted, so that writers of group references can refuse new references to the
// group while its deletion completes. A mark expires a while after CreatedAt, by the database's clock, so that one a
// deletion left behind does not refuse references forever; the deletion then deletes nothing.
type SCIMPendingGroupDeletion struct {
	GroupID      string `gorm:"primaryKey"`
	ConnectionID string `gorm:"not null;index"`
	// RunID identifies the deletion that placed the mark. A deletion deletes only the groups it marked itself, and
	// read the references of after marking them.
	RunID     string `gorm:"not null;default:''"`
	CreatedAt time.Time
}

// SCIMGroupSubjectCleanup records a group that a SCIM DELETE deleted while SCIM was enforced, whose subjects in access
// policies are still to be removed. The policies live in the controller store, which cannot share the transaction
// that deleted the group.
type SCIMGroupSubjectCleanup struct {
	GroupID string `gorm:"primaryKey"`
	// Namespace holds the access policies whose subjects are removed.
	Namespace string `gorm:"not null"`
	CreatedAt time.Time
	// ClaimedUntil is when a replica's claim to run the cleanup expires. A failed cleanup keeps its claim, so this is
	// also when it is retried.
	ClaimedUntil *time.Time
	Attempts     int `gorm:"not null;default:0"`
	LastError    string
}

// SCIMReferenceWrite records a write of new group references that is in progress. A deletion of unreferenced groups
// waits for the writes recorded when its marks committed, so that the references it reads afterwards include
// theirs, and every later write sees the marks. A write that never finished stops counting once it expires.
type SCIMReferenceWrite struct {
	ID        string    `gorm:"primaryKey"`
	ExpiresAt time.Time `gorm:"not null;index"`
}

// SCIMRequestFailure records an authenticated SCIM request that failed, so administrators can see why provisioning
// tasks fail in the identity provider. Only the most recent failures of each connection are kept.
//
// Detail can echo values from the request, such as a userName, so it is encrypted like users are.
type SCIMRequestFailure struct {
	ID           uint   `gorm:"primaryKey"`
	ConnectionID string `gorm:"not null;index"`
	CreatedAt    time.Time
	Method       string
	// Resource is the request path below the connection's base URL.
	Resource  string
	Status    int
	SCIMType  string
	Detail    string
	Encrypted bool `gorm:"not null;default:false"`
}

// SCIMUserProfile holds the supported SCIM user attributes that Obot's User cannot represent. The primary email and
// the display name are also projected onto the User.
type SCIMUserProfile struct {
	Name              *SCIMName        `json:"name,omitempty"`
	DisplayName       string           `json:"displayName,omitempty"`
	NickName          string           `json:"nickName,omitempty"`
	ProfileURL        string           `json:"profileUrl,omitempty"`
	Title             string           `json:"title,omitempty"`
	UserType          string           `json:"userType,omitempty"`
	PreferredLanguage string           `json:"preferredLanguage,omitempty"`
	Locale            string           `json:"locale,omitempty"`
	Timezone          string           `json:"timezone,omitempty"`
	Emails            []SCIMMultiValue `json:"emails,omitempty"`
	PhoneNumbers      []SCIMMultiValue `json:"phoneNumbers,omitempty"`
}

// SCIMName is the SCIM user's structured name.
type SCIMName struct {
	Formatted       string `json:"formatted,omitempty"`
	FamilyName      string `json:"familyName,omitempty"`
	GivenName       string `json:"givenName,omitempty"`
	MiddleName      string `json:"middleName,omitempty"`
	HonorificPrefix string `json:"honorificPrefix,omitempty"`
	HonorificSuffix string `json:"honorificSuffix,omitempty"`
}

// SCIMMultiValue is one value of a multi-valued SCIM attribute such as emails.
type SCIMMultiValue struct {
	Value   string `json:"value,omitempty"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
	Primary bool   `json:"primary,omitempty"`
}

// HasToken reports whether the connection has a current bearer token. A connection without one answers every request
// with 401.
func (c SCIMConnection) HasToken() bool {
	return c.TokenVerifier != ""
}

// TokenExpiresAt returns when the current bearer token stops being accepted, or nil when the connection has none.
func (c SCIMConnection) TokenExpiresAt() *time.Time {
	if !c.HasToken() || c.TokenIssuedAt == nil {
		return nil
	}
	expiresAt := c.TokenIssuedAt.Add(SCIMTokenLifetime)
	return &expiresAt
}

// TokenAccepted reports whether the current bearer token is still accepted.
func (c SCIMConnection) TokenAccepted(now time.Time) bool {
	expiresAt := c.TokenExpiresAt()
	return expiresAt != nil && now.Before(*expiresAt)
}

// PreviousTokenAccepted reports whether the token that the last rotation replaced is still accepted.
func (c SCIMConnection) PreviousTokenAccepted(now time.Time) bool {
	return c.PreviousTokenVerifier != "" && c.PreviousTokenExpiresAt != nil && now.Before(*c.PreviousTokenExpiresAt)
}

// SCIMUserNameKey returns the key that SCIM userNames are compared and checked for uniqueness by, as RFC 7644 section 5
// requires: the PRECIS UsernameCaseMapped profile of RFC 8265, applied to each whitespace-separated part of the
// userName, which maps full-width characters and case, and normalizes to NFC. It returns an error for a userName that
// the profile does not allow, such as one that is empty or has control characters.
func SCIMUserNameKey(userName string) (string, error) {
	parts := strings.Fields(userName)
	if len(parts) == 0 {
		return "", errors.New("userName is empty")
	}
	for i, part := range parts {
		key, err := precis.UsernameCaseMapped.CompareKey(part)
		if err != nil {
			return "", err
		}
		parts[i] = key
	}
	return strings.Join(parts, " "), nil
}

// NormalizeSCIMGroupName trims and case-folds a SCIM group display name for comparison.
func NormalizeSCIMGroupName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// PrimaryEmail returns the email marked primary, or the first email when none is.
func (p SCIMUserProfile) PrimaryEmail() string {
	for _, email := range p.Emails {
		if email.Primary && email.Value != "" {
			return email.Value
		}
	}
	for _, email := range p.Emails {
		if email.Value != "" {
			return email.Value
		}
	}
	return ""
}

// ObotDisplayName returns the name to show for the user in Obot: the display name, then the formatted name, then the
// given and family names. It is empty when the profile has none of them.
func (p SCIMUserProfile) ObotDisplayName() string {
	if name := strings.TrimSpace(p.DisplayName); name != "" {
		return name
	}
	if p.Name == nil {
		return ""
	}
	if name := strings.TrimSpace(p.Name.Formatted); name != "" {
		return name
	}
	return strings.TrimSpace(strings.Join([]string{strings.TrimSpace(p.Name.GivenName), strings.TrimSpace(p.Name.FamilyName)}, " "))
}

// Retired reports whether the binding has been retired.
func (b SCIMUserBinding) Retired() bool {
	return b.RetiredAt != nil
}

// Retired reports whether the binding has been retired.
func (b SCIMGroupBinding) Retired() bool {
	return b.RetiredAt != nil
}
