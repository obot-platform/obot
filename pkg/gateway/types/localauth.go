//nolint:revive
package types

import "time"

// LocalAuthUser is a username/password user managed by the local auth provider.
// The email address is the login name and is also what identifies the user to the rest of Obot,
// so it is immutable: to change it, delete the user and create a new one.
type LocalAuthUser struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	Email        string    `json:"email"`
	HashedEmail  string    `json:"-" gorm:"uniqueIndex"`
	PasswordHash string    `json:"-"`
	Encrypted    bool      `json:"-"`
}

// LocalAuthSession is a login session created by the local auth provider.
// ID is the SHA-256 hash of the session token that is handed to the browser,
// so a database leak does not hand out usable sessions.
type LocalAuthSession struct {
	ID        string    `json:"-" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt" gorm:"index"`
	UserID    uint      `json:"userID" gorm:"index"`
}
