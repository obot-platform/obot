package poweruserworkspace

import (
	"fmt"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadonlyDowngradeUsesStandardUserWorkspaceCleanup(t *testing.T) {
	for _, original := range []types.Role{types.RolePowerUser, types.RolePowerUserPlus, types.RoleAdmin} {
		for _, target := range []types.Role{types.RoleBasic, types.RoleAdminReadonly, types.RoleAdminReadonly | types.RoleAuditor} {
			t.Run(fmt.Sprintf("%d-to-%d", original, target), func(t *testing.T) {
				workspace := &v1.PowerUserWorkspace{
					Name:      "puw1-7",
					Namespace: "default",
					Spec: v1.PowerUserWorkspaceSpec{
						UserID: "7",
						Role:   original,
					},
				}
				storage := fake.NewClientBuilder().
					WithScheme(storagescheme.Scheme).
					WithObjects(workspace).
					WithIndex(&v1.PowerUserWorkspace{}, "spec.userID", func(obj kclient.Object) []string {
						return []string{obj.(*v1.PowerUserWorkspace).Spec.UserID}
					}).Build()
				h := &Handler{}
				require.NoError(t, h.reconcileWorkspace(t.Context(), storage, "default", gatewaytypes.User{ID: 7}, target))
				var workspaces v1.PowerUserWorkspaceList
				require.NoError(t, storage.List(t.Context(), &workspaces, kclient.InNamespace("default")))
				require.Empty(t, workspaces.Items)
			})
		}
	}
}
