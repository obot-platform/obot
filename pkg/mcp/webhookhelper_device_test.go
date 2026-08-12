package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestDeviceFilterSurfaceIndexKeys(t *testing.T) {
	base := &v1.MCPWebhookValidation{
		Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
			Resources: []types.Resource{{
				Type: types.ResourceTypeDevice,
				ID:   "*",
			}},
			DeviceSurfaces: []types.FilterSurface{
				types.FilterSurfaceUserPrompt,
				types.FilterSurfaceToolResponse,
			},
		}},
		Status: v1.MCPWebhookValidationStatus{Configured: true},
	}

	tests := []struct {
		name   string
		mutate func(*v1.MCPWebhookValidation)
		want   []string
	}{
		{
			name: "enabled configured device filter",
			want: []string{"user_prompt", "tool_response"},
		},
		{
			name: "disabled",
			mutate: func(v *v1.MCPWebhookValidation) {
				v.Spec.Manifest.Disabled = true
			},
		},
		{
			name: "unconfigured",
			mutate: func(v *v1.MCPWebhookValidation) {
				v.Status.Configured = false
			},
		},
		{
			name: "MCP only",
			mutate: func(v *v1.MCPWebhookValidation) {
				v.Spec.Manifest.Resources = nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := base.DeepCopy()
			if tt.mutate != nil {
				tt.mutate(filter)
			}
			got, err := DeviceFilterSurfaceIndexKeys(filter)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("keys = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("keys = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
