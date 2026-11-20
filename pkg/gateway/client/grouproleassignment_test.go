package client

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestNormalizeToHighestRole(t *testing.T) {
	tests := []struct {
		name     string
		combined types2.Role
		expected types2.Role
	}{
		{
			name:     "Single Owner",
			combined: types2.RoleOwner,
			expected: types2.RoleOwner,
		},
		{
			name:     "Single Admin",
			combined: types2.RoleAdmin,
			expected: types2.RoleAdmin,
		},
		{
			name:     "Single AdminReadonly",
			combined: types2.RoleAdminReadonly,
			expected: types2.RoleAdminReadonly | types2.RoleBasic,
		},
		{
			name:     "Admin and AdminReadonly combined keeps Admin",
			combined: types2.RoleAdmin | types2.RoleAdminReadonly,
			expected: types2.RoleAdmin,
		},
		{
			name:     "AdminReadonly preserves PowerUserPlus",
			combined: types2.RoleAdminReadonly | types2.RolePowerUserPlus | types2.RoleBasic,
			expected: types2.RoleAdminReadonly | types2.RolePowerUserPlus,
		},
		{
			name:     "AdminReadonly preserves the higher power role",
			combined: types2.RoleAdminReadonly | types2.RolePowerUserPlus | types2.RolePowerUser,
			expected: types2.RoleAdminReadonly | types2.RolePowerUserPlus,
		},
		{
			name:     "AdminReadonly preserves PowerUser and keeps Auditor",
			combined: types2.RoleAdminReadonly | types2.RolePowerUser | types2.RoleAuditor,
			expected: types2.RoleAdminReadonly | types2.RolePowerUser | types2.RoleAuditor,
		},
		{
			name:     "AdminReadonly and Basic keeps both bits",
			combined: types2.RoleAdminReadonly | types2.RoleBasic,
			expected: types2.RoleAdminReadonly | types2.RoleBasic,
		},
		{
			name:     "Single Basic",
			combined: types2.RoleBasic,
			expected: types2.RoleBasic,
		},
		{
			name:     "Owner and Admin combined keeps Owner",
			combined: types2.RoleOwner | types2.RoleAdmin,
			expected: types2.RoleOwner,
		},
		{
			name:     "Admin and PowerUser combined keeps Admin",
			combined: types2.RoleAdmin | types2.RolePowerUser,
			expected: types2.RoleAdmin,
		},
		{
			name:     "PowerUserPlus and PowerUser keeps PowerUserPlus",
			combined: types2.RolePowerUserPlus | types2.RolePowerUser,
			expected: types2.RolePowerUserPlus,
		},
		{
			name:     "All base roles keeps Owner",
			combined: types2.RoleOwner | types2.RoleAdmin | types2.RolePowerUserPlus | types2.RolePowerUser | types2.RoleBasic,
			expected: types2.RoleOwner,
		},
		{
			name:     "Auditor preserved with Admin",
			combined: types2.RoleAdmin | types2.RoleAuditor,
			expected: types2.RoleAdmin | types2.RoleAuditor,
		},
		{
			name:     "Auditor preserved when merging Owner and Admin",
			combined: types2.RoleOwner | types2.RoleAdmin | types2.RoleAuditor,
			expected: types2.RoleOwner | types2.RoleAuditor,
		},
		{
			name:     "All add-ons preserved when merging multiple base roles",
			combined: types2.RoleOwner | types2.RoleAdmin | types2.RolePowerUser | types2.RoleAuditor,
			expected: types2.RoleOwner | types2.RoleAuditor,
		},
		{
			name:     "Auditor alone normalizes to Basic with Auditor",
			combined: types2.RoleAuditor,
			expected: types2.RoleBasic | types2.RoleAuditor,
		},
		{
			name:     "Zero role normalizes to Basic",
			combined: 0,
			expected: types2.RoleBasic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeToHighestRole(tt.combined)
			if result != tt.expected {
				t.Errorf("normalizeToHighestRole(%d) = %d, want %d", tt.combined, result, tt.expected)
			}
		})
	}
}

