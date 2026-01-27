package server

import (
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestConvertGroupRoleAssignment(t *testing.T) {
	tests := []struct {
		name       string
		assignment *types.GroupRoleAssignment
		want       types2.GroupRoleAssignment
	}{
		{
			name: "basic assignment",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "engineering",
				Role:        types2.RoleAdmin,
				Description: "Engineering team",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "engineering",
				Role:        types2.RoleAdmin,
				Description: "Engineering team",
			},
		},
		{
			name: "owner role",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "admins",
				Role:        types2.RoleOwner,
				Description: "Admin group",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "admins",
				Role:        types2.RoleOwner,
				Description: "Admin group",
			},
		},
		{
			name: "power user role",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "developers",
				Role:        types2.RolePowerUser,
				Description: "Developer access",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "developers",
				Role:        types2.RolePowerUser,
				Description: "Developer access",
			},
		},
		{
			name: "power user plus role",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "senior-devs",
				Role:        types2.RolePowerUserPlus,
				Description: "Senior developer access",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "senior-devs",
				Role:        types2.RolePowerUserPlus,
				Description: "Senior developer access",
			},
		},
		{
			name: "auditor role",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "auditors",
				Role:        types2.RoleAuditor,
				Description: "Auditor team",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "auditors",
				Role:        types2.RoleAuditor,
				Description: "Auditor team",
			},
		},
		{
			name: "combined owner and auditor roles",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "super-admins",
				Role:        types2.RoleOwner | types2.RoleAuditor,
				Description: "Super admin group",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "super-admins",
				Role:        types2.RoleOwner | types2.RoleAuditor,
				Description: "Super admin group",
			},
		},
		{
			name: "empty description",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "test-group",
				Role:        types2.RoleAdmin,
				Description: "",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "test-group",
				Role:        types2.RoleAdmin,
				Description: "",
			},
		},
		{
			name: "group name with special characters",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "github/org-team",
				Role:        types2.RolePowerUser,
				Description: "GitHub org team",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "github/org-team",
				Role:        types2.RolePowerUser,
				Description: "GitHub org team",
			},
		},
		{
			name: "long description",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "team-alpha",
				Role:        types2.RoleAdmin,
				Description: "This is a very long description that contains multiple sentences and provides detailed information about the group role assignment and its purpose within the organization.",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "team-alpha",
				Role:        types2.RoleAdmin,
				Description: "This is a very long description that contains multiple sentences and provides detailed information about the group role assignment and its purpose within the organization.",
			},
		},
		{
			name: "unknown role",
			assignment: &types.GroupRoleAssignment{
				GroupName:   "unknown-group",
				Role:        types2.RoleUnknown,
				Description: "Unknown role",
			},
			want: types2.GroupRoleAssignment{
				GroupName:   "unknown-group",
				Role:        types2.RoleUnknown,
				Description: "Unknown role",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertGroupRoleAssignment(tt.assignment)

			if got.GroupName != tt.want.GroupName {
				t.Errorf("GroupName = %v, want %v", got.GroupName, tt.want.GroupName)
			}
			if got.Role != tt.want.Role {
				t.Errorf("Role = %v, want %v", got.Role, tt.want.Role)
			}
			if got.Description != tt.want.Description {
				t.Errorf("Description = %v, want %v", got.Description, tt.want.Description)
			}
		})
	}
}
