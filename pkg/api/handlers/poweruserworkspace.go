package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type PowerUserWorkspaceHandler struct{}

func NewPowerUserWorkspaceHandler() *PowerUserWorkspaceHandler {
	return &PowerUserWorkspaceHandler{}
}

// List returns all PowerUserWorkspaces accessible to the user
func (h *PowerUserWorkspaceHandler) List(req api.Context) error {
	var list v1.PowerUserWorkspaceList
	
	if req.UserIsAdmin() {
		// Admins can see all workspaces
		if err := req.List(&list); err != nil {
			return err
		}
	} else {
		// Regular users can only see their own workspace
		if err := req.List(&list, &kclient.ListOptions{
			Namespace: req.Namespace(),
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.userID": req.User.GetUID(),
			}),
		}); err != nil {
			return err
		}
	}

	var items []types.PowerUserWorkspace
	for _, workspace := range list.Items {
		items = append(items, convertPowerUserWorkspace(workspace))
	}

	return req.Write(types.PowerUserWorkspaceList{
		Items: items,
	})
}

// Get returns a specific PowerUserWorkspace by ID
func (h *PowerUserWorkspaceHandler) Get(req api.Context) error {
	workspaceID := req.PathValue("workspace_id")
	
	var workspace v1.PowerUserWorkspace
	if err := req.Get(&workspace, workspaceID); err != nil {
		return err
	}

	// Check access - user must own the workspace or be admin
	if workspace.Spec.UserID != req.User.GetUID() && !req.UserIsAdmin() {
		return types.NewErrForbidden("access denied to workspace")
	}

	return req.Write(convertPowerUserWorkspace(workspace))
}

// Create creates a new PowerUserWorkspace (admin only operation, but typically done automatically)
func (h *PowerUserWorkspaceHandler) Create(req api.Context) error {
	if !req.UserIsAdmin() {
		return types.NewErrForbidden("only admins can manually create workspaces")
	}

	var input types.PowerUserWorkspaceManifest
	if err := req.Read(&input); err != nil {
		return err
	}

	// Validate that the user has the appropriate role
	user, err := req.GatewayClient.UserByID(req.Context(), input.UserID)
	if err != nil {
		return types.NewErrBadRequest("invalid user ID: %v", err)
	}

	if user.Role != types.RoleAdmin && user.Role != types.RolePowerUserPlus && user.Role != types.RolePowerUser {
		return types.NewErrBadRequest("user does not have elevated role required for workspace")
	}

	workspace := &v1.PowerUserWorkspace{
		Spec: v1.PowerUserWorkspaceSpec{
			UserID: input.UserID,
			Role:   user.Role,
		},
	}

	if err := req.Create(workspace); err != nil {
		return err
	}

	return req.Write(convertPowerUserWorkspace(*workspace))
}

// Delete deletes a PowerUserWorkspace
func (h *PowerUserWorkspaceHandler) Delete(req api.Context) error {
	workspaceID := req.PathValue("workspace_id")
	
	var workspace v1.PowerUserWorkspace
	if err := req.Get(&workspace, workspaceID); err != nil {
		return err
	}

	// Check access - user must own the workspace or be admin
	if workspace.Spec.UserID != req.User.GetUID() && !req.UserIsAdmin() {
		return types.NewErrForbidden("access denied to workspace")
	}

	if err := req.Delete(&workspace); err != nil {
		return err
	}

	return req.Write(convertPowerUserWorkspace(workspace))
}

func convertPowerUserWorkspace(workspace v1.PowerUserWorkspace) types.PowerUserWorkspace {
	return types.PowerUserWorkspace{
		Metadata:    MetadataFrom(&workspace),
		UserID:      workspace.Spec.UserID,
		Role:        workspace.Spec.Role,
		Ready:       workspace.Status.Ready,
		ResourceCount: convertResourceCount(workspace.Status.ResourceCount),
	}
}

func convertResourceCount(rc *v1.PowerUserWorkspaceResourceCount) *types.PowerUserWorkspaceResourceCount {
	if rc == nil {
		return nil
	}
	return &types.PowerUserWorkspaceResourceCount{
		MCPServers:              rc.MCPServers,
		MCPServerCatalogEntries: rc.MCPServerCatalogEntries,
		AccessControlRules:      rc.AccessControlRules,
	}
}