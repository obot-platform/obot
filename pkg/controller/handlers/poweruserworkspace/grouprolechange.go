package poweruserworkspace

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

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

	// Process each user directly instead of creating separate events
	for _, user := range users {
		if err := h.reconcileUserWorkspace(req.Ctx, req.Client, req.Namespace, user); err != nil {
			// Log error but continue processing other users
			log.Errorf("failed to reconcile workspace for user %d: %v", user.ID, err)
		}
	}

	// Delete the GroupRoleChange event now that we've processed it
	return req.Delete(groupRoleChange)
}

// reconcileUserWorkspace reconciles the workspace for a single user based on their effective role.
// This contains the same logic as HandleRoleChange but can be called directly without creating an event.
func (h *Handler) reconcileUserWorkspace(ctx context.Context, client kclient.Client, namespace string, user gatewaytypes.User) error {
	// Compute current effective role
	groupIDs, err := h.gatewayClient.ListGroupIDsForUser(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to list groups for user %d: %w", user.ID, err)
	}

	effectiveRole, err := h.gatewayClient.ResolveUserEffectiveRole(ctx, &user, groupIDs)
	if err != nil {
		return fmt.Errorf("failed to resolve effective role for user %d: %w", user.ID, err)
	}

	// Reconcile workspace state to match effective role
	return h.reconcileWorkspace(ctx, client, namespace, user, effectiveRole)
}
