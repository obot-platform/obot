//nolint:revive
package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

type TempSetupUser struct {
	ID               uint        `json:"id" gorm:"primaryKey"`
	UserID           uint        `json:"userID" gorm:"index"`
	Username         string      `json:"username"`
	Email            string      `json:"email"`
	Role             types2.Role `json:"role"`
	IconURL          string      `json:"iconURL"`
	AuthProviderName string      `json:"authProviderName"`
	AuthProviderNS   string      `json:"authProviderNS"`
	CreatedAt        time.Time   `json:"createdAt"`
}
