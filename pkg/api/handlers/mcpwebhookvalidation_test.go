package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestMCPWebhookValidationManifest_Validate(t *testing.T) {
	tests := []struct {
		name     string
		manifest types.MCPWebhookValidationManifest
		wantErr  bool
	}{
		{
			name: "url only",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
			},
		},
		{
			name: "system server manifest only",
			manifest: types.MCPWebhookValidationManifest{
				ToolName: "validate-webhook",
				SystemMCPServerManifest: &types.SystemMCPServerManifest{
					Name:    "validator",
					Enabled: new(true),
					Runtime: types.RuntimeContainerized,
					ContainerizedConfig: &types.ContainerizedRuntimeConfig{
						Image: "example/image:latest",
						Port:  8080,
						Path:  "/mcp",
					},
				},
			},
		},
		{
			name:     "missing both url and system server manifest",
			manifest: types.MCPWebhookValidationManifest{},
			wantErr:  true,
		},
		{
			name: "url and system server manifest are mutually exclusive",
			manifest: types.MCPWebhookValidationManifest{
				URL: "https://example.com/webhook",
				SystemMCPServerManifest: &types.SystemMCPServerManifest{
					Name: "validator",
				},
			},
			wantErr: true,
		},
		{
			name: "validation allows embedded manifest shape checks to happen later",
			manifest: types.MCPWebhookValidationManifest{
				SystemMCPServerManifest: &types.SystemMCPServerManifest{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManifest(tt.manifest)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
