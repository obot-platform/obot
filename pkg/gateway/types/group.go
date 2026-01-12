//nolint:revive
package types

import "time"

// Group represents a group that users can belong to in an auth provider.
type Group struct {
	// ID is the globally unique identifier for the group.
	// Each auth provider should use a different prefix for their groups to avoid collisions with other providers.
	ID string `json:"id" gorm:"primaryKey;unique"`

	// AuthProviderName is the name of the auth provider that the group belongs to.
	// This is used to identify the auth provider that the group belongs to.
	AuthProviderName string `json:"authProviderName" gorm:"primaryKey;index:idx_group_auth_provider"`

	// AuthProviderNamespace is the namespace of the auth provider that the group belongs to.
	// Note: This is pretty much always "default", but we're keeping it here for parity with the Identity type.
	AuthProviderNamespace string `json:"authProviderNamespace" gorm:"primaryKey;index:idx_group_auth_provider"`

	// Name is the display name of the group.
	Name string `json:"name"`

	// Description is the optional description of the group.
	Description *string `json:"description"`

	// IconURL is the URL of the group's icon.
	IconURL *string `json:"iconURL"`
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
