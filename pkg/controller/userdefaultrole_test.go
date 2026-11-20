package controller

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRepairDefaultUserRoleSetting(t *testing.T) {
	ctx := t.Context()
	store := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultRoleSettingName}
	require.NoError(t, ensureDefaultUserRoleSetting(ctx, store))
	var setting v1.UserDefaultRoleSetting
	require.NoError(t, store.Get(ctx, key, &setting))
	require.Equal(t, types.RoleBasic, setting.Spec.Role)
	for role, want := range map[types.Role]types.Role{
		types.RoleAdmin | types.RoleAuditor: types.RoleAdmin | types.RoleAuditor,
		types.RoleOwner:                     types.RoleOwner,
		types.RoleAuditor:                   types.RoleBasic,
		types.RoleAdmin | types.RoleBasic:   types.RoleBasic,
		1:                                   types.RoleAdmin,
		2:                                   types.RolePowerUserPlus,
		3:                                   types.RolePowerUser,
		10:                                  types.RoleBasic,
	} {
		setting.Spec.Role = role
		require.NoError(t, store.Update(ctx, &setting))
		version := setting.ResourceVersion
		require.NoError(t, repairDefaultUserRoleSetting(ctx, store))
		require.NoError(t, store.Get(ctx, key, &setting))
		require.Equal(t, want, setting.Spec.Role, "saved role %d", role)
		if role == want {
			require.Equal(t, version, setting.ResourceVersion)
		} else {
			require.NotEqual(t, version, setting.ResourceVersion)
		}
		version = setting.ResourceVersion
		require.NoError(t, repairDefaultUserRoleSetting(ctx, store))
		require.NoError(t, store.Get(ctx, key, &setting))
		require.Equal(t, version, setting.ResourceVersion)
	}
}
