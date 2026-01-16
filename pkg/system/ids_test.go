package system

import (
	"testing"
)

func TestIsThreadID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid thread ID", id: "t1abc123", want: true},
		{name: "thread prefix only", id: "t1", want: true},
		{name: "different prefix", id: "a1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "t1 in middle", id: "abct1def", want: false},
		{name: "case sensitive - uppercase", id: "T1abc123", want: false},
		{name: "chat run ID", id: "r1chatabc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsThreadID(tt.id)
			if got != tt.want {
				t.Errorf("IsThreadID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsThreadTemplateID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid thread template ID", id: "tt1abc123", want: true},
		{name: "prefix only", id: "tt1", want: true},
		{name: "different prefix", id: "t1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "TT1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsThreadTemplateID(tt.id)
			if got != tt.want {
				t.Errorf("IsThreadTemplateID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsToolID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid tool ID", id: "tl1abc123", want: true},
		{name: "prefix only", id: "tl1", want: true},
		{name: "different prefix", id: "t1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "TL1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsToolID(tt.id)
			if got != tt.want {
				t.Errorf("IsToolID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsAgentID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid agent ID", id: "a1abc123", want: true},
		{name: "prefix only", id: "a1", want: true},
		{name: "different prefix", id: "t1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "A1abc123", want: false},
		{name: "a1 in middle", id: "xya1zz", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsAgentID(tt.id)
			if got != tt.want {
				t.Errorf("IsAgentID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsRunID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid run ID", id: "r1abc123", want: true},
		{name: "prefix only", id: "r1", want: true},
		{name: "chat run ID starts with r1", id: "r1chatabc123", want: true},
		{name: "different prefix", id: "t1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "R1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRunID(tt.id)
			if got != tt.want {
				t.Errorf("IsRunID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsWebhookID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid webhook ID", id: "wh1abc123", want: true},
		{name: "prefix only", id: "wh1", want: true},
		{name: "different prefix", id: "w1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "WH1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsWebhookID(tt.id)
			if got != tt.want {
				t.Errorf("IsWebhookID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsWorkflowID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid workflow ID", id: "w1abc123", want: true},
		{name: "prefix only", id: "w1", want: true},
		{name: "workflow execution prefix", id: "we1abc123", want: false},
		{name: "workflow step prefix", id: "ws1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "W1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsWorkflowID(tt.id)
			if got != tt.want {
				t.Errorf("IsWorkflowID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsEmailReceiverID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid email receiver ID", id: "er1abc123", want: true},
		{name: "prefix only", id: "er1", want: true},
		{name: "different prefix", id: "e1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "ER1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsEmailReceiverID(tt.id)
			if got != tt.want {
				t.Errorf("IsEmailReceiverID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsChatRunID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid chat run ID", id: "r1chatabc123", want: true},
		{name: "prefix only", id: "r1chat", want: true},
		{name: "regular run ID", id: "r1abc123", want: false},
		{name: "different prefix", id: "t1chatabc", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "R1CHATabc123", want: false},
		{name: "chat in middle", id: "abc-r1chat-def", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsChatRunID(tt.id)
			if got != tt.want {
				t.Errorf("IsChatRunID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsMCPServerID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid MCP server ID", id: "ms1abc123", want: true},
		{name: "prefix only", id: "ms1", want: true},
		{name: "MCP server instance ID", id: "msi1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "MS1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMCPServerID(tt.id)
			if got != tt.want {
				t.Errorf("IsMCPServerID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsMCPServerInstanceID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid MCP server instance ID", id: "msi1abc123", want: true},
		{name: "prefix only", id: "msi1", want: true},
		{name: "MCP server ID", id: "ms1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "MSI1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMCPServerInstanceID(tt.id)
			if got != tt.want {
				t.Errorf("IsMCPServerInstanceID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsPowerUserWorkspaceID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid power user workspace ID", id: "puw1abc123", want: true},
		{name: "prefix only", id: "puw1", want: true},
		{name: "different prefix", id: "pw1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "PUW1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPowerUserWorkspaceID(tt.id)
			if got != tt.want {
				t.Errorf("IsPowerUserWorkspaceID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsSystemMCPServerID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid system MCP server ID", id: "sms1abc123", want: true},
		{name: "prefix only", id: "sms1", want: true},
		{name: "regular MCP server ID", id: "ms1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "SMS1abc123", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSystemMCPServerID(tt.id)
			if got != tt.want {
				t.Errorf("IsSystemMCPServerID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestIsModelID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "valid model ID", id: "m1abc123", want: true},
		{name: "prefix only", id: "m1", want: true},
		{name: "different prefix", id: "t1abc123", want: false},
		{name: "empty string", id: "", want: false},
		{name: "case sensitive", id: "M1abc123", want: false},
		{name: "m1 in middle", id: "xym1zz", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsModelID(tt.id)
			if got != tt.want {
				t.Errorf("IsModelID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

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
