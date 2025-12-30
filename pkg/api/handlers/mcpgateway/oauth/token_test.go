package oauth

import (
	"testing"

	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func TestValidateAPIKeyAccess(t *testing.T) {
	// MCP server IDs use prefix "ms1", MCP server instance IDs use prefix "msi1"
	tests := []struct {
		name    string
		apiKey  *gwtypes.APIKey
		mcpID   string
		wantErr bool
	}{
		{
			name: "empty scopes denies MCP server",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs:         nil,
				MCPServerInstanceIDs: nil,
			},
			mcpID:   "ms1abc123",
			wantErr: true,
		},
		{
			name: "empty slices denies MCP server",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs:         []string{},
				MCPServerInstanceIDs: []string{},
			},
			mcpID:   "ms1abc123",
			wantErr: true,
		},
		{
			name: "empty scopes denies MCP server instance",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs:         nil,
				MCPServerInstanceIDs: nil,
			},
			mcpID:   "msi1abc123",
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
			name: "key allows matching instance",
			apiKey: &gwtypes.APIKey{
				MCPServerInstanceIDs: []string{"msi1xyz789"},
			},
			mcpID:   "msi1xyz789",
			wantErr: false,
		},
		{
			name: "key denies non-matching instance",
			apiKey: &gwtypes.APIKey{
				MCPServerInstanceIDs: []string{"msi1xyz789"},
			},
			mcpID:   "msi1other123",
			wantErr: true,
		},
		{
			name: "key with server restriction denies MCP server instance",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs: []string{"ms1abc123"},
			},
			mcpID:   "msi1xyz789",
			wantErr: true,
		},
		{
			name: "key with instance restriction denies MCP server",
			apiKey: &gwtypes.APIKey{
				MCPServerInstanceIDs: []string{"msi1xyz789"},
			},
			mcpID:   "ms1abc123",
			wantErr: true,
		},
		{
			name: "key with both restrictions allows matching server",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs:         []string{"ms1abc123"},
				MCPServerInstanceIDs: []string{"msi1xyz789"},
			},
			mcpID:   "ms1abc123",
			wantErr: false,
		},
		{
			name: "key with both restrictions allows matching instance",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs:         []string{"ms1abc123"},
				MCPServerInstanceIDs: []string{"msi1xyz789"},
			},
			mcpID:   "msi1xyz789",
			wantErr: false,
		},
		{
			name: "key with both restrictions denies non-matching ID",
			apiKey: &gwtypes.APIKey{
				MCPServerIDs:         []string{"ms1abc123"},
				MCPServerInstanceIDs: []string{"msi1xyz789"},
			},
			mcpID:   "ms1other456",
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
