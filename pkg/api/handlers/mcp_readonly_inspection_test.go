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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestAdminReadonlyInspectionDoesNotDeployStoppedServers(t *testing.T) {
	require.NoError(t, appsv1.AddToScheme(storagescheme.Scheme))
	for _, role := range []types.Role{types.RoleAdminReadonly, types.RoleAdminReadonly | types.RoleAuditor, types.RoleBasic | types.RoleAuditor} {
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
						// A required value with no credential stored for it. Checking whether the server is configured
						// reveals its credentials and would fail the request, so read-only inspection must skip it.
						Config: []types.MCPConfig{{
							Key:       "VALIDATOR_TOKEN",
							Required:  true,
							Sensitive: true,
						}},
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

// TestAdminReadonlyInspectionDoesNotConnectToVMCPs checks that a vMCP ID given to the server details and logs routes
// does not create the caller's connection to that vMCP. It also shows that read-only inspection does not resolve the
// server as an action, which is what would refresh another user's server's last request time.
func TestAdminReadonlyInspectionDoesNotConnectToVMCPs(t *testing.T) {
	const vmcpID = "vmcp1inspection"
	vmcp := &v1.VMCP{
		ObjectMeta: objectMetaForVMCPActionTest(vmcpID),
		Spec: v1.VMCPSpec{
			UserID: "another-user",
			Manifest: types.VMCPManifest{
				DisplayName: "Another user's vMCP",
				Profiles: []types.VMCPProfile{{
					Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				}},
			},
		},
	}
	manager, storage, gateway := newVMCPActionSessionManager(t, vmcp)
	handler := &MCPHandler{mcpSessionManager: manager}

	for _, role := range []types.Role{types.RoleAdminReadonly, types.RoleBasic | types.RoleAuditor} {
		for _, endpoint := range []struct {
			name    string
			handler func(api.Context) error
		}{
			{
				name:    "details",
				handler: handler.GetServerDetails,
			},
			{
				name:    "logs",
				handler: handler.StreamServerLogs,
			},
		} {
			t.Run(strconv.Itoa(int(role))+"/"+endpoint.name, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, "/", nil)
				request.SetPathValue("mcp_server_id", vmcpID)
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
				require.True(t, apierrors.IsNotFound(err), "expected not found, got %v", err)

				var instances v1.VMCPInstanceList
				require.NoError(t, storage.List(t.Context(), &instances))
				require.Empty(t, instances.Items)
			})
		}
	}
}

// TestAdminReadonlyInspectionResolvesConnections checks that the server details and logs routes accept a connection
// to a multi-user server and inspect it read-only through the server it connects to. The caller's own connection to a
// shared server they don't own does not let them start it.
func TestAdminReadonlyInspectionResolvesConnections(t *testing.T) {
	require.NoError(t, appsv1.AddToScheme(storagescheme.Scheme))
	const (
		readerID       = "readonly-user"
		otherInstance  = system.MCPServerInstancePrefix + "other"
		readerInstance = system.MCPServerInstancePrefix + "reader"
	)
	server := &v1.MCPServer{
		Name:      system.MCPServerPrefix + "shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			MCPCatalogID: system.DefaultCatalog,
			Manifest: types.MCPServerManifest{
				Runtime:             types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{Image: "example.com/mcp:test", Port: 8080},
			},
		},
	}
	instances := []*v1.MCPServerInstance{
		{
			ObjectMeta: metav1.ObjectMeta{Name: otherInstance, Namespace: system.DefaultNamespace},
			Spec: v1.MCPServerInstanceSpec{
				UserID:        "another-user",
				MCPServerName: server.Name,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: readerInstance, Namespace: system.DefaultNamespace},
			Spec: v1.MCPServerInstanceSpec{
				UserID:        readerID,
				MCPServerName: server.Name,
			},
		},
	}
	manager, storage, gateway := newVMCPActionSessionManager(t, server, instances[0], instances[1])
	handler := &MCPHandler{mcpSessionManager: manager}

	for _, role := range []types.Role{types.RoleAdminReadonly, types.RoleBasic | types.RoleAuditor} {
		for _, endpoint := range []struct {
			name    string
			handler func(api.Context) error
		}{
			{
				name:    "details",
				handler: handler.GetServerDetails,
			},
			{
				name:    "logs",
				handler: handler.StreamServerLogs,
			},
		} {
			call := func(id string) error {
				request := httptest.NewRequest(http.MethodGet, "/", nil)
				request.SetPathValue("mcp_server_id", id)
				return endpoint.handler(api.Context{
					Request:        request,
					ResponseWriter: httptest.NewRecorder(),
					Storage:        storage,
					GatewayClient:  gateway,
					User: &kuser.DefaultInfo{
						UID:    readerID,
						Groups: role.Groups(),
					},
				})
			}

			for _, connection := range []struct {
				name string
				id   string
			}{
				{
					name: "another user's connection",
					id:   otherInstance,
				},
				{
					name: "own connection",
					id:   readerInstance,
				},
			} {
				t.Run(strconv.Itoa(int(role))+"/"+endpoint.name+"/"+connection.name, func(t *testing.T) {
					err := call(connection.id)
					httpErr, ok := errors.AsType[*types.ErrHTTP](err)
					require.True(t, ok, "expected HTTP stopped-server error, got %v", err)
					require.Equal(t, http.StatusConflict, httpErr.Code)

					var deployments appsv1.DeploymentList
					require.NoError(t, storage.List(t.Context(), &deployments))
					require.Empty(t, deployments.Items)
				})
			}
		}
	}
}
