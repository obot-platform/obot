package client

import (
	"context"
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPostgresSignInPreservesConcurrentRoleChanges(t *testing.T) {
	for _, tt := range []struct {
		name           string
		changesProfile bool
		requestedRole  apitypes.Role
		wantRole       apitypes.Role
	}{
		{
			name:     "daily activity write",
			wantRole: apitypes.RoleBasic,
		},
		{
			name:           "profile write",
			changesProfile: true,
			wantRole:       apitypes.RoleBasic,
		},
		{
			name:          "explicit role raise",
			requestedRole: apitypes.RolePowerUserPlus,
			wantRole:      apitypes.RolePowerUserPlus,
		},
		{
			name:           "profile write and explicit role raise",
			changesProfile: true,
			requestedRole:  apitypes.RolePowerUserPlus,
			wantRole:       apitypes.RolePowerUserPlus,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newPostgresLifecycleTestClient(t)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			db := c.db.WithContext(ctx)

			initial := types.Identity{
				AuthProviderName:      lifecycleTestLocalProvider.Name,
				AuthProviderNamespace: lifecycleTestLocalProvider.Namespace,
				ProviderUsername:      "viewer",
				ProviderUserID:        "viewer",
				Email:                 "viewer@example.test",
			}
			signIn := func(id types.Identity, role apitypes.Role) ensuredIdentity {
				t.Helper()
				var result ensuredIdentity
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
					var err error
					result, err = c.ensureIdentity(ctx, tx, &id, "", role, UserLimit{Unlimited: true})
					return err
				}))
				return result
			}
			originalRole := apitypes.RoleBasic | apitypes.RoleAdminReadonly
			user := signIn(initial, originalRole).user
			require.Equal(t, originalRole, user.Role)
			lastActive := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -2)
			require.NoError(t, db.Model(new(types.User)).Where("id = ?", user.ID).
				UpdateColumn("last_active_day", lastActive).Error)

			group := types.Group{
				ID:                    "test:admins",
				Name:                  "Admins",
				AuthProviderName:      initial.AuthProviderName,
				AuthProviderNamespace: initial.AuthProviderNamespace,
			}
			require.NoError(t, db.Create(&group).Error)
			require.NoError(t, db.Create(&types.GroupMemberships{
				UserID:  user.ID,
				GroupID: group.ID,
			}).Error)
			_, err := c.CreateGroupRoleAssignment(ctx, group.ID, apitypes.RoleBasic, "")
			require.NoError(t, err)

			// The sign-in query has opened its result set. Promote the group in a separate
			// transaction before sign-in can write anything from its user snapshot.
			promoted := false
			require.NoError(t, db.Callback().Row().After("gorm:row").Register("promote_group_after_sign_in_read", func(query *gorm.DB) {
				if promoted || query.Error != nil || !strings.Contains(query.Statement.SQL.String(), "AS scim_bound_to_connection") {
					return
				}
				promoted = true
				_, err := c.UpdateGroupRoleAssignment(ctx, group.ID, apitypes.RoleAdmin, "", true)
				if err != nil {
					_ = query.AddError(err)
				}
			}))

			incoming := initial
			if tt.changesProfile {
				incoming.ProviderUsername = "renamed"
				incoming.Email = "renamed@example.test"
			}
			require.Equal(t, tt.requestedRole != apitypes.RoleUnknown, signIn(incoming, tt.requestedRole).roleRaised)
			require.True(t, promoted, "group promotion must run between the sign-in read and write")

			var stored types.User
			require.NoError(t, db.First(&stored, user.ID).Error)
			require.Equal(t, tt.wantRole, stored.Role)
			require.Equal(t, incoming.ProviderUsername, stored.Username)
			require.Equal(t, incoming.Email, stored.Email)
			require.True(t, stored.LastActiveDay.After(lastActive))
		})
	}
}
