package setup

import (
	"fmt"
	"log/slog"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// PromoteToOwner grants the Owner role to an identity that has proven it can sign in through the
// provider Obot is about to depend on. Bootstrap owner confirmation and the staged auth provider
// switch share it rather than each deciding for themselves. An address configured as an Admin
// through the environment is refused, since that configuration must win over a runtime decision.
func PromoteToOwner(req api.Context, user *gatewaytypes.User) error {
	if user.Role.HasRole(types.RoleOwner) {
		return nil
	}

	if req.GatewayClient.HasExplicitRole(user.Email).HasRole(types.RoleAdmin) {
		slog.Info("Rejecting owner promotion for explicitly configured admin", "userID", user.ID)
		return types.NewErrBadRequest(
			"cannot promote %s to Owner: the address is configured as an Admin through the environment",
			user.Email,
		)
	}

	user.Role = user.Role.SwitchBaseRole(types.RoleOwner)
	if _, err := req.GatewayClient.UpdateUser(req.Context(), true, user, fmt.Sprintf("%d", user.ID)); err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}
	slog.Info("Promoted user to owner", "userID", user.ID)

	// The role change propagates the new role to the rest of the system. A failure here still
	// leaves a correct role in the database, so it is logged rather than failing the promotion.
	if err := req.Create(&v1.UserRoleChange{
		GenerateName: system.UserRoleChangePrefix,
		Namespace:    system.DefaultNamespace,
		Spec: v1.UserRoleChangeSpec{
			UserID: user.ID,
		},
	}); err != nil {
		slog.Warn("failed to create user role change for new owner", "userID", user.ID, "error", err)
	}

	return nil
}
