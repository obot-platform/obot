//nolint:revive
package types

import "time"

// Group represents a group that users can belong to in an auth provider.
type Group struct {
	// ID is the globally unique identifier for the group.
	// Each auth provider should use a different prefix for their groups to avoid collisions with other providers.
	ID string `json:"id" gorm:"primaryKey;unique;index:idx_group_auth_provider_name,priority:4"`

	// AuthProviderName is the name of the auth provider that the group belongs to.
	// This is used to identify the auth provider that the group belongs to.
	AuthProviderName string `json:"authProviderName" gorm:"primaryKey;index:idx_group_auth_provider;index:idx_group_auth_provider_name,priority:1"`

	// AuthProviderNamespace is the namespace of the auth provider that the group belongs to.
	// Note: This is pretty much always "default", but we're keeping it here for parity with the Identity type.
	AuthProviderNamespace string `json:"authProviderNamespace" gorm:"primaryKey;index:idx_group_auth_provider;index:idx_group_auth_provider_name,priority:2"`

	// Name is the display name of the group.
	// Indexed because cached group listings are paged by keyset in (name, id) order.
	Name string `json:"name" gorm:"index:idx_group_auth_provider_name,priority:3"`

	// IconURL is the URL of the group's icon.
	IconURL *string `json:"iconURL"`
}

// Group listing sources, reported on GroupListResponse so callers can tell a live directory
// listing apart from the partial view built up from previous sign-ins.
const (
	// GroupSourceProvider means the listing came from the auth provider and is complete.
	GroupSourceProvider = "provider"

	// GroupSourceCache means the listing came from the groups table, which only ever contains
	// groups observed during a user sign-in and is therefore partial.
	GroupSourceCache = "cache"
)

// GroupListResponse is the paginated response for a group listing.
type GroupListResponse struct {
	Items []Group `json:"items"`

	// NextCursor is the opaque position to pass back to fetch the following page. It is absent on
	// the last page, and its presence is the only indication that more results exist.
	NextCursor string `json:"nextCursor,omitempty"`

	// Source is where the items came from: GroupSourceProvider or GroupSourceCache.
	Source string `json:"source"`

	// Degraded is true when the auth provider could not be listed and the response fell back to
	// cached groups. It distinguishes a real failure from a provider that has no group support.
	Degraded bool `json:"degraded"`
}

// GroupMemberships represents a user's membership in a group.
type GroupMemberships struct {
	// UserID is the ID of the user that is a member of the group.
	UserID uint `json:"userID" gorm:"primaryKey"`

	// GroupID is the globally unique identifier for the group.
	GroupID string `json:"groupID" gorm:"primaryKey"`

	// CreatedAt is when the group membership was created.
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime"`
}
