package client

import (
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestUserInfoByIDEffectiveRole(t *testing.T) {
	c := newTestClient(t)
	db := c.db.WithContext(t.Context())
	u := &types.User{Username: "alice", Email: "alice@example.com", Role: apitypes.RoleBasic}
	require.NoError(t, db.Create(u).Error)
	require.NoError(t, db.Create(&types.Group{ID: "team"}).Error)
	require.NoError(t, db.Create(&types.GroupMemberships{UserID: u.ID, GroupID: "team"}).Error)
	_, err := c.CreateGroupRoleAssignment(t.Context(), "team", apitypes.RoleAdmin, "")
	require.NoError(t, err)

	info, err := c.UserInfoByID(t.Context(), u.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, apitypes.RoleAdmin.Groups(), info.GetGroups())
	require.Equal(t, u.Username, info.GetName())
	require.Equal(t, []string{"team"}, info.GetExtra()["auth_provider_groups"])
	require.Equal(t, []string{u.Email}, info.GetExtra()["email"])

	require.NoError(t, c.DeleteGroupRoleAssignment(t.Context(), "team"))
	info, err = c.UserInfoByID(t.Context(), u.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, apitypes.RoleBasic.Groups(), info.GetGroups())

	// A role lookup failure must not silently return incomplete authorization data.
	require.NoError(t, db.Migrator().DropTable(&types.GroupRoleAssignment{}))
	info, err = c.UserInfoByID(t.Context(), u.ID)
	require.ErrorContains(t, err, "failed to resolve effective role")
	require.Nil(t, info)
}
