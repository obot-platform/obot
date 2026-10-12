package db

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const (
	// userImpersonationRole is the role bit that the removed User Impersonation role used.
	userImpersonationRole = 256
)

// removeUserImpersonationRole clears the removed User Impersonation role from every stored role.
func removeUserImpersonationRole(tx *gorm.DB) error {
	for _, model := range []any{&types.User{}, &types.GroupRoleAssignment{}, &types.TempSetupUser{}} {
		if err := tx.Unscoped().Model(model).
			Where("role & ? <> 0", userImpersonationRole).
			Update("role", gorm.Expr("role - ?", userImpersonationRole)).Error; err != nil {
			return fmt.Errorf("failed to remove the user impersonation role from %T: %w", model, err)
		}
	}
	return nil
}
