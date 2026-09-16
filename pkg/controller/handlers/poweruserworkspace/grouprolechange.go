package poweruserworkspace

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// HandleGroupRoleChange processes GroupRoleChange events by reconciling workspaces
// for all users in the group based on their current effective role.
func (h *Handler) HandleGroupRoleChange(req router.Request, _ router.Response) error {
	groupRoleChange := req.Object.(*v1.GroupRoleChange)
	groupName := groupRoleChange.Spec.GroupName

	// Get all users in this group
	users, err := h.gatewayClient.GetUsersInGroup(req.Ctx, groupName)
	if err != nil {
		return fmt.Errorf("failed to get users in group %s: %w", groupName, err)
	}

	// Create a UserRoleChange object for each user in the group.
	for _, user := range users {
		if err := req.Client.Create(req.Ctx, &v1.UserRoleChange{
			Namespace:    req.Namespace,
			GenerateName: system.UserRoleChangePrefix,
			Spec: v1.UserRoleChangeSpec{
				UserID: user.ID,
			},
		}); err != nil {
			return fmt.Errorf("failed to create role change event for user %v: %w", user.ID, err)
		}
	}

	// Delete the GroupRoleChange event now that we've processed it
	return req.Delete(groupRoleChange)
}
