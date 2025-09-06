package handlers

import (
	"fmt"

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

// List returns power user workspaces. Admins see all, non-admins see only their own.
func (*PowerUserWorkspaceHandler) List(req api.Context) error {
	var list v1.PowerUserWorkspaceList
	if req.UserIsAdmin() {
		// Admins can see all PowerUserWorkspaces
		if err := req.List(&list); err != nil {
			return fmt.Errorf("failed to list power user workspaces: %w", err)
		}
	} else {
		// Non-admins can only see their own workspace
		userID := req.User.GetUID()
		if err := req.List(&list, &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.userID": userID,
			}),
		}); err != nil {
			return fmt.Errorf("failed to list power user workspaces: %w", err)
		}
	}

	items := make([]types.PowerUserWorkspace, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, convertPowerUserWorkspace(item))
	}

	return req.Write(types.PowerUserWorkspaceList{
		Items: items,
	})
}

// Get returns a specific power user workspace by ID.
func (*PowerUserWorkspaceHandler) Get(req api.Context) error {
	var workspace v1.PowerUserWorkspace
	if err := req.Get(&workspace, req.PathValue("workspace_id")); err != nil {
		return fmt.Errorf("failed to get power user workspace: %w", err)
	}

	return req.Write(convertPowerUserWorkspace(workspace))
}

func convertPowerUserWorkspace(workspace v1.PowerUserWorkspace) types.PowerUserWorkspace {
	return types.PowerUserWorkspace{
		Metadata: MetadataFrom(&workspace),
		UserID:   workspace.Spec.UserID,
		Role:     workspace.Spec.Role,
	}
}
