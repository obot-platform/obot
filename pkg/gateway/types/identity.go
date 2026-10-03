//nolint:revive
package types

import (
	"time"
)

type Identity struct {
	AuthProviderName      string    `json:"authProviderName" gorm:"primaryKey;index:idx_user_auth_id"`
	AuthProviderNamespace string    `json:"authProviderNamespace" gorm:"primaryKey;index:idx_user_auth_id"`
	ProviderUsername      string    `json:"providerUsername"`
	ProviderUserID        string    `json:"providerUserID"`
	HashedProviderUserID  string    `json:"hashedProviderUserID" gorm:"primaryKey"`
	ProviderGroupLookupID string    `json:"providerGroupLookupID"`
	Email                 string    `json:"email"`
	HashedEmail           string    `json:"hashedEmail"`
	UserID                uint      `json:"userID" gorm:"index:idx_user_auth_id"`
	IconURL               string    `json:"iconURL"`
	IconLastChecked       time.Time `json:"iconLastChecked"`
	Encrypted             bool      `json:"encrypted"`

	// AuthProviderGroupsLastChecked is the last time the identity's auth provider groups were checked.
	AuthProviderGroupsLastChecked time.Time `json:"authProviderGroupsLastChecked"`

	// FirstSignInAt is when the identity first signed in through its auth provider. It is unset for an
	// identity that was created before anyone signed in with it, so an identity alone does not prove that its
	// user can sign in. The first sign-in of a user who is not disabled sets it, and nothing clears it.
	FirstSignInAt *time.Time `json:"firstSignInAt,omitempty"`

	// AuthProviderGroups is the set of auth provider groups that the identity is a member of.
	AuthProviderGroups []Group `json:"groups" gorm:"-"`
}

func (i Identity) GroupLookupID() string {
	if i.ProviderGroupLookupID != "" {
		return i.ProviderGroupLookupID
	}
	return i.ProviderUserID
}

func (i Identity) GetAuthProviderGroupIDs() []string {
	ids := make([]string, len(i.AuthProviderGroups))
	for i, group := range i.AuthProviderGroups {
		ids[i] = group.ID
	}

	return ids
}
