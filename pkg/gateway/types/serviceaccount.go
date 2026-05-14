package types

import "time"

type ServiceAccountAPIKey struct {
	ID                 uint       `gorm:"primaryKey;autoIncrement"`
	ServiceAccountName string     `gorm:"index;not null"`
	HashedSecret       string     `json:"-"`
	Token              string     `gorm:"-" json:"-"`
	CreatedAt          time.Time  `json:"createdAt"`
	ValidAfter         time.Time  `json:"validAfter"`
	RetireAfter        *time.Time `json:"retireAfter,omitempty"`
}

func (k *ServiceAccountAPIKey) PlaintextToken() string {
	if k == nil {
		return ""
	}
	return k.Token
}
