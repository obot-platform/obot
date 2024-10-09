package types

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	types2 "github.com/otto8-ai/otto8/apiclient/types"
	"gorm.io/gorm"
)

type User struct {
	ID        uint        `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time   `json:"createdAt"`
	Username  string      `json:"username" gorm:"unique"`
	Email     string      `json:"email"`
	Role      types2.Role `json:"role"`
}

func ConvertUser(u *User) *types2.User {
	if u == nil {
		return nil
	}

	return &types2.User{
		Metadata: types2.Metadata{
			ID:      fmt.Sprint(u.ID),
			Created: *types2.NewTime(u.CreatedAt),
		},
		Username: u.Username,
		Email:    u.Email,
		Role:     u.Role,
	}
}

type UserQuery struct {
	Username string
	Email    string
	Role     types2.Role
}

func NewUserQuery(u url.Values) UserQuery {
	limit, err := strconv.Atoi(u.Get("limit"))
	if err != nil || limit <= 0 {
		limit = 0
	}

	offset, err := strconv.Atoi(u.Get("continue"))
	if err != nil || offset < 0 {
		offset = 0
	}

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
