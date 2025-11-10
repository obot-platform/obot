package server

import (
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

func TestExtractBaseRole(t *testing.T) {
	tests := []struct {
		name     string
		role     types2.Role
		expected types2.Role
	}{
		{"Admin only", types2.RoleAdmin, types2.RoleAdmin},
		{"Admin with Auditor", types2.RoleAdmin | types2.RoleAuditor, types2.RoleAdmin},
		{"Owner only", types2.RoleOwner, types2.RoleOwner},
		{"Owner with Auditor", types2.RoleOwner | types2.RoleAuditor, types2.RoleOwner},
		{"Auditor only", types2.RoleAuditor, 0},
		{"PowerUser with Auditor", types2.RolePowerUser | types2.RoleAuditor, types2.RolePowerUser},
		{"Owner and Admin", types2.RoleOwner | types2.RoleAdmin, types2.RoleOwner | types2.RoleAdmin},
		{"Owner and Admin with Auditor", types2.RoleOwner | types2.RoleAdmin | types2.RoleAuditor, types2.RoleOwner | types2.RoleAdmin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBaseRole(tt.role)
			if result != tt.expected {
				t.Errorf("extractBaseRole(%d) = %d, want %d", tt.role, result, tt.expected)
			}
		})
	}
}

func TestHasAuditorRole(t *testing.T) {
	tests := []struct {
		name     string
		role     types2.Role
		expected bool
	}{
		{"Admin only", types2.RoleAdmin, false},
		{"Admin with Auditor", types2.RoleAdmin | types2.RoleAuditor, true},
		{"Auditor only", types2.RoleAuditor, true},
		{"Owner only", types2.RoleOwner, false},
		{"Owner with Auditor", types2.RoleOwner | types2.RoleAuditor, true},
		{"PowerUser only", types2.RolePowerUser, false},
		{"Owner and Admin", types2.RoleOwner | types2.RoleAdmin, false},
		{"Owner and Admin with Auditor", types2.RoleOwner | types2.RoleAdmin | types2.RoleAuditor, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasAuditorRole(tt.role)
			if result != tt.expected {
				t.Errorf("hasAuditorRole(%d) = %v, want %v", tt.role, result, tt.expected)
			}
		})
	}
}