// TestResolveEffectiveRoleAdminReadonlyPreservesBaseRole checks single and bulk resolution against a database.
func TestResolveEffectiveRoleAdminReadonlyPreservesBaseRole(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	if err := c.db.WithContext(ctx).Create(&types.GroupRoleAssignment{GroupName: "test:readers", Role: types2.RoleAdminReadonly}).Error; err != nil {
		t.Fatalf("failed to create group role assignment: %v", err)
	}

	if err := c.db.WithContext(ctx).Create(&types.GroupRoleAssignment{
		GroupName: "test:auditors",
		Role:      types2.RoleAuditor | types2.RoleAdminReadonly,
	}).Error; err != nil {
		t.Fatalf("failed to create second group role assignment: %v", err)
	}

	tests := []struct {
		name   string
		user   types.User
		groups []string
		want   types2.Role
	}{
		{
			name:   "power user plus in the group",
			user:   types.User{ID: 1, Role: types2.RolePowerUserPlus},
			groups: []string{"test:readers"},
			want:   types2.RolePowerUserPlus | types2.RoleAdminReadonly,
		},
		{
			name:   "power user in the group",
			user:   types.User{ID: 2, Role: types2.RolePowerUser},
			groups: []string{"test:readers"},
			want:   types2.RolePowerUser | types2.RoleAdminReadonly,
		},
		{
			name:   "basic user in the group",
			user:   types.User{ID: 3, Role: types2.RoleBasic},
			groups: []string{"test:readers"},
			want:   types2.RoleBasic | types2.RoleAdminReadonly,
		},
		{
			name:   "admin in the group",
			user:   types.User{ID: 4, Role: types2.RoleAdmin},
			groups: []string{"test:readers"},
			want:   types2.RoleAdmin,
		},
		{
			name:   "power user plus outside the group",
			user:   types.User{ID: 5, Role: types2.RolePowerUserPlus},
			groups: []string{"test:others"},
			want:   types2.RolePowerUserPlus,
		},
		{
			name:   "multiple groups independently grant the add-on",
			user:   types.User{ID: 6, Role: types2.RoleBasic},
			groups: []string{"test:readers", "test:auditors"},
			want:   types2.RoleBasic | types2.RoleAdminReadonly | types2.RoleAuditor,
		},
		{
			name:   "direct grant survives loss of group grant",
			user:   types.User{ID: 7, Role: types2.RoleBasic | types2.RoleAdminReadonly},
			groups: []string{"test:others"},
			want:   types2.RoleBasic | types2.RoleAdminReadonly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.ResolveUserEffectiveRole(ctx, &tt.user, tt.groups)
			if err != nil {
				t.Fatalf("ResolveUserEffectiveRole() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveUserEffectiveRole() = %d, want %d", got, tt.want)
			}

			bulk, err := c.ResolveUserEffectiveRolesBulk(ctx, []types.User{tt.user}, map[uint][]string{tt.user.ID: tt.groups})
			if err != nil {
				t.Fatalf("ResolveUserEffectiveRolesBulk() error = %v", err)
			}
			if bulk[tt.user.ID] != tt.want {
				t.Errorf("ResolveUserEffectiveRolesBulk() = %d, want %d", bulk[tt.user.ID], tt.want)
			}
		})
	}
}

func TestAdminCanChangeReadOnlyAdminWithoutChangingAuditorGrant(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	const group = "test:readers"

	_, err := c.CreateGroupRoleAssignment(ctx, group, types2.RoleAuditor, "")
	require.NoError(t, err)

	both := types2.RoleAuditor | types2.RoleAdminReadonly
	assignment, err := c.UpdateGroupRoleAssignment(ctx, group, both, "", false)
	require.NoError(t, err)
	require.Equal(t, both, assignment.Role)

	_, err = c.UpdateGroupRoleAssignment(ctx, group, types2.RoleAdminReadonly, "", false)
	require.ErrorIs(t, err, ErrGroupRoleAssignmentOwnerRequired)
	require.ErrorIs(t, c.DeleteGroupRoleAssignment(ctx, group, false), ErrGroupRoleAssignmentOwnerRequired)

	assignment, err = c.UpdateGroupRoleAssignment(ctx, group, types2.RoleAuditor, "", false)
	require.NoError(t, err)
	require.Equal(t, types2.RoleAuditor, assignment.Role)
	require.NoError(t, c.DeleteGroupRoleAssignment(ctx, group, true))

	_, err = c.CreateGroupRoleAssignment(ctx, group, types2.RoleAdminReadonly, "")
	require.NoError(t, err)
	_, err = c.UpdateGroupRoleAssignment(ctx, group, both, "", false)
	require.ErrorIs(t, err, ErrGroupRoleAssignmentOwnerRequired)
	require.NoError(t, c.DeleteGroupRoleAssignment(ctx, group, false))
}

func TestReadonlyAdminDirectGrantOnDemotion(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	db := c.db.WithContext(ctx)
	user := &types.User{
		Username:       "admin",
		HashedUsername: "admin-username",
		Email:          "admin@example.com",
		HashedEmail:    "admin-email",
		Role:           types2.RoleAdmin,
	}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(&types.User{
		Username:       "other-admin",
		HashedUsername: "other-admin-username",
		Email:          "other-admin@example.com",
		HashedEmail:    "other-admin-email",
		Role:           types2.RoleAdmin,
	}).Error)

	role := types2.RoleBasic | types2.RoleAdminReadonly
	updated, err := c.UpdateUser(ctx, true, &types.User{Role: role}, strconv.FormatUint(uint64(user.ID), 10))
	require.NoError(t, err)
	require.Equal(t, role, updated.Role)
}

