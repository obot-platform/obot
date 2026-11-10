package client

import (
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserInfoByIDEffectiveRole(t *testing.T) {
	c := newTestClient(t)
	db := c.db.WithContext(t.Context())
	u := &types.User{Username: "alice", Email: "alice@example.com", Role: apitypes.RoleBasic}
	require.NoError(t, db.Create(u).Error)
	require.NoError(t, db.Create(&types.Group{ID: "team"}).Error)
	require.NoError(t, db.Create(&types.GroupMemberships{UserID: u.ID, GroupID: "team"}).Error)
	_, err := c.CreateGroupRoleAssignment(t.Context(), "team", apitypes.RoleAdminReadonly, "")
	require.NoError(t, err)

	info, err := c.UserInfoByID(t.Context(), u.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, (apitypes.RoleBasic | apitypes.RoleAdminReadonly).Groups(), info.GetGroups())
	require.ElementsMatch(t, (apitypes.RoleBasic | apitypes.RoleAdminReadonly).Groups(), info.GetExtra()["obot_groups"])
	require.Equal(t, u.Username, info.GetName())
	require.Equal(t, []string{"team"}, info.GetExtra()["auth_provider_groups"])
	require.Equal(t, []string{u.Email}, info.GetExtra()["email"])

	require.NoError(t, c.DeleteGroupRoleAssignment(t.Context(), "team", false))
	info, err = c.UserInfoByID(t.Context(), u.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, apitypes.RoleBasic.Groups(), info.GetGroups())
	require.ElementsMatch(t, apitypes.RoleBasic.Groups(), info.GetExtra()["obot_groups"])

	// A role lookup failure must not silently return incomplete authorization data.
	require.NoError(t, db.Migrator().DropTable(&types.GroupRoleAssignment{}))
	info, err = c.UserInfoByID(t.Context(), u.ID)
	require.ErrorContains(t, err, "failed to resolve effective role")
	require.Nil(t, info)
}

func TestIdentityInfo(t *testing.T) {
	c := newTestClient(t)
	u := &types.User{Username: "alice", Email: "alice@example.com", Role: apitypes.RoleBasic}
	require.NoError(t, c.db.WithContext(t.Context()).Create(u).Error)

	want, err := c.UserInfoByID(t.Context(), u.ID)
	require.NoError(t, err)
	info, err := c.IdentityInfo(t.Context(), principal.User(u.ID))
	require.NoError(t, err)
	require.Equal(t, want, info)
	got, ok := principal.FromUserInfo(info)
	require.True(t, ok)
	require.Equal(t, principal.User(u.ID), got)

	_, err = c.IdentityInfo(t.Context(), principal.User(u.ID+1))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	// Agents are not stored yet, so no agent is found.
	info, err = c.IdentityInfo(t.Context(), principal.Agent(u.ID))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Nil(t, info)

	_, err = c.IdentityInfo(t.Context(), principal.Principal{})
	require.Error(t, err)
}
