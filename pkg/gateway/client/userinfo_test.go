package client

import (
	"strings"
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

	// No agent has the user's ID, so none is found.
	info, err = c.IdentityInfo(t.Context(), principal.Agent(u.ID))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Nil(t, info)

	_, err = c.IdentityInfo(t.Context(), principal.Principal{})
	require.Error(t, err)
}

func TestIdentityInfoOfAnAgent(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	agent, err := c.CreateAgent(ctx, types.Agent{
		DisplayName: "builder",
	})
	require.NoError(t, err)
	p := principal.Agent(agent.ID)

	for _, step := range []struct {
		name       string
		change     func() error
		wantStatus apitypes.UserStatus
	}{
		{
			name:       "active",
			change:     func() error { return nil },
			wantStatus: apitypes.UserStatusActive,
		},
		{
			name: "disabled",
			change: func() error {
				_, err := c.DisableAgent(ctx, agent.ID, "rotating keys")
				return err
			},
			wantStatus: apitypes.UserStatusDisabled,
		},
	} {
		require.NoError(t, step.change(), step.name)

		info, err := c.IdentityInfo(ctx, p)
		require.NoError(t, err, step.name)
		require.Equal(t, "agent:"+p.ID, info.GetUID(), step.name)
		require.Equal(t, info.GetUID(), info.GetName(), step.name)
		require.Empty(t, info.GetGroups(), step.name)
		got, ok := principal.FromUserInfo(info)
		require.True(t, ok, step.name)
		require.Equal(t, p, got, step.name)
		status, recorded := principal.UserStatus(info)
		require.True(t, recorded, step.name)
		require.Equal(t, step.wantStatus, status, step.name)
		for _, key := range []string{"email", "obot_groups", "auth_provider_groups"} {
			require.NotContains(t, info.GetExtra(), key, step.name)
		}
	}

	// A deleted agent is not found, as a deleted user is not.
	require.NoError(t, c.DeleteAgent(ctx, agent.ID))
	info, err := c.IdentityInfo(ctx, p)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Nil(t, info)

	_, err = c.IdentityInfo(ctx, principal.Agent(agent.ID+1))
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestIdentityInfoRefusesDelegatedOwners checks that the owner of a user's setup through an agent is never loaded as an
// identity: it is not a principal, and no principal built from its parts names the agent or the user.
func TestIdentityInfoRefusesDelegatedOwners(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	agent, err := c.CreateAgent(ctx, types.Agent{DisplayName: "assistant"})
	require.NoError(t, err)
	u := &types.User{Username: "alice", Email: "alice@example.com", Role: apitypes.RoleBasic}
	require.NoError(t, c.db.WithContext(ctx).Create(u).Error)

	owner := principal.DelegatedOwner{AgentID: agent.ID, UserID: u.ID}.String()
	_, err = principal.Parse(owner)
	require.Error(t, err)

	for _, p := range []principal.Principal{
		{Kind: principal.KindAgent, ID: strings.TrimPrefix(owner, "agent:")},
		{Kind: principal.KindUser, ID: owner},
		{Kind: principal.KindUser, ID: strings.TrimPrefix(owner, "agent:")},
	} {
		info, err := c.IdentityInfo(ctx, p)
		require.Error(t, err, "IdentityInfo(%#v)", p)
		require.Nil(t, info)
	}
}
