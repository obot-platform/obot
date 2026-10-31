package types

import (
	"slices"
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

func TestSwitchBaseRole(t *testing.T) {
	tests := []struct {
		name        string
		currentRole Role
		newBaseRole Role
		expected    Role
	}{
		{"Switch base role preserves Auditor", RoleAdmin | RoleAuditor, RolePowerUser, RolePowerUser | RoleAuditor},
		{"Switch base role with no add-ons", RoleOwner, RoleAdmin, RoleAdmin},
		{"Switch from basic to admin no add-ons", RoleBasic, RoleAdmin, RoleAdmin},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.currentRole.SwitchBaseRole(tt.newBaseRole)
			if result != tt.expected {
				t.Errorf("SwitchBaseRole(%d, %d) = %d, want %d", tt.currentRole, tt.newBaseRole, result, tt.expected)
			}
		})
	}
}

func TestGroups(t *testing.T) {
	tests := []struct {
		name           string
		role           Role
		expectContains []string
		expectMissing  []string
	}{
		{
			"Owner gets owner, admin, power user groups",
			RoleOwner,
			[]string{GroupOwner, GroupAdmin, GroupPowerUserPlus, GroupPowerUser, GroupBasic, GroupAuthenticated, GroupAPI, GroupLLM, GroupSkills, GroupMCP, GroupDeviceScans},
			[]string{GroupAuditor},
		},
		{
			"Admin with Auditor",
			RoleAdmin | RoleAuditor,
			[]string{GroupAdmin, GroupAuditor, GroupAuthenticated, GroupAPI, GroupLLM, GroupSkills, GroupMCP, GroupDeviceScans},
			[]string{GroupOwner},
		},
		{
			"Owner with all add-ons",
			RoleOwner | RoleAuditor,
			[]string{GroupOwner, GroupAdmin, GroupAuditor, GroupAuthenticated, GroupAPI, GroupLLM, GroupSkills, GroupMCP, GroupDeviceScans},
			nil,
		},
		{
			name:           "Admin gets admin read-only group",
			role:           RoleAdmin,
			expectContains: []string{GroupAdmin, GroupAdminReadonly, GroupBasic},
			expectMissing:  []string{GroupOwner, GroupAuditor},
		},
		{
			name:           "Admin read-only gets only basic privileges besides its own group",
			role:           RoleAdminReadonly,
			expectContains: []string{GroupAdminReadonly, GroupBasic, GroupAuthenticated, GroupAPI},
			expectMissing:  []string{GroupOwner, GroupAdmin, GroupAuditor, GroupPowerUserPlus, GroupPowerUser},
		},
		{
			name:           "Admin read-only with Auditor",
			role:           RoleAdminReadonly | RoleAuditor,
			expectContains: []string{GroupAdminReadonly, GroupAuditor, GroupBasic},
			expectMissing:  []string{GroupOwner, GroupAdmin, GroupPowerUserPlus, GroupPowerUser},
		},
		{
			name:           "Unnormalized source bitmap exposes both groups",
			role:           RoleAdminReadonly | RolePowerUserPlus,
			expectContains: []string{GroupAdminReadonly, GroupPowerUserPlus, GroupPowerUser, GroupBasic},
			expectMissing:  []string{GroupOwner, GroupAdmin, GroupAuditor},
		},
		{
			name:           "Basic with Auditor does not get admin read-only group",
			role:           RoleBasic | RoleAuditor,
			expectContains: []string{GroupAuditor, GroupBasic},
			expectMissing:  []string{GroupAdminReadonly, GroupAdmin},
		},
		{
			"Unknown role gets no groups",
			RoleUnknown,
			nil,
			[]string{GroupOwner, GroupAdmin, GroupAuditor, GroupAuthenticated, GroupAPI, GroupLLM, GroupSkills, GroupMCP, GroupDeviceScans},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := tt.role.Groups()
			for _, expected := range tt.expectContains {
				if !slices.Contains(groups, expected) {
					t.Errorf("Groups(%d) missing expected group %q, got %v", tt.role, expected, groups)
				}
			}
			for _, unexpected := range tt.expectMissing {
				if slices.Contains(groups, unexpected) {
					t.Errorf("Groups(%d) unexpectedly contains group %q, got %v", tt.role, unexpected, groups)
				}
			}
		})
	}
}

