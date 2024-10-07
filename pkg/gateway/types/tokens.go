package types

import "time"

type AuthToken struct {
	ID             string    `json:"id" gorm:"index:idx_id_hashed_token"`
	UserID         uint      `json:"-" gorm:"index"`
	AuthProviderID uint      `json:"-" gorm:"index"`
	HashedToken    string    `json:"-" gorm:"index:idx_id_hashed_token"`
	CreatedAt      time.Time `json:"createdAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type TokenRequest struct {
	ID                    string `gorm:"primaryKey"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
	State                 string `gorm:"index"`
	Nonce                 string
	Token                 string
	ExpiresAt             time.Time
	CompletionRedirectURL string
	Error                 string
	TokenRetrieved        bool
}
