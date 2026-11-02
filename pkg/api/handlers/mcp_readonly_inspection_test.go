package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestAdminReadonlyInspectionDoesNotDeployStoppedServers(t *testing.T) {
	require.NoError(t, appsv1.AddToScheme(storagescheme.Scheme))
	for _, role := range []types.Role{types.RoleAdminReadonly, types.RoleAdminReadonly | types.RoleAuditor} {
		t.Run(strconv.Itoa(int(role)), func(t *testing.T) {
			const webhookID = "mwhv1inspection"
			server := &v1.MCPServer{
				Name:      system.MCPServerPrefix + "inspection",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					UserID:       "another-user",
					MCPCatalogID: system.DefaultCatalog,
					Manifest: types.MCPServerManifest{
						Runtime:             types.RuntimeContainerized,
						ContainerizedConfig: &types.ContainerizedRuntimeConfig{Image: "example.com/mcp:test", Port: 8080},
					},
				},
			}
			systemServer := &v1.SystemMCPServer{
				Name:      system.SystemMCPServerPrefix + webhookID,
				Namespace: system.DefaultNamespace,
				UID:       "inspection-uid",
				Spec: v1.SystemMCPServerSpec{
					Manifest: types.SystemMCPServerManifest{
						Runtime:             types.RuntimeContainerized,
						ContainerizedConfig: &types.ContainerizedRuntimeConfig{Image: "example.com/validator:test", Port: 8080},
					},
				},
			}
			webhook := &v1.MCPWebhookValidation{
				Name:      webhookID,
				Namespace: system.DefaultNamespace,
			}
			manager, storage, gateway := newVMCPActionSessionManager(t, server, systemServer, webhook)
			mcpHandler := &MCPHandler{mcpSessionManager: manager}
			systemHandler := &SystemMCPServerHandler{mcpSessionManager: manager}
			webhookHandler := &MCPWebhookValidationHandler{mcpSessionManager: manager}
			for _, endpoint := range []struct {
				name    string
				handler func(api.Context) error
			}{
				{
					name:    "server details",
					handler: mcpHandler.GetServerDetails,
				},
				{
					name:    "server logs",
					handler: mcpHandler.StreamServerLogs,
				},
				{
					name:    "system server details",
					handler: systemHandler.GetDetails,
				},
				{
					name:    "system server logs",
					handler: systemHandler.Logs,
				},
				{
					name:    "webhook validator details",
					handler: webhookHandler.GetDetails,
				},
				{
					name:    "webhook validator logs",
					handler: webhookHandler.Logs,
				},
			} {
				t.Run(endpoint.name, func(t *testing.T) {
					request := httptest.NewRequest(http.MethodGet, "/", nil)
					request.SetPathValue("mcp_server_id", server.Name)
					request.SetPathValue("id", systemServer.Name)
					request.SetPathValue("mcp_webhook_validation_id", webhookID)
					err := endpoint.handler(api.Context{
						Request:        request,
						ResponseWriter: httptest.NewRecorder(),
						Storage:        storage,
						GatewayClient:  gateway,
						User: &kuser.DefaultInfo{
							UID:    "readonly-user",
							Groups: role.Groups(),
						},
					})
					httpErr, ok := errors.AsType[*types.ErrHTTP](err)
					require.True(t, ok, "expected HTTP stopped-server error, got %v", err)
					require.Equal(t, http.StatusConflict, httpErr.Code)
					require.Equal(t, "MCP server is not running", httpErr.Message)

					var deployments appsv1.DeploymentList
					require.NoError(t, storage.List(t.Context(), &deployments))
					require.Empty(t, deployments.Items)
				})
			}
		})
	}
}
