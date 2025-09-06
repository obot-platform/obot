package poweruserworkspace

import (
	"context"
	"strconv"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/create"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gatewayClient *gclient.Client
}

func NewHandler(gatewayClient *gclient.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (h *Handler) HandleRoleChange(req router.Request, _ router.Response) error {
	roleChange := req.Object.(*v1.UserRoleChange)
	userIDStr := strconv.Itoa(int(roleChange.Spec.UserID))

	oldPrivileged := h.isPrivilegedRole(roleChange.Spec.OldRole)
	newPrivileged := h.isPrivilegedRole(roleChange.Spec.NewRole)

	if !oldPrivileged && newPrivileged {
		user, err := h.gatewayClient.UserByID(req.Ctx, userIDStr)
		if err != nil {
			return err
		}
		if err := h.ensureWorkspaceForUser(req.Ctx, req.Client, req.Namespace, *user); err != nil {
			return err
		}
	} else if oldPrivileged && !newPrivileged {
		if err := h.deleteWorkspaceForUser(req.Ctx, req.Client, req.Namespace, userIDStr); err != nil {
			return err
		}
	} else if oldPrivileged && newPrivileged {
		if err := h.updateWorkspaceRole(req.Ctx, req.Client, req.Namespace, userIDStr, roleChange.Spec.NewRole); err != nil {
			return err
		}

		// If demoting to PowerUser from PowerUserPlus or Admin, clean up workspace resources.
		// PowerUsers are not allowed to manage Access Control Rules or multi-user MCPServers.
		if roleChange.Spec.NewRole == types.RolePowerUser &&
			(roleChange.Spec.OldRole == types.RolePowerUserPlus ||
				roleChange.Spec.OldRole == types.RoleAdmin) {
			if err := h.cleanupWorkspaceForDemotionToPowerUser(req.Ctx, req.Client, req.Namespace, userIDStr); err != nil {
				return err
			}
		}
	}

	return req.Delete(roleChange)
}

func (h *Handler) CreateACR(req router.Request, _ router.Response) error {
	workspace := req.Object.(*v1.PowerUserWorkspace)

	// Create the default access control rule for this workspace
	if err := h.createDefaultAccessControlRule(req.Ctx, req.Client, req.Namespace, workspace); err != nil {
		return err
	}

	return nil
}

func (h *Handler) isPrivilegedRole(role types.Role) bool {
	return role.HasRole(types.RolePowerUser) || role.HasRole(types.RolePowerUserPlus) || role.HasRole(types.RoleAdmin)
}

func (h *Handler) ensureWorkspaceForUser(ctx context.Context, client kclient.Client, namespace string, user gatewaytypes.User) error {
	userIDStr := strconv.Itoa(int(user.ID))

	var existingWorkspaces v1.PowerUserWorkspaceList
	if err := client.List(ctx, &existingWorkspaces, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userIDStr,
		}),
	}); err != nil {
		return err
	}

	if len(existingWorkspaces.Items) > 0 {
		return nil
	}

	workspace := &v1.PowerUserWorkspace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name.SafeConcatName(system.PowerUserWorkspacePrefix, userIDStr),
		},
		Spec: v1.PowerUserWorkspaceSpec{
			UserID: userIDStr,
			Role:   user.Role,
		},
	}

	return create.OrGet(ctx, client, workspace)
}

func (h *Handler) deleteWorkspaceForUser(ctx context.Context, client kclient.Client, namespace string, userID string) error {
	var workspaces v1.PowerUserWorkspaceList
	if err := client.List(ctx, &workspaces, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if err := client.Delete(ctx, &workspace); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) updateWorkspaceRole(ctx context.Context, client kclient.Client, namespace string, userID string, newRole types.Role) error {
	var workspaces v1.PowerUserWorkspaceList
	if err := client.List(ctx, &workspaces, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if workspace.Spec.Role != newRole {
			workspace.Spec.Role = newRole
			if err := client.Update(ctx, &workspace); err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *Handler) cleanupWorkspaceForDemotionToPowerUser(ctx context.Context, client kclient.Client, namespace string, userID string) error {
	// Find the user's workspace
	var workspaces v1.PowerUserWorkspaceList
	if err := client.List(ctx, &workspaces, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return err
	}

	if len(workspaces.Items) == 0 {
		return nil // No workspace to clean up
	}

	workspaceName := workspaces.Items[0].Name

	// Delete non-generated AccessControlRules in this workspace
	var acrs v1.AccessControlRuleList
	if err := client.List(ctx, &acrs, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceID": workspaceName,
		}),
	}); err != nil {
		return err
	}

	for _, acr := range acrs.Items {
		if !acr.Spec.Generated {
			if err := client.Delete(ctx, &acr); err != nil {
				return err
			}
		}
	}

	// Delete all MCPServers in this workspace
	var servers v1.MCPServerList
	if err := client.List(ctx, &servers, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceID": workspaceName,
		}),
	}); err != nil {
		return err
	}

	for _, server := range servers.Items {
		if err := client.Delete(ctx, &server); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) createDefaultAccessControlRule(ctx context.Context, client kclient.Client, namespace string, workspace *v1.PowerUserWorkspace) error {
	// Check if the default ACR already exists
	var existingACRs v1.AccessControlRuleList
	if err := client.List(ctx, &existingACRs, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceID": workspace.Name,
		}),
	}); err != nil {
		return err
	}

	// Check if a generated ACR already exists for this workspace
	for _, acr := range existingACRs.Items {
		if acr.Spec.Generated {
			return nil // Already exists
		}
	}

	// Create the default access control rule
	defaultACR := &v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: system.AccessControlRulePrefix,
		},
		Spec: v1.AccessControlRuleSpec{
			PowerUserWorkspaceID: workspace.Name,
			Generated:            true,
			Manifest: types.AccessControlRuleManifest{
				DisplayName: "Default Access Rule",
				Subjects: []types.Subject{
					{
						Type: types.SubjectTypeUser,
						ID:   workspace.Spec.UserID,
					},
				},
				Resources: []types.Resource{
					{
						Type: types.ResourceTypeSelector,
						ID:   "*",
					},
				},
			},
		},
	}

	return create.OrGet(ctx, client, defaultACR)
}
