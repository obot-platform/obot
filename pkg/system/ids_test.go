package system

import (
	"testing"
)

func TestGetProjectShareName(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		projectID string
	}{
		{
			name:      "standard IDs",
			userID:    "user123",
			projectID: "t1project456",
		},
		{
			name:      "empty user ID",
			userID:    "",
			projectID: "t1project456",
		},
		{
			name:      "empty project ID",
			userID:    "user123",
			projectID: "",
		},
		{
			name:      "both empty",
			userID:    "",
			projectID: "",
		},
		{
			name:      "project ID without thread prefix",
			userID:    "user123",
			projectID: "project456",
		},
		{
			name:      "project ID with project prefix already",
			userID:    "user123",
			projectID: "p1project456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetProjectShareName(tt.userID, tt.projectID)

			// Verify it starts with the correct prefix
			if got == "" {
				t.Error("GetProjectShareName() returned empty string")
			}
			if len(got) > 0 && got[:3] != ThreadSharePrefix {
				t.Errorf("GetProjectShareName() = %q, should start with %q", got, ThreadSharePrefix)
			}

			// Test consistency - same inputs should produce same output
			got2 := GetProjectShareName(tt.userID, tt.projectID)
			if got != got2 {
				t.Errorf("GetProjectShareName() is not consistent: first=%q, second=%q", got, got2)
			}
		})
	}
}

func TestGetProjectShareName_Uniqueness(t *testing.T) {
	// Test that different inputs produce different outputs
	result1 := GetProjectShareName("user1", "t1project1")
	result2 := GetProjectShareName("user2", "t1project1")
	result3 := GetProjectShareName("user1", "t1project2")

	if result1 == result2 {
		t.Error("different users should produce different share names")
	}
	if result1 == result3 {
		t.Error("different projects should produce different share names")
	}
}

func TestGetPowerUserWorkspaceID(t *testing.T) {
	tests := []struct {
		name   string
		userID string
	}{
		{name: "standard user ID", userID: "user123"},
		{name: "empty user ID", userID: ""},
		{name: "user ID with special chars", userID: "user@example.com"},
		{name: "numeric user ID", userID: "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPowerUserWorkspaceID(tt.userID)

			// Verify it starts with the correct prefix
			if len(got) > 0 && got[:4] != PowerUserWorkspacePrefix {
				t.Errorf("GetPowerUserWorkspaceID() = %q, should start with %q", got, PowerUserWorkspacePrefix)
			}

			// Test consistency
			got2 := GetPowerUserWorkspaceID(tt.userID)
			if got != got2 {
				t.Errorf("GetPowerUserWorkspaceID() is not consistent: first=%q, second=%q", got, got2)
			}
		})
	}
}

func TestGetPowerUserWorkspaceID_Uniqueness(t *testing.T) {
	// Test that different user IDs produce different workspace IDs
	result1 := GetPowerUserWorkspaceID("user1")
	result2 := GetPowerUserWorkspaceID("user2")

	if result1 == result2 {
		t.Error("different users should produce different workspace IDs")
	}
}
