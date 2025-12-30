package oauth

import (
	"testing"

	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func TestValidateAPIKeyAccess(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  *gwtypes.APIKey
		mcpID   string
		wantErr bool
	}{
		{
			name: "empty scopes denies MCP server",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs: nil,
			},
			mcpID:   "ms1abc123",
			wantErr: true,
		},
		{
			name: "empty slice denies MCP server",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs: []string{},
			},
			mcpID:   "ms1abc123",
			wantErr: true,
		},
		{
			name: "key allows matching MCP server",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs: []string{"ms1abc123", "ms1def456"},
			},
			mcpID:   "ms1abc123",
			wantErr: false,
		},
		{
			name: "key denies non-matching MCP server",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs: []string{"ms1def456"},
			},
			mcpID:   "ms1abc123",
			wantErr: true,
		},
		{
			name: "key allows second server in list",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs: []string{"ms1abc123", "ms1def456"},
			},
			mcpID:   "ms1def456",
			wantErr: false,
		},
		{
			name: "key with multiple servers denies unlisted server",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs: []string{"ms1abc123", "ms1def456"},
			},
			mcpID:   "ms1other789",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAPIKeyAccess(tt.apiKey, tt.mcpID)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAPIKeyAccess() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
