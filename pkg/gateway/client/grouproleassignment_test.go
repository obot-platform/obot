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
			"Single Owner",
			types2.RoleOwner,
			types2.RoleOwner,
		},
		{
			"Single Admin",
			types2.RoleAdmin,
			types2.RoleAdmin,
		},
		{
			"Single Basic",
			types2.RoleBasic,
			types2.RoleBasic,
		},
		{
			"Owner and Admin combined keeps Owner",
			types2.RoleOwner | types2.RoleAdmin,
			types2.RoleOwner,
		},
		{
			"Admin and PowerUser combined keeps Admin",
			types2.RoleAdmin | types2.RolePowerUser,
			types2.RoleAdmin,
		},
		{
			"PowerUserPlus and PowerUser keeps PowerUserPlus",
			types2.RolePowerUserPlus | types2.RolePowerUser,
			types2.RolePowerUserPlus,
		},
		{
			"All base roles keeps Owner",
			types2.RoleOwner | types2.RoleAdmin | types2.RolePowerUserPlus | types2.RolePowerUser | types2.RoleBasic,
			types2.RoleOwner,
		},
		{
			"Auditor preserved with Admin",
			types2.RoleAdmin | types2.RoleAuditor,
			types2.RoleAdmin | types2.RoleAuditor,
		},
		{
			"Auditor preserved when merging Owner and Admin",
			types2.RoleOwner | types2.RoleAdmin | types2.RoleAuditor,
			types2.RoleOwner | types2.RoleAuditor,
		},
		{
			"UserImpersonation preserved with Admin",
			types2.RoleAdmin | types2.RoleUserImpersonation,
			types2.RoleAdmin | types2.RoleUserImpersonation,
		},
		{
			"UserImpersonation preserved when merging Owner and Admin",
			types2.RoleOwner | types2.RoleAdmin | types2.RoleUserImpersonation,
			types2.RoleOwner | types2.RoleUserImpersonation,
		},
		{
			"Both Auditor and UserImpersonation preserved",
			types2.RoleAdmin | types2.RoleAuditor | types2.RoleUserImpersonation,
			types2.RoleAdmin | types2.RoleAuditor | types2.RoleUserImpersonation,
		},
		{
			"All add-ons preserved when merging multiple base roles",
			types2.RoleOwner | types2.RoleAdmin | types2.RolePowerUser | types2.RoleAuditor | types2.RoleUserImpersonation,
			types2.RoleOwner | types2.RoleAuditor | types2.RoleUserImpersonation,
		},
		{
			"Auditor alone normalizes to Basic with Auditor",
			types2.RoleAuditor,
			types2.RoleBasic | types2.RoleAuditor,
		},
		{
			"UserImpersonation alone normalizes to Basic with UserImpersonation",
			types2.RoleUserImpersonation,
			types2.RoleBasic | types2.RoleUserImpersonation,
		},
		{
			"Zero role normalizes to Basic",
			0,
			types2.RoleBasic,
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
