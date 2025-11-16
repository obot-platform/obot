package types

import (
	"testing"
)

func TestExtractBaseRole(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected Role
	}{
		{"Admin only", RoleAdmin, RoleAdmin},
		{"Admin with Auditor", RoleAdmin | RoleAuditor, RoleAdmin},
		{"Owner only", RoleOwner, RoleOwner},
		{"Owner with Auditor", RoleOwner | RoleAuditor, RoleOwner},
		{"Auditor only", RoleAuditor, 0},
		{"PowerUser with Auditor", RolePowerUser | RoleAuditor, RolePowerUser},
		{"Owner and Admin", RoleOwner | RoleAdmin, RoleOwner | RoleAdmin},
		{"Owner and Admin with Auditor", RoleOwner | RoleAdmin | RoleAuditor, RoleOwner | RoleAdmin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role.ExtractBaseRole()
			if result != tt.expected {
				t.Errorf("extractBaseRole(%d) = %d, want %d", tt.role, result, tt.expected)
			}
		})
	}
}

func TestHasAuditorRole(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected bool
	}{
		{"Admin only", RoleAdmin, false},
		{"Admin with Auditor", RoleAdmin | RoleAuditor, true},
		{"Auditor only", RoleAuditor, true},
		{"Owner only", RoleOwner, false},
		{"Owner with Auditor", RoleOwner | RoleAuditor, true},
		{"PowerUser only", RolePowerUser, false},
		{"Owner and Admin", RoleOwner | RoleAdmin, false},
		{"Owner and Admin with Auditor", RoleOwner | RoleAdmin | RoleAuditor, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role.HasAuditorRole()
			if result != tt.expected {
				t.Errorf("hasAuditorRole(%d) = %v, want %v", tt.role, result, tt.expected)
			}
		})
	}
}
