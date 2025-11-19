// nolint:revive
package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

// GroupRoleAssignment assigns a role to all members of an auth provider group.
type GroupRoleAssignment struct {
	// GroupName is the name of the auth provider group (used as primary key)
	GroupName string `json:"groupName" gorm:"primaryKey"`

	// CreatedAt is when the assignment was created
	CreatedAt time.Time `json:"createdAt" gorm:"autoCreateTime"`

	// UpdatedAt is when the assignment was last modified
	UpdatedAt time.Time `json:"updatedAt" gorm:"autoUpdateTime"`

	// Role is the role to assign to all members of the group
	Role types2.Role `json:"role" gorm:"not null"`

	// Description is an optional description of why this assignment exists
	Description string `json:"description"`
}
