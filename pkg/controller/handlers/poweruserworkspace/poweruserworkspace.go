package poweruserworkspace

import (
	"fmt"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gatewayClient *gateway.Client
}

func New(gatewayClient *gateway.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

// EnsureWorkspaceForUser creates a PowerUserWorkspace for users with elevated roles
func (h *Handler) EnsureWorkspaceForUser(req router.Request, _ router.Response) error {
	// This handler would be triggered by user changes or system startup
	// For now, we'll implement this as a reactive handler when users with elevated roles are detected
	
	// Get all users with elevated roles from the gateway
	users, err := h.gatewayClient.UsersIncludeDeleted(req.Ctx)
	if err != nil {
		return fmt.Errorf("failed to get users: %w", err)
	}

	for _, user := range users {
		// Skip deleted users and users without elevated roles
		if user.DeletedAt != nil {
			continue
		}
		
		if user.Role != types.RoleAdmin && user.Role != types.RolePowerUserPlus && user.Role != types.RolePowerUser {
			continue
		}

		// Check if workspace already exists for this user
		var existingWorkspaces v1.PowerUserWorkspaceList
		if err := req.List(&existingWorkspaces, &kclient.ListOptions{
			Namespace: req.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.userID": strconv.FormatUint(uint64(user.ID), 10),
			}),
		}); err != nil {
			return fmt.Errorf("failed to list existing workspaces for user %d: %w", user.ID, err)
		}

		// If workspace doesn't exist, create it
		if len(existingWorkspaces.Items) == 0 {
			workspace := &v1.PowerUserWorkspace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.PowerUserWorkspacePrefix,
					Namespace:    req.Namespace,
				},
				Spec: v1.PowerUserWorkspaceSpec{
					UserID: strconv.FormatUint(uint64(user.ID), 10),
					Role:   user.Role,
				},
			}

			if err := req.Create(workspace); err != nil {
				return fmt.Errorf("failed to create workspace for user %d: %w", user.ID, err)
			}
		}
	}

	return nil
}

// CleanupOnRoleDemotion deletes PowerUserWorkspace when user loses elevated role
func (h *Handler) CleanupOnRoleDemotion(req router.Request, _ router.Response) error {
	workspace := req.Object.(*v1.PowerUserWorkspace)
	
	// Get the current user information from gateway
	userID, err := strconv.ParseUint(workspace.Spec.UserID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid userID in workspace: %w", err)
	}

	user, err := h.gatewayClient.UserByID(req.Ctx, uint(userID))
	if err != nil {
		// If user is deleted, we should clean up the workspace
		if gateway.IsUserNotFound(err) {
			return req.Delete(workspace)
		}
		return fmt.Errorf("failed to get user %s: %w", workspace.Spec.UserID, err)
	}

	// If user no longer has an elevated role, delete the workspace
	if user.Role != types.RoleAdmin && user.Role != types.RolePowerUserPlus && user.Role != types.RolePowerUser {
		return req.Delete(workspace)
	}

	// Update workspace role if it has changed
	if workspace.Spec.Role != user.Role {
		workspace.Spec.Role = user.Role
		return req.Update(workspace)
	}

	return nil
}

// ValidateOwnership ensures workspace belongs to the correct user
func (h *Handler) ValidateOwnership(req router.Request, _ router.Response) error {
	workspace := req.Object.(*v1.PowerUserWorkspace)
	
	// Validate that userID is a valid user
	userID, err := strconv.ParseUint(workspace.Spec.UserID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid userID in workspace: %w", err)
	}

	_, err = h.gatewayClient.UserByID(req.Ctx, uint(userID))
	if err != nil {
		return fmt.Errorf("workspace references invalid user %s: %w", workspace.Spec.UserID, err)
	}

	return nil
}

// UpdateResourceCount tracks the number of resources owned by this workspace
func (h *Handler) UpdateResourceCount(req router.Request, _ router.Response) error {
	workspace := req.Object.(*v1.PowerUserWorkspace)
	
	// Count MCPServers owned by this workspace
	var mcpServers v1.MCPServerList
	if err := req.List(&mcpServers, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceName": workspace.Name,
		}),
	}); err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	// Count MCPServerCatalogEntries owned by this workspace
	var catalogEntries v1.MCPServerCatalogEntryList
	if err := req.List(&catalogEntries, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceName": workspace.Name,
		}),
	}); err != nil {
		return fmt.Errorf("failed to list catalog entries: %w", err)
	}

	// Count AccessControlRules owned by this workspace
	var acrs v1.AccessControlRuleList
	if err := req.List(&acrs, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceName": workspace.Name,
		}),
	}); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	// Update resource count in status
	resourceCount := &v1.PowerUserWorkspaceResourceCount{
		MCPServers:              len(mcpServers.Items),
		MCPServerCatalogEntries: len(catalogEntries.Items),
		AccessControlRules:      len(acrs.Items),
	}

	if workspace.Status.ResourceCount == nil ||
		workspace.Status.ResourceCount.MCPServers != resourceCount.MCPServers ||
		workspace.Status.ResourceCount.MCPServerCatalogEntries != resourceCount.MCPServerCatalogEntries ||
		workspace.Status.ResourceCount.AccessControlRules != resourceCount.AccessControlRules {
		
		workspace.Status.ResourceCount = resourceCount
		workspace.Status.Ready = true
		return req.Client.Status().Update(req.Ctx, workspace)
	}

	return nil
}