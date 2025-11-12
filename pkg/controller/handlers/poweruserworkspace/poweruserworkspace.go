package poweruserworkspace

import (
	"context"
	goerrors "errors"
	"fmt"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/create"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/errors"
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

	// Get the user
	user, err := h.gatewayClient.UserByID(req.Ctx, userIDStr)
	if err != nil {
		// Only clean up if the user is actually not found (deleted)
		// For transient errors (network, DB connection, etc.), return the error to retry
		if goerrors.Is(err, gorm.ErrRecordNotFound) {
			// User has been deleted - clean up any workspaces
			if err := h.deleteWorkspaceForUser(req.Ctx, req.Client, req.Namespace, userIDStr); err != nil {
				return err
			}
			return req.Delete(roleChange)
		}
		// For other errors, return them so the controller retries
		return fmt.Errorf("failed to get user %s: %w", userIDStr, err)
	}

	// Compute current effective role
	groupIDs, err := h.gatewayClient.ListGroupIDsForUser(req.Ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to list groups for user %d: %w", user.ID, err)
	}

	effectiveRole, err := h.gatewayClient.ResolveUserEffectiveRole(req.Ctx, user, groupIDs)
	if err != nil {
		return fmt.Errorf("failed to resolve effective role for user %d: %w", user.ID, err)
	}

	// Reconcile workspace state to match effective role
	if err := h.reconcileWorkspace(req.Ctx, req.Client, req.Namespace, *user, effectiveRole); err != nil {
		return err
	}

	return req.Delete(roleChange)
}

func (h *Handler) CreateACR(req router.Request, _ router.Response) error {
	workspace := req.Object.(*v1.PowerUserWorkspace)

	// Create the default access control rule for this workspace
	return h.createDefaultAccessControlRule(req.Ctx, req.Client, req.Namespace, workspace)
}

func (h *Handler) isPrivilegedRole(role types.Role) bool {
	return role.HasRole(types.RolePowerUser)
}

// reconcileWorkspace ensures the user's PowerUserWorkspace state matches their effective role.
// This is the core reconciliation logic that makes UserRoleChange events idempotent.
func (h *Handler) reconcileWorkspace(ctx context.Context, client kclient.Client, namespace string, user gatewaytypes.User, effectiveRole types.Role) error {
	userIDStr := strconv.Itoa(int(user.ID))

	// Get existing workspaces
	var workspaces v1.PowerUserWorkspaceList
	if err := client.List(ctx, &workspaces, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userIDStr,
		}),
	}); err != nil {
		return fmt.Errorf("failed to list workspaces: %w", err)
	}

	isPrivileged := h.isPrivilegedRole(effectiveRole)

	// Case 1: Should have workspace but doesn't
	if isPrivileged && len(workspaces.Items) == 0 {
		return h.createWorkspaceWithRole(ctx, client, namespace, user, effectiveRole)
	}

	// Case 2: Shouldn't have workspace but does
	if !isPrivileged && len(workspaces.Items) > 0 {
		return h.deleteAllWorkspaces(ctx, client, workspaces.Items)
	}

	// Case 3: Should have workspace and does - reconcile role
	if isPrivileged && len(workspaces.Items) > 0 {
		return h.reconcileWorkspaceRole(ctx, client, &workspaces.Items[0], effectiveRole)
	}

	// Case 4: Shouldn't have workspace and doesn't - no action needed
	return nil
}

// createWorkspaceWithRole creates a PowerUserWorkspace with the given role.
func (h *Handler) createWorkspaceWithRole(ctx context.Context, client kclient.Client, namespace string, user gatewaytypes.User, role types.Role) error {
	userIDStr := strconv.Itoa(int(user.ID))

	workspace := &v1.PowerUserWorkspace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      system.GetPowerUserWorkspaceID(userIDStr),
		},
		Spec: v1.PowerUserWorkspaceSpec{
			UserID: userIDStr,
			Role:   role,
		},
	}

	return create.OrGet(ctx, client, workspace)
}

