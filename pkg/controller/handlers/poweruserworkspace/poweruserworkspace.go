package poweruserworkspace

import (
	"context"
	"fmt"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	types2 "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	client kclient.Client
}

func NewHandler(client kclient.Client) *Handler {
	return &Handler{
		client: client,
	}
}

func (h *Handler) PowerUserWorkspace(req router.Request, _ router.Response) error {
	puw := req.Object.(*v1.PowerUserWorkspace)

	// Ensure PowerUserWorkspace exists and is properly configured
	if puw.DeletionTimestamp != nil {
		return h.handleDeletion(req.Ctx, puw)
	}

	return h.ensureWorkspace(req.Ctx, puw)
}

func (h *Handler) ensureWorkspace(ctx context.Context, puw *v1.PowerUserWorkspace) error {
	// Validate the role is appropriate for a PowerUserWorkspace
	if !isPowerUserOrAdmin(puw.Spec.Role) {
		return fmt.Errorf("invalid role for PowerUserWorkspace: %v", puw.Spec.Role)
	}

	// Update resource counts
	if err := h.updateResourceCounts(ctx, puw); err != nil {
		return fmt.Errorf("failed to update resource counts: %w", err)
	}

	return nil
}

func (h *Handler) handleDeletion(ctx context.Context, puw *v1.PowerUserWorkspace) error {
	// The deletion refs system will handle cascading deletes
	// We just need to ensure any cleanup logic is performed
	
	// Remove finalizers to allow deletion to proceed
	if len(puw.Finalizers) > 0 {
		puw.Finalizers = nil
		return h.client.Update(ctx, puw)
	}

	return nil
}

func (h *Handler) updateResourceCounts(ctx context.Context, puw *v1.PowerUserWorkspace) error {
	// Count AccessControlRules
	acrList := &v1.AccessControlRuleList{}
	if err := h.client.List(ctx, acrList, 
		kclient.InNamespace(puw.Namespace),
		kclient.MatchingFields{"spec.powerUserWorkspaceID": puw.Name}); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	// Count MCPServers
	mcpServerList := &v1.MCPServerList{}
	if err := h.client.List(ctx, mcpServerList,
		kclient.InNamespace(puw.Namespace),
		kclient.MatchingFields{"spec.powerUserWorkspaceID": puw.Name}); err != nil {
		return fmt.Errorf("failed to list mcp servers: %w", err)
	}

	// Count MCPServerCatalogEntries
	mcpEntryList := &v1.MCPServerCatalogEntryList{}
	if err := h.client.List(ctx, mcpEntryList,
		kclient.InNamespace(puw.Namespace),
		kclient.MatchingFields{"spec.powerUserWorkspaceID": puw.Name}); err != nil {
		return fmt.Errorf("failed to list mcp server catalog entries: %w", err)
	}

	// Update status
	puw.Status.ResourceCounts = v1.PowerUserWorkspaceResourceCounts{
		AccessControlRules:       len(acrList.Items),
		MCPServers:              len(mcpServerList.Items),
		MCPServerCatalogEntries: len(mcpEntryList.Items),
	}

	return h.client.Status().Update(ctx, puw)
}

// EnsurePowerUserWorkspaceForUser creates or updates a PowerUserWorkspace for the given user
func (h *Handler) EnsurePowerUserWorkspaceForUser(ctx context.Context, userID string, role types2.Role) (*v1.PowerUserWorkspace, error) {
	if !isPowerUserOrAdmin(role) {
		return nil, fmt.Errorf("role %v does not require a PowerUserWorkspace", role)
	}

	workspaceName := fmt.Sprintf("workspace-%s", userID)
	workspace := &v1.PowerUserWorkspace{}
	
	err := h.client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      workspaceName,
	}, workspace)

	if apierrors.IsNotFound(err) {
		// Create new workspace
		workspace = &v1.PowerUserWorkspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workspaceName,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.PowerUserWorkspaceSpec{
				UserID: userID,
				Role:   role,
			},
		}
		return workspace, h.client.Create(ctx, workspace)
	} else if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	// Update existing workspace if role changed
	if workspace.Spec.Role != role {
		workspace.Spec.Role = role
		if err := h.client.Update(ctx, workspace); err != nil {
			return nil, fmt.Errorf("failed to update workspace role: %w", err)
		}
	}

	return workspace, nil
}

// DeletePowerUserWorkspaceForUser deletes the PowerUserWorkspace for the given user
func (h *Handler) DeletePowerUserWorkspaceForUser(ctx context.Context, userID string) error {
	workspaceName := fmt.Sprintf("workspace-%s", userID)
	workspace := &v1.PowerUserWorkspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workspaceName,
			Namespace: system.DefaultNamespace,
		},
	}

	err := h.client.Delete(ctx, workspace)
	if apierrors.IsNotFound(err) {
		// Already deleted, that's fine
		return nil
	}
	return err
}

// GetPowerUserWorkspaceForUser gets the PowerUserWorkspace for the given user
func (h *Handler) GetPowerUserWorkspaceForUser(ctx context.Context, userID string) (*v1.PowerUserWorkspace, error) {
	workspaceName := fmt.Sprintf("workspace-%s", userID)
	workspace := &v1.PowerUserWorkspace{}
	
	err := h.client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      workspaceName,
	}, workspace)

	if apierrors.IsNotFound(err) {
		return nil, nil // No workspace found
	}
	return workspace, err
}

func isPowerUserOrAdmin(role types2.Role) bool {
	return role == types2.RoleAdmin || role == types2.RolePowerUser || role == types2.RolePowerUserPlus
}

// Helper function to get user ID from string
func getUserIDFromString(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}