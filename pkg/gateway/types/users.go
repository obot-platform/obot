//nolint:revive
package types

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
)

const (
	// UserDisabledReasonSCIMInactive means the identity provider deprovisioned the user through SCIM.
	UserDisabledReasonSCIMInactive UserDisabledReason = "scim_inactive"
	// UserDisabledReasonSCIMUnprovisioned means SCIM was enforced for the user's auth provider before the
	// identity provider provisioned the user.
	UserDisabledReasonSCIMUnprovisioned UserDisabledReason = "scim_unprovisioned"
)

var (
	// UserLifecycleColumns are the user columns that only lifecycle operations write. Every other user write
	// omits them, so that a stale copy of a user can neither disable nor re-enable the account.
	UserLifecycleColumns = []string{"disabled_at", "disabled_reason"}
)

// UserDisabledReason records why a user is disabled.
type UserDisabledReason string

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
	LastActiveDay          time.Time `json:"lastActiveDay"`
	Internal               bool      `json:"internal" gorm:"default:false"`
	DailyInputTokensLimit  int       `json:"dailyInputTokensLimit"`
	DailyOutputTokensLimit int       `json:"dailyOutputTokensLimit"`
	Encrypted              bool      `json:"encrypted"`
	// Soft delete fields
	DeletedAt        *time.Time `json:"deletedAt,omitempty"`
	OriginalEmail    string     `json:"-"`
	OriginalUsername string     `json:"-"`

	// Lifecycle fields. A disabled user keeps their account, identities, roles, memberships, and resources, but
	// is denied access. Unlike deletion, disabling is reversible. Only lifecycle operations write these fields;
	// see UserLifecycleColumns.
	DisabledAt     *time.Time         `json:"disabledAt,omitempty"`
	DisabledReason UserDisabledReason `json:"disabledReason,omitempty" gorm:"not null;default:''"`
}

type UserQuery struct {
	Username       string
	Email          string
	Role           types2.Role
	IncludeDeleted bool
}

// Valid reports whether r is a known disable reason.
func (r UserDisabledReason) Valid() bool {
	switch r {
	case UserDisabledReasonSCIMInactive, UserDisabledReasonSCIMUnprovisioned:
		return true
	default:
		return false
	}
}

// Status returns the user's lifecycle status. Deletion takes precedence over disabling.
func (u User) Status() types2.UserStatus {
	switch {
	case u.DeletedAt != nil:
		return types2.UserStatusDeleted
	case u.DisabledAt != nil:
		return types2.UserStatusDisabled
	default:
		return types2.UserStatusActive
	}
}

// ManagementSource returns what controls the user's lifecycle status, as far as the user row shows it. Only
// SCIM sets a disable reason, so a user with one is managed by SCIM. So is a user with a SCIM binding, which the
// row does not show, so callers that read the binding report SCIM for it.
func (u User) ManagementSource() types2.UserManagementSource {
	if u.DisabledReason.Valid() {
		return types2.UserManagementSourceSCIM
	}
	return types2.UserManagementSourceObot
}

func ConvertUser(u *User, roleFixed bool, authProviderName string) *types2.User {
	return ConvertUserWithEffectiveRole(u, roleFixed, authProviderName, u.Role)
}

func ConvertUserWithEffectiveRole(u *User, roleFixed bool, authProviderName string, effectiveRole types2.Role) *types2.User {
	if u == nil {
		return nil
	}

	groups := effectiveRole.Groups()
	if authProviderName == system.BootstrapName {
		groups = []string{types2.GroupOwner, types2.GroupAdmin, types2.GroupBasic, types2.GroupAuthenticated}
	}

	user := &types2.User{
		ID:                     fmt.Sprint(u.ID),
		Created:                *types2.NewTime(u.CreatedAt),
		DisplayName:            u.DisplayName,
		Username:               u.Username,
		Email:                  u.Email,
		Role:                   u.Role,
		EffectiveRole:          effectiveRole,
		Groups:                 groups,
		ExplicitRole:           roleFixed,
		IconURL:                u.IconURL,
		Timezone:               u.Timezone,
		CurrentAuthProvider:    authProviderName,
		LastActiveDay:          *types2.NewTime(u.LastActiveDay),
		Internal:               u.Internal,
		DailyInputTokensLimit:  u.DailyInputTokensLimit,
		DailyOutputTokensLimit: u.DailyOutputTokensLimit,
		OriginalEmail:          u.OriginalEmail,
		OriginalUsername:       u.OriginalUsername,
		Status:                 u.Status(),
		ManagementSource:       u.ManagementSource(),
	}

	if u.DeletedAt != nil {
		user.DeletedAt = types2.NewTime(*u.DeletedAt)
	}
	if u.DisabledAt != nil {
		user.DisabledAt = types2.NewTime(*u.DisabledAt)
		user.DisabledReason = string(u.DisabledReason)
	}

	return user
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
		db = db.Where("hashed_username = ?", hash.String(q.Username))
	}
	if q.Email != "" {
		db = db.Where("hashed_email = ?", hash.String(q.Email))
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
