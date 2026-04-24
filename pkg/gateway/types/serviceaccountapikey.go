//nolint:revive
package types

import "time"

type ServiceAccountAPIKey struct {
	ID                 uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	ServiceAccountName string     `json:"serviceAccountName" gorm:"index"`
	HashedToken        string     `json:"-" gorm:"index"`
	CreatedAt          time.Time  `json:"createdAt"`
	ValidAfter         time.Time  `json:"validAfter" gorm:"index"`
	RetireAfter        *time.Time `json:"retireAfter,omitempty" gorm:"index"`
}

type ServiceAccountAPIKeyCreateResponse struct {
	ServiceAccountAPIKey
	Token string `json:"token"`
}
