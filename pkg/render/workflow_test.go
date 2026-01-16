package render

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestIsExternalTool(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		expected bool
	}{
		{
			name:     "external tool with dot",
			tool:     "github.com/some-tool",
			expected: true,
		},
		{
			name:     "external tool with forward slash",
			tool:     "some/tool",
			expected: true,
		},
		{
			name:     "external tool with backslash",
			tool:     "some\\tool",
			expected: true,
		},
		{
			name:     "local tool name",
			tool:     "my-tool",
			expected: false,
		},
		{
			name:     "local tool with dashes",
			tool:     "my-custom-tool",
			expected: false,
		},
		{
			name:     "empty string",
			tool:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsExternalTool(tt.tool)
			if result != tt.expected {
				t.Errorf("IsExternalTool(%q) = %v, want %v", tt.tool, result, tt.expected)
			}
		})
	}
}

func TestIsValidEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid env variable",
			env:     "MY_VAR",
			wantErr: false,
		},
		{
			name:    "valid env with lowercase",
			env:     "my_var",
			wantErr: false,
		},
		{
			name:    "valid env with numbers",
			env:     "MY_VAR_123",
			wantErr: false,
		},
		{
			name:    "valid env starting with underscore",
			env:     "_MY_VAR",
			wantErr: false,
		},
		{
			name:    "invalid env starting with number",
			env:     "1MY_VAR",
			wantErr: true,
			errMsg:  "must match",
		},
		{
			name:    "invalid env with hyphen",
			env:     "MY-VAR",
			wantErr: true,
			errMsg:  "must match",
		},
		{
			name:    "invalid env with special characters",
			env:     "MY$VAR",
			wantErr: true,
			errMsg:  "must match",
		},
		{
			name:    "invalid env starting with OBOT",
			env:     "OBOT_SOMETHING",
			wantErr: true,
			errMsg:  "cannot start with OBOT, GPTSCRIPT or KNOW",
		},
		{
			name:    "invalid env starting with GPTSCRIPT",
			env:     "GPTSCRIPT_VAR",
			wantErr: true,
			errMsg:  "cannot start with OBOT, GPTSCRIPT or KNOW",
		},
		{
			name:    "invalid env starting with KNOW",
			env:     "KNOW_DATA",
			wantErr: true,
			errMsg:  "cannot start with OBOT, GPTSCRIPT or KNOW",
		},
		{
			name:    "empty string",
			env:     "",
			wantErr: true,
			errMsg:  "must match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidEnv(tt.env)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsValidEnv(%q) error = %v, wantErr %v", tt.env, err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("IsValidEnv(%q) error = %v, want error containing %q", tt.env, err, tt.errMsg)
				}
			}
		})
	}
}

func TestIsValidImage(t *testing.T) {
	tests := []struct {
		name    string
		image   string
		wantErr bool
	}{
		{
			name:    "simple image name",
			image:   "ubuntu",
			wantErr: false,
		},
		{
			name:    "image with tag",
			image:   "ubuntu:20.04",
			wantErr: false,
		},
		{
			name:    "image with registry",
			image:   "docker.io/library/ubuntu",
			wantErr: false,
		},
		{
			name:    "image with registry and tag",
			image:   "docker.io/library/ubuntu:20.04",
			wantErr: false,
		},
		{
			name:    "image with port",
			image:   "localhost:5000/myimage",
			wantErr: false,
		},
		{
			name:    "image with digest",
			image:   "ubuntu@sha256:abc123",
			wantErr: true,
		},
		{
			name:    "image with spaces",
			image:   "ubuntu 20.04",
			wantErr: true,
		},
		{
			name:    "empty string",
			image:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidImage(tt.image)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsValidImage(%q) error = %v, wantErr %v", tt.image, err, tt.wantErr)
			}
		})
	}
}

func TestIsValidToolType(t *testing.T) {
	tests := []struct {
		name     string
		toolType types.ToolType
		wantErr  bool
	}{
		{
			name:     "container type",
			toolType: "container",
			wantErr:  false,
		},
		{
			name:     "script type",
			toolType: "script",
			wantErr:  false,
		},
		{
			name:     "javascript type",
			toolType: "javascript",
			wantErr:  false,
		},
		{
			name:     "python type",
			toolType: "python",
			wantErr:  false,
		},
		{
			name:     "invalid type",
			toolType: "invalid",
			wantErr:  true,
		},
		{
			name:     "docker type (not in allowed list)",
			toolType: "docker",
			wantErr:  true,
		},
		{
			name:     "empty string",
			toolType: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidToolType(tt.toolType)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsValidToolType(%q) error = %v, wantErr %v", tt.toolType, err, tt.wantErr)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && indexContains(s, substr)))
}

func indexContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
