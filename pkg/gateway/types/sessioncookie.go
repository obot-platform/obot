//nolint:revive
package types

type SessionCookie struct {
	HashedSessionID       string `json:"-" gorm:"primaryKey"`
	AuthProviderNamespace string `json:"-" gorm:"primaryKey"`
	AuthProviderName      string `json:"-" gorm:"primaryKey"`
	UserID                uint   `json:"-" gorm:"index"`
	Cookie                string
	Encrypted             bool
}
