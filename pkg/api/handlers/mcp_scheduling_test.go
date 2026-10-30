package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRedeployWithK8sSettingsWithoutConnection(t *testing.T) {
	for _, tc := range []struct {
		name       string
		vmcpID     string
		instanceID string
	}{
		{
			name:   "shared component",
			vmcpID: "vmcp1shared",
		},
		{
			name:       "personal component",
			instanceID: "vmcpi1personal",
		},
		{
			name: "ordinary server",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &v1.MCPServer{
				Name:      "ms1scheduling",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					UserID:         "owner",
					MCPCatalogID:   system.DefaultCatalog,
					VMCPID:         tc.vmcpID,
					VMCPInstanceID: tc.instanceID,
					Manifest: types.MCPServerManifest{
						Runtime:   types.RuntimeNPX,
						NPXConfig: &types.NPXRuntimeConfig{Package: "test-package"},
						Config:    []types.MCPConfig{{Key: "TOKEN", Required: true}},
					},
				},
			}
			settings := &v1.K8sSettings{
				Name:      system.K8sSettingsName,
				Namespace: system.DefaultNamespace,
			}
			manager, storage, gateway := newVMCPActionSessionManager(t, server, settings)
			handler := &MCPHandler{
				mcpSessionManager: manager,
				mcpRuntimeBackend: mcp.RuntimeBackendKubernetes,
			}
			require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: server.CredentialContext(server.Spec.UserID),
				Name:    server.Name,
				Secrets: map[string]string{"TOKEN": "deployment-token"},
			}))

			config, err := manager.ServerConfigForDeployment(t.Context(), *server)
			require.NoError(t, err)
			require.Contains(t, config.Env, "TOKEN=deployment-token")
			require.Empty(t, config.MCPServerInstanceID)

			// An up-to-date deployment exercises the endpoint without a live Kubernetes API.
			server.Status.K8sSettingsHash, err = handler.currentK8sSettingsHashWithImagePullSecrets(settings.Spec, *server, nil)
			require.NoError(t, err)
			require.NoError(t, storage.Update(t.Context(), server))

			request := httptest.NewRequest(http.MethodPost, "/", nil)
			request.SetPathValue("mcp_server_id", server.Name)
			request.SetPathValue("catalog_id", system.DefaultCatalog)
			ctx := api.Context{
				Request:        request,
				ResponseWriter: httptest.NewRecorder(),
				Storage:        storage,
				GatewayClient:  gateway,
				User: &kuser.DefaultInfo{
					UID:   "unconnected-admin",
					Extra: map[string][]string{"obot_groups": types.RoleAdmin.Groups()},
				},
			}
			require.NoError(t, handler.RedeployWithK8sSettings(ctx))

			request.SetPathValue("catalog_id", "other-catalog")
			require.ErrorContains(t, handler.RedeployWithK8sSettings(ctx), "MCP server not found")

			var connections v1.MCPServerInstanceList
			require.NoError(t, storage.List(t.Context(), &connections, kclient.InNamespace(system.DefaultNamespace)))
			require.Empty(t, connections.Items)

			server.Spec.Manifest.Config = append(server.Spec.Manifest.Config, types.MCPConfig{Key: "MISSING", Required: true})
			_, err = manager.ServerConfigForDeployment(t.Context(), *server)
			require.ErrorContains(t, err, "MISSING")
		})
	}
}
