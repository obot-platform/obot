package types

import "time"

// Group represents a group that users can belong to in an auth provider.
type Group struct {
	ID                    string    `json:"id" gorm:"primaryKey;unique"`
	AuthProviderName      string    `json:"authProviderName" gorm:"primaryKey;index:idx_group_auth_provider"`
	AuthProviderNamespace string    `json:"authProviderNamespace" gorm:"primaryKey;index:idx_group_auth_provider"`
	Name                  string    `json:"name"`
	IconURL               *string   `json:"iconURL"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}
