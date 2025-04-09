package types

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

type User struct {
	ID             uint        `json:"id" gorm:"primaryKey"`
	CreatedAt      time.Time   `json:"createdAt"`
	Username       string      `json:"username"`
	HashedUsername string      `json:"-" gorm:"unique"`
	Email          string      `json:"email"`
	HashedEmail    string      `json:"-"`
	VerifiedEmail  *bool       `json:"verifiedEmail,omitempty"`
	Role           types2.Role `json:"role"`
	IconURL        string      `json:"iconURL"`
	Timezone       string      `json:"timezone"`
	// LastActiveDay is the time of the last request made by this user, currently at the 24 hour granularity.
	LastActiveDay time.Time `json:"lastActiveDay"`
	Encrypted     bool      `json:"encrypted"`
}

func ConvertUser(u *User, roleFixed bool, authProviderName string) *types2.User {
	if u == nil {
		return nil
	}

	return &types2.User{
		Metadata: types2.Metadata{
			ID:      fmt.Sprint(u.ID),
			Created: *types2.NewTime(u.CreatedAt),
		},
		Username:            u.Username,
		Email:               u.Email,
		Role:                u.Role,
		ExplicitAdmin:       roleFixed,
		IconURL:             u.IconURL,
		Timezone:            u.Timezone,
		CurrentAuthProvider: authProviderName,
		LastActiveDay:       *types2.NewTime(u.LastActiveDay),
	}
}

type UserQuery struct {
	Username string
	Email    string
	Role     types2.Role
}

func NewUserQuery(u url.Values) UserQuery {
	role, err := strconv.Atoi(u.Get("role"))
	if err != nil || role < 0 {
		role = 0
	}

	return UserQuery{
		Username: u.Get("username"),
		Email:    u.Get("email"),
		Role:     types2.Role(role),
	}
}

func (q UserQuery) Scope(db *gorm.DB) *gorm.DB {
	if q.Username != "" {
		db = db.Where("hashed_username = ?", "%"+hash.String(q.Username)+"%")
	}
	if q.Email != "" {
		db = db.Where("hashed_email = ?", "%"+hash.String(q.Email)+"%")
	}
	if q.Role != 0 {
		db = db.Where("role = ?", q.Role)
	}

	return db.Order("id")
}