// deleteAllWorkspaces deletes all given workspaces.
func (h *Handler) deleteAllWorkspaces(ctx context.Context, client kclient.Client, workspaces []v1.PowerUserWorkspace) error {
	for _, workspace := range workspaces {
		if err := client.Delete(ctx, &workspace); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// reconcileWorkspaceRole ensures the workspace's role matches the effective role.
func (h *Handler) reconcileWorkspaceRole(ctx context.Context, client kclient.Client, workspace *v1.PowerUserWorkspace, effectiveRole types.Role) error {
	// If role already matches, nothing to do
	if workspace.Spec.Role == effectiveRole {
		return nil
	}

	oldRole := workspace.Spec.Role
	workspace.Spec.Role = effectiveRole

	if err := client.Update(ctx, workspace); err != nil {
		return err
	}

	// If demoting from PowerUserPlus/Admin to PowerUser, clean up resources
	if !effectiveRole.HasRole(types.RolePowerUserPlus) && oldRole.HasRole(types.RolePowerUserPlus) {
		return h.cleanupWorkspaceResources(ctx, client, workspace)
	}

	return nil
}

// cleanupWorkspaceResources removes ACRs and MCPServers when demoting to PowerUser.
func (h *Handler) cleanupWorkspaceResources(ctx context.Context, client kclient.Client, workspace *v1.PowerUserWorkspace) error {
	namespace := workspace.Namespace

	// Delete AccessControlRules in this workspace
	var acrs v1.AccessControlRuleList
	if err := client.List(ctx, &acrs, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceID": workspace.Name,
		}),
	}); err != nil {
		return err
	}

	for _, acr := range acrs.Items {
		if err := client.Delete(ctx, &acr); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	// Reset DefaultAccessControlRuleGenerated status
	if workspace.Status.DefaultAccessControlRuleGenerated {
		workspace.Status.DefaultAccessControlRuleGenerated = false
		if err := client.Status().Update(ctx, workspace); err != nil {
			return err
		}
	}

	// Delete all MCPServers in this workspace
	var servers v1.MCPServerList
	if err := client.List(ctx, &servers, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceID": workspace.Name,
		}),
	}); err != nil {
		return err
	}

	for _, server := range servers.Items {
		if err := client.Delete(ctx, &server); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
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
		if err := client.Delete(ctx, &workspace); err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (h *Handler) createDefaultAccessControlRule(ctx context.Context, client kclient.Client, namespace string, workspace *v1.PowerUserWorkspace) error {
	// Power Users have implicit access to their own workspace resources through the workspace ownership check.
	// Only create default ACRs for PowerUserPlus and above, where the wildcard selector grants access to all users.
	if workspace.Spec.Role.IsExactBaseRole(types.RolePowerUser) {
		return nil
	}

	if workspace.Status.DefaultAccessControlRuleGenerated {
		return nil
	}

	var existingACRs v1.AccessControlRuleList
	if err := client.List(ctx, &existingACRs, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.powerUserWorkspaceID": workspace.Name,
		}),
	}); err != nil {
		return err
	}

	for _, acr := range existingACRs.Items {
		if acr.Spec.Generated {
			workspace.Status.DefaultAccessControlRuleGenerated = true
			return client.Status().Update(ctx, workspace)
		}
	}

	// For power user plus and admin, generate a rule that gives all users access
	defaultACR := &v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: system.AccessControlRulePrefix,
			Finalizers:   []string{v1.AccessControlRuleFinalizer},
		},
		Spec: v1.AccessControlRuleSpec{
			PowerUserWorkspaceID: workspace.Name,
			Generated:            true,
			Manifest: types.AccessControlRuleManifest{
				DisplayName: "Default Access Rule",
				Subjects: []types.Subject{
					{
						Type: types.SubjectTypeSelector,
						ID:   "*",
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

	if err := client.Create(ctx, defaultACR); err != nil {
		return err
	}

	workspace.Status.DefaultAccessControlRuleGenerated = true
	return client.Status().Update(ctx, workspace)
}
