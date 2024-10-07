package types

type Identity struct {
	AuthProviderID   uint   `json:"authProviderID" gorm:"primaryKey;index:idx_user_auth_id"`
	ProviderUsername string `json:"providerUsername" gorm:"primaryKey"`
	Email            string `json:"email"`
	UserID           uint   `json:"userID" gorm:"index:idx_user_auth_id"`
}
