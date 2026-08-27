package controller

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileObotMCPServerUsesAgentsFeatureState(t *testing.T) {
	tests := []struct {
		name          string
		agentsEnabled bool
		existing      *v1.SystemMCPServer
		wantEnabled   bool
	}{
		{
			name:          "creates disabled server when agents are disabled",
			agentsEnabled: false,
			wantEnabled:   false,
		},
		{
			name:          "creates enabled server when agents are enabled",
			agentsEnabled: true,
			wantEnabled:   true,
		},
		{
			name:          "disables existing server when agents are disabled",
			agentsEnabled: false,
			existing: &v1.SystemMCPServer{
				Name:      system.ObotMCPServerName,
				Namespace: system.DefaultNamespace,
			},
			wantEnabled: false,
		},
		{
			name:          "enables existing server when agents are enabled",
			agentsEnabled: true,
			existing: &v1.SystemMCPServer{
				Name:      system.ObotMCPServerName,
				Namespace: system.DefaultNamespace,
				Spec: v1.SystemMCPServerSpec{
					Manifest: disabledSystemMCPServerManifest(),
				},
			},
			wantEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(storagescheme.Scheme)
			if tt.existing != nil {
				builder = builder.WithObjects(tt.existing)
			}
			client := builder.Build()

			if err := reconcileObotMCPServer(t.Context(), client, tt.agentsEnabled, "http://obot", "obot-mcp-image"); err != nil {
				t.Fatalf("reconcileObotMCPServer() error = %v", err)
			}

			var server v1.SystemMCPServer
			if err := client.Get(t.Context(), kclient.ObjectKey{
				Name:      system.ObotMCPServerName,
				Namespace: system.DefaultNamespace,
			}, &server); err != nil {
				t.Fatalf("failed to get obot MCP server: %v", err)
			}

			gotEnabled := server.Spec.Manifest.Enabled == nil || *server.Spec.Manifest.Enabled
			if gotEnabled != tt.wantEnabled {
				t.Fatalf("server enabled = %v, want %v", gotEnabled, tt.wantEnabled)
			}
		})
	}
}

func disabledSystemMCPServerManifest() types.SystemMCPServerManifest {
	return types.SystemMCPServerManifest{Enabled: new(false)}
}
