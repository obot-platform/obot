package client

import (
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
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
			name:     "UserImpersonation preserved with Admin",
			combined: types2.RoleAdmin | types2.RoleUserImpersonation,
			expected: types2.RoleAdmin | types2.RoleUserImpersonation,
		},
		{
			name:     "UserImpersonation preserved when merging Owner and Admin",
			combined: types2.RoleOwner | types2.RoleAdmin | types2.RoleUserImpersonation,
			expected: types2.RoleOwner | types2.RoleUserImpersonation,
		},
		{
			name:     "Both Auditor and UserImpersonation preserved",
			combined: types2.RoleAdmin | types2.RoleAuditor | types2.RoleUserImpersonation,
			expected: types2.RoleAdmin | types2.RoleAuditor | types2.RoleUserImpersonation,
		},
		{
			name:     "All add-ons preserved when merging multiple base roles",
			combined: types2.RoleOwner | types2.RoleAdmin | types2.RolePowerUser | types2.RoleAuditor | types2.RoleUserImpersonation,
			expected: types2.RoleOwner | types2.RoleAuditor | types2.RoleUserImpersonation,
		},
		{
			name:     "Auditor alone normalizes to Basic with Auditor",
			combined: types2.RoleAuditor,
			expected: types2.RoleBasic | types2.RoleAuditor,
		},
		{
			name:     "UserImpersonation alone normalizes to Basic with UserImpersonation",
			combined: types2.RoleUserImpersonation,
			expected: types2.RoleBasic | types2.RoleUserImpersonation,
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
