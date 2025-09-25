//nolint:revive
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
	DisplayName    string      `json:"displayName"`
	Username       string      `json:"username"`
	HashedUsername string      `json:"-" gorm:"unique"`
	Email          string      `json:"email"`
	HashedEmail    string      `json:"-"`
	VerifiedEmail  *bool       `json:"verifiedEmail,omitempty"`
	Role           types2.Role `json:"role"`
	IconURL        string      `json:"iconURL"`
	Timezone       string      `json:"timezone"`
	// LastActiveDay is the time of the last request made by this user, currently at the 24 hour granularity.
	LastActiveDay              time.Time `json:"lastActiveDay"`
	Internal                   bool      `json:"internal" gorm:"default:false"`
	DailyPromptTokensLimit     int       `json:"dailyPromptTokensLimit"`
	DailyCompletionTokensLimit int       `json:"dailyCompletionTokensLimit"`
	Encrypted                  bool      `json:"encrypted"`
	// Soft delete fields
	DeletedAt        *time.Time `json:"deletedAt,omitempty"`
	OriginalEmail    string     `json:"-"`
	OriginalUsername string     `json:"-"`
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
		DisplayName:                u.DisplayName,
		Username:                   u.Username,
		Email:                      u.Email,
		Role:                       u.Role,
		Groups:                     u.Role.Groups(),
		ExplicitRole:               roleFixed,
		IconURL:                    u.IconURL,
		Timezone:                   u.Timezone,
		CurrentAuthProvider:        authProviderName,
		LastActiveDay:              *types2.NewTime(u.LastActiveDay),
		Internal:                   u.Internal,
		DailyPromptTokensLimit:     u.DailyPromptTokensLimit,
		DailyCompletionTokensLimit: u.DailyCompletionTokensLimit,
		OriginalEmail:              u.OriginalEmail,
		OriginalUsername:           u.OriginalUsername,
	}

	if u.DeletedAt != nil {
		user.DeletedAt = types2.NewTime(*u.DeletedAt)
	}

	return user
}

type UserQuery struct {
	Username       string
	Email          string
	Role           types2.Role
	IncludeDeleted bool
}

func NewUserQuery(u url.Values) UserQuery {
	role, err := strconv.Atoi(u.Get("role"))
	if err != nil || role < 0 {
		role = 0
	}

	return UserQuery{
		Username:       u.Get("username"),
		Email:          u.Get("email"),
		Role:           types2.Role(role),
		IncludeDeleted: u.Get("includeDeleted") == "true",
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

	// Filter out soft-deleted users by default
	if !q.IncludeDeleted {
		db = db.Where("deleted_at IS NULL")
	}

	return db.Order("id")
}
