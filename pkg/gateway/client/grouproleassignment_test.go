package client

import (
	"testing"

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
			expected: types2.RoleAdminReadonly,
		},
		{
			name:     "Admin and AdminReadonly combined keeps Admin",
			combined: types2.RoleAdmin | types2.RoleAdminReadonly,
			expected: types2.RoleAdmin,
		},
		{
			name:     "AdminReadonly replaces PowerUserPlus",
			combined: types2.RoleAdminReadonly | types2.RolePowerUserPlus | types2.RoleBasic,
			expected: types2.RoleAdminReadonly,
		},
		{
			name:     "AdminReadonly replaces both power roles",
			combined: types2.RoleAdminReadonly | types2.RolePowerUserPlus | types2.RolePowerUser,
			expected: types2.RoleAdminReadonly,
		},
		{
			name:     "AdminReadonly replaces PowerUser and keeps Auditor",
			combined: types2.RoleAdminReadonly | types2.RolePowerUser | types2.RoleAuditor,
			expected: types2.RoleAdminReadonly | types2.RoleAuditor,
		},
		{
			name:     "AdminReadonly and Basic keeps only AdminReadonly",
			combined: types2.RoleAdminReadonly | types2.RoleBasic,
			expected: types2.RoleAdminReadonly,
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

// TestResolveEffectiveRoleAdminReadonlyReplacesPowerUser checks single and bulk resolution against a database.
func TestResolveEffectiveRoleAdminReadonlyReplacesPowerUser(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	if err := c.db.WithContext(ctx).Create(&types.GroupRoleAssignment{GroupName: "test:readers", Role: types2.RoleAdminReadonly}).Error; err != nil {
		t.Fatalf("failed to create group role assignment: %v", err)
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
			want:   types2.RoleAdminReadonly,
		},
		{
			name:   "power user in the group",
			user:   types.User{ID: 2, Role: types2.RolePowerUser},
			groups: []string{"test:readers"},
			want:   types2.RoleAdminReadonly,
		},
		{
			name:   "basic user in the group",
			user:   types.User{ID: 3, Role: types2.RoleBasic},
			groups: []string{"test:readers"},
			want:   types2.RoleAdminReadonly,
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
