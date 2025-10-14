package adminworkspace

import (
	"context"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	types2 "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gatewayClient *gclient.Client
}

func New(gatewayClient *gclient.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

// EnsureAllAdminAndOwnerWorkspaces ensures PowerUserWorkspaces exist for all admin and owner users
// This should be called during controller startup
func (h *Handler) EnsureAllAdminAndOwnerWorkspaces(ctx context.Context, client kclient.Client, namespace string) error {
	admins, err := h.gatewayClient.Users(ctx, types2.UserQuery{
		Role: types.RoleAdmin,
	})
	if err != nil {
		return err
	}

	owners, err := h.gatewayClient.Users(ctx, types2.UserQuery{
		Role: types.RoleOwner,
	})
	if err != nil {
		return err
	}

	for _, user := range append(admins, owners...) {
		if err := h.ensureAdminOrOwnerWorkspace(ctx, client, namespace, &user); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) ensureAdminOrOwnerWorkspace(ctx context.Context, client kclient.Client, namespace string, user *types2.User) error {
	userIDStr := strconv.Itoa(int(user.ID))

	// Check if user already has a PowerUserWorkspace
	var existingWorkspaces v1.PowerUserWorkspaceList
	if err := client.List(ctx, &existingWorkspaces, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userIDStr,
		}),
	}); err != nil {
		return err
	}

	// If workspace already exists, nothing to do
	if len(existingWorkspaces.Items) > 0 {
		return nil
	}

	// Create PowerUserWorkspace directly for the user
	workspace := &v1.PowerUserWorkspace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      system.GetPowerUserWorkspaceID(userIDStr),
		},
		Spec: v1.PowerUserWorkspaceSpec{
			UserID: userIDStr,
			Role:   user.Role,
		},
	}

	return client.Create(ctx, workspace)
}