func TestRoleValues(t *testing.T) {
	// Verify role bit values are correct powers of 2 and don't overlap
	roles := []struct {
		name  string
		role  Role
		value int
	}{
		{"RoleBasic", RoleBasic, 4},
		{"RoleOwner", RoleOwner, 8},
		{"RoleAdmin", RoleAdmin, 16},
		{"RoleAuditor", RoleAuditor, 32},
		{"RolePowerUserPlus", RolePowerUserPlus, 64},
		{"RolePowerUser", RolePowerUser, 128},
		{
			name:  "RoleAdminReadonly",
			role:  RoleAdminReadonly,
			value: 256,
		},
	}

	for _, r := range roles {
		t.Run(r.name, func(t *testing.T) {
			if int(r.role) != r.value {
				t.Errorf("%s = %d, want %d", r.name, r.role, r.value)
			}
		})
	}

	// Verify no two roles share the same bit
	seen := make(map[Role]string)
	for _, r := range roles {
		if existing, ok := seen[r.role]; ok {
			t.Errorf("%s and %s have the same value %d", r.name, existing, r.role)
		}
		seen[r.role] = r.name
	}
}

func TestHasRole(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		check    Role
		expected bool
	}{
		{"Owner has Admin", RoleOwner, RoleAdmin, true},
		{"Owner has Basic", RoleOwner, RoleBasic, true},
		{"Admin does not have Owner", RoleAdmin, RoleOwner, false},
		{
			name:     "Admin read-only has Basic",
			role:     RoleAdminReadonly,
			check:    RoleBasic,
			expected: true,
		},
		{
			name:     "Admin read-only does not have Admin",
			role:     RoleAdminReadonly,
			check:    RoleAdmin,
			expected: false,
		},
		{
			name:     "Admin read-only does not have PowerUser",
			role:     RoleAdminReadonly,
			check:    RolePowerUser,
			expected: false,
		},
		{
			name:     "Admin does not have Admin read-only",
			role:     RoleAdmin,
			check:    RoleAdminReadonly,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role.HasRole(tt.check)
			if result != tt.expected {
				t.Errorf("Role(%d).HasRole(%d) = %v, want %v", tt.role, tt.check, result, tt.expected)
			}
		})
	}
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		name string
		role Role
		want bool
	}{
		{
			name: "basic",
			role: RoleBasic,
			want: true,
		},
		{
			name: "admin read-only",
			role: RoleAdminReadonly,
			want: true,
		},
		{
			name: "admin read-only with auditor",
			role: RoleAdminReadonly | RoleAuditor,
			want: true,
		},
		{
			name: "owner with auditor",
			role: RoleOwner | RoleAuditor,
			want: true,
		},
		{
			name: "admin read-only with power user plus",
			role: RoleAdminReadonly | RolePowerUserPlus,
			want: false,
		},
		{
			name: "admin read-only with power user and auditor",
			role: RoleAdminReadonly | RolePowerUser | RoleAuditor,
			want: false,
		},
		{
			name: "admin read-only with both power user roles",
			role: RoleAdminReadonly | RolePowerUserPlus | RolePowerUser,
			want: false,
		},
		{
			name: "admin with power user plus",
			role: RoleAdmin | RolePowerUserPlus,
			want: false,
		},
		{
			name: "auditor only",
			role: RoleAuditor,
			want: false,
		},
		{
			name: "two base roles",
			role: RoleAdmin | RoleAdminReadonly,
			want: false,
		},
		{
			name: "unknown bit",
			role: 1 << 20,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.IsValid(); got != tt.want {
				t.Errorf("Role(%d).IsValid() = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}
