package types

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"gorm.io/gorm"
)

type User struct {
	ID        uint        `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time   `json:"createdAt"`
	Username  string      `json:"username" gorm:"unique"`
	Email     string      `json:"email"`
	Role      types2.Role `json:"role"`
	IconURL   string      `json:"iconURL"`
	Timezone  string      `json:"timezone"`
}

func ConvertUser(u *User, roleFixed bool, authProviderName string) *types2.User {
	if u == nil {
		return nil
	}

	user := &types2.User{
		Metadata: types2.Metadata{
			ID:      fmt.Sprint(u.ID),
			Created: *types2.NewTime(u.CreatedAt),
		},
		Username:      u.Username,
		Email:         u.Email,
		Role:          u.Role,
		ExplicitAdmin: roleFixed,
		IconURL:       u.IconURL,
		Timezone:      u.Timezone,
		CurrentAuthProvider: authProviderName,
	}

	return user
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
		db = db.Where("username LIKE ?", "%"+q.Username+"%")
	}
	if q.Email != "" {
		db = db.Where("email LIKE ?", "%"+q.Email+"%")
	}
	if q.Role != 0 {
		db = db.Where("role = ?", q.Role)
	}

	return db.Order("id")
}
