package types

import (
	"context"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"gorm.io/gorm"
)

const (
	RoleUnknown Role = iota
	RoleAdmin

	// RoleBasic is the default role. Leaving a little space for future roles.
	RoleBasic Role = 10

	defaultUserLimit = 1000
)

type Role int

func (u Role) HasRole(role Role) bool {
	return role >= u
}

type User struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`
	Username  string    `json:"username" gorm:"unique"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
}

type UserQuery struct {
	Username string
	Email    string
	Role     Role
	Limit    int
	Continue int
}

func NewUserQuery(ctx context.Context, u url.Values, logger *slog.Logger) UserQuery {
	limit, err := strconv.Atoi(u.Get("limit"))
	if err != nil || limit <= 0 {
		logger.DebugContext(ctx, "failed to parse limit query param", "err", err)
		limit = defaultUserLimit
	}

	offset, err := strconv.Atoi(u.Get("continue"))
	if err != nil || offset < 0 {
		logger.DebugContext(ctx, "failed to parse offset query param", "err", err)
		offset = 0
	}

	role, err := strconv.Atoi(u.Get("role"))
	if err != nil || role < 0 {
		logger.DebugContext(ctx, "failed to parse role query param", "err", err)
		role = 0
	}

	return UserQuery{
		Limit:    limit + 1,
		Continue: offset,
		Username: u.Get("username"),
		Email:    u.Get("email"),
		Role:     Role(role),
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

	return db.Order("id").Where("id >= ?", q.Continue).Limit(q.Limit)
}