func TestReadonlyAdminDirectGrantClearedOnPromotion(t *testing.T) {
	for _, via := range []string{"direct", "direct owner", "mapping create", "mapping update", "IdP membership", "SCIM membership"} {
		t.Run(via, func(t *testing.T) {
			c := newTestClient(t)
			ctx := t.Context()
			db := c.db.WithContext(ctx)
			u := &types.User{
				Username: "viewer",
				Email:    "viewer@example.com",
				Role:     types2.RoleBasic | types2.RoleAuditor | types2.RoleAdminReadonly,
			}
			require.NoError(t, db.Create(u).Error)
			group := types.Group{
				ID:   "test:admins",
				Name: "Administrators",
			}
			require.NoError(t, db.Create(&group).Error)

			switch via {
			case "direct":
				_, err := c.UpdateUser(ctx, true, &types.User{
					Role: types2.RoleAdmin | types2.RoleAuditor | types2.RoleAdminReadonly,
				}, strconv.FormatUint(uint64(u.ID), 10))
				require.NoError(t, err)
			case "direct owner":
				_, err := c.UpdateUser(ctx, true, &types.User{
					Role: types2.RoleOwner | types2.RoleAuditor | types2.RoleAdminReadonly,
				}, strconv.FormatUint(uint64(u.ID), 10))
				require.NoError(t, err)
			case "mapping create", "mapping update":
				require.NoError(t, db.Create(&types.GroupMemberships{
					GroupID: group.ID,
					UserID:  u.ID,
				}).Error)
				role := types2.RoleAdmin
				if via == "mapping update" {
					role = types2.RolePowerUserPlus
				}
				_, err := c.CreateGroupRoleAssignment(ctx, group.ID, role, "")
				require.NoError(t, err)
				if via == "mapping update" {
					_, err := c.UpdateGroupRoleAssignment(ctx, group.ID, types2.RoleAdmin, "", true)
					require.NoError(t, err)
				}
			case "IdP membership", "SCIM membership":
				_, err := c.CreateGroupRoleAssignment(ctx, group.ID, types2.RoleAdmin, "")
				require.NoError(t, err)
				require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
					if via == "SCIM membership" {
						return writeGroupMembershipChangesTx(tx, group.ID, map[uint]bool{u.ID: false})
					}
					_, _, err := c.ensureGroupMemberships(ctx, tx, &types.Identity{
						UserID:             u.ID,
						AuthProviderGroups: []types.Group{group},
					})
					return err
				}))
			}
			id := strconv.FormatUint(uint64(u.ID), 10)
			stored, err := c.UserByID(ctx, id)
			require.NoError(t, err)
			require.True(t, stored.Role.HasRole(types2.RoleAuditor))
			require.False(t, stored.Role.HasRole(types2.RoleAdminReadonly))
			groupIDs, err := c.ListGroupIDsForUser(ctx, u.ID)
			require.NoError(t, err)
			role, err := c.ResolveUserEffectiveRole(ctx, stored, groupIDs)
			require.NoError(t, err)
			require.True(t, role.HasRole(types2.RoleAdmin))
			require.False(t, role.HasRole(types2.RoleAdminReadonly))

			_, err = c.UpdateUser(ctx, true, &types.User{
				Role: types2.RoleBasic | types2.RoleAuditor | types2.RoleAdminReadonly,
			}, id)
			switch via {
			case "direct":
				require.ErrorAs(t, err, new(*LastAdminError))
			case "direct owner":
				require.ErrorAs(t, err, new(*LastOwnerError))
			default:
				var httpErr *types2.ErrHTTP
				require.ErrorAs(t, err, &httpErr)
				require.Equal(t, http.StatusBadRequest, httpErr.Code)
				require.ErrorContains(t, err, "already inherit Read-Only Admin access")
			}

			if len(groupIDs) > 0 {
				require.NoError(t, c.DeleteGroupRoleAssignment(ctx, group.ID, true))
				role, err = c.ResolveUserEffectiveRole(ctx, stored, groupIDs)
				require.NoError(t, err)
				require.Equal(t, types2.RoleBasic|types2.RoleAuditor, role)

				const reviewers = "test:reviewers"
				_, err = c.CreateGroupRoleAssignment(ctx, reviewers, types2.RoleAdminReadonly, "")
				require.NoError(t, err)
				_, err = c.CreateGroupRoleAssignment(ctx, group.ID, types2.RoleAdmin, "")
				require.NoError(t, err)
				overlapping := append(groupIDs, reviewers)
				role, err = c.ResolveUserEffectiveRole(ctx, stored, overlapping)
				require.NoError(t, err)
				require.Equal(t, types2.RoleAdmin|types2.RoleAuditor, role)
				require.NoError(t, c.DeleteGroupRoleAssignment(ctx, group.ID, true))
				role, err = c.ResolveUserEffectiveRole(ctx, stored, overlapping)
				require.NoError(t, err)
				require.Equal(t, types2.RoleBasic|types2.RoleAuditor|types2.RoleAdminReadonly, role)
			}
		})
	}
}
