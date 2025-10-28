//nolint:revive
package types

import "time"

type Identity struct {
	AuthProviderName      string    `json:"authProviderName" gorm:"primaryKey;index:idx_user_auth_id"`
	AuthProviderNamespace string    `json:"authProviderNamespace" gorm:"primaryKey;index:idx_user_auth_id"`
	ProviderUsername      string    `json:"providerUsername"`
	ProviderUserID        string    `json:"providerUserID"`
	HashedProviderUserID  string    `json:"hashedProviderUserID" gorm:"primaryKey"`
	Email                 string    `json:"email"`
	HashedEmail           string    `json:"hashedEmail"`
	UserID                uint      `json:"userID" gorm:"index:idx_user_auth_id"`
	IconURL               string    `json:"iconURL"`
	IconLastChecked       time.Time `json:"iconLastChecked"`
	Encrypted             bool      `json:"encrypted"`

	// AuthProviderGroupsLastChecked is the last time the identity's auth provider groups were checked.
	AuthProviderGroupsLastChecked time.Time `json:"authProviderGroupsLastChecked"`

	// AuthProviderGroups is the set of auth provider groups that the identity is a member of.
	AuthProviderGroups []Group `json:"groups" gorm:"-"`
}

func (i Identity) GetAuthProviderGroupIDs() []string {
	ids := make([]string, len(i.AuthProviderGroups))
	for i, group := range i.AuthProviderGroups {
		ids[i] = group.ID
	}

	return ids
}
