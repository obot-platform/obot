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
}
