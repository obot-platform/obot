package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVMCPComponentToolPreviewConfigUsesCachedSnapshotAndFixedConfiguration(t *testing.T) {
	vmcp := vmcpToolPreviewTestObject("vmcp1snapshot")
	otherVMCP := vmcpToolPreviewTestObject("vmcp1snapshot-other")
	storage := clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(vmcp, otherVMCP).
		Build()
	gateway := newHandlerTestGateway(t)
	component := vmcp.Spec.Manifest.Components[0]
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{
			vmcpconfig.ConfigurationKey(component.ID, "TOKEN"):  "snapshot-token",
			vmcpconfig.ConfigurationKey(component.ID, "USER"):   "must-not-be-used",
			vmcpconfig.ConfigurationKey(component.ID, "DENIED"): "must-not-be-used",
			vmcpconfig.ConfigurationKey("component-b", "TOKEN"): "other-component",
		},
	}))
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(otherVMCP.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{
			vmcpconfig.ConfigurationKey(component.ID, "TOKEN"): "other-vmcp-token",
		},
	}))

	handler := NewMCPCatalogHandler("", "https://obot.example.com", "", nil, nil, gateway, nil, "")
	request := func(vmcpID, userID, componentID string) api.Context {
		req := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcpID+"/components/"+componentID+"/generate-tool-previews", nil)
		req.SetPathValue("vmcp_id", vmcpID)
		req.SetPathValue("component_id", componentID)
		return api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        req,
			Storage:        storage,
			GatewayClient:  gateway,
			User:           testUser(userID),
		}
	}

	gotVMCP, gotComponent, server, serverConfig, err := handler.vmcpComponentToolPreviewConfig(request(vmcp.Name, "user-one", component.ID))
	require.NoError(t, err)
	assert.Equal(t, vmcp.Name, gotVMCP.Name)
	assert.Equal(t, component.ID, gotComponent.ID)
	assert.Equal(t, types.RuntimeRemote, server.Spec.Manifest.Runtime)
	require.NotNil(t, server.Spec.Manifest.RemoteConfig)
	assert.Equal(t, "https://snapshot.example/mcp", server.Spec.Manifest.RemoteConfig.URL)
	assert.Equal(t, "https://snapshot.example/mcp", serverConfig.URL)
	assert.Equal(t, []string{"TOKEN=snapshot-token"}, serverConfig.Env)
	assert.Equal(t, "true", serverConfig.AuditLogMetadata[mcp.AuditLogIgnore])
	assert.NotContains(t, serverConfig.Env, "USER=must-not-be-used")
	assert.NotContains(t, serverConfig.Env, "DENIED=must-not-be-used")
	assert.Contains(t, server.Name, "vmcp-tool-preview-")
	assert.Equal(t, server.Name, serverConfig.MCPServerName)

	_, _, serverForOtherUser, _, err := handler.vmcpComponentToolPreviewConfig(request(vmcp.Name, "user-two", component.ID))
	require.NoError(t, err)
	assert.NotEqual(t, server.Name, serverForOtherUser.Name, "preview identities must not share user OAuth state")

	otherComponent := vmcp.Spec.Manifest.Components[1]
	_, _, serverForOtherComponent, _, err := handler.vmcpComponentToolPreviewConfig(request(vmcp.Name, "user-one", otherComponent.ID))
	require.NoError(t, err)
	assert.NotEqual(t, server.Name, serverForOtherComponent.Name, "preview identities must not share component OAuth state")

	_, _, serverForOtherVMCP, _, err := handler.vmcpComponentToolPreviewConfig(request(otherVMCP.Name, "user-one", component.ID))
	require.NoError(t, err)
	assert.NotEqual(t, server.Name, serverForOtherVMCP.Name, "preview identities must not share vMCP OAuth state")

	var stored v1.VMCP
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &stored))
	assert.Equal(t, vmcp.Spec.Manifest, stored.Spec.Manifest, "preview resolution must not mutate the vMCP snapshot")

	var source v1.MCPServerCatalogEntry
	err = storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: component.MCPServerCatalogEntryID}, &source)
	assert.True(t, apierrors.IsNotFound(err), "preview resolution must not look up the source catalog entry")
}

func TestGenerateVMCPComponentToolPreviewsOAuthURLUsesCachedSnapshot(t *testing.T) {
	vmcp := vmcpToolPreviewTestObject("vmcp1oauth-preview")
	storage := clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(vmcp).
		Build()
	gateway := newHandlerTestGateway(t)
	component := vmcp.Spec.Manifest.Components[0]
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{
			vmcpconfig.ConfigurationKey(component.ID, "TOKEN"): "oauth-snapshot-token",
		},
	}))
	checker := newRecordingMCPAuthChecker("https://oauth.example/authorize")
	handler := NewMCPCatalogHandler("", "https://obot.example.com", "", nil, checker, gateway, nil, "")
	req := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcp.Name+"/components/"+component.ID+"/generate-tool-previews/oauth-url", nil)
	req.SetPathValue("vmcp_id", vmcp.Name)
	req.SetPathValue("component_id", component.ID)
	recorder := httptest.NewRecorder()

	require.NoError(t, handler.GenerateVMCPComponentToolPreviewsOAuthURL(api.Context{
		ResponseWriter: recorder,
		Request:        req,
		Storage:        storage,
		GatewayClient:  gateway,
		User:           testUser("oauth-preview-user"),
	}))

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "https://snapshot.example/mcp", checker.config.URL)
	assert.Equal(t, []string{"TOKEN=oauth-snapshot-token"}, checker.config.Env)
	assert.Equal(t, "true", checker.config.AuditLogMetadata[mcp.AuditLogIgnore])
	assert.Equal(t, types.RuntimeRemote, checker.server.Spec.Manifest.Runtime)
	assert.Contains(t, checker.server.Name, "vmcp-tool-preview-")
	var response map[string]string
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "https://oauth.example/authorize", response["oauthURL"])
}

func TestTempServerAndConfigIgnoresAuditLogsWithoutChangingNormalTraffic(t *testing.T) {
	entryManifest := types.MCPServerCatalogEntryManifest{
		Name:    "preview-entry",
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL: "https://preview.example/mcp",
		},
	}

	server, previewConfig, err := tempServerAndConfig(
		t.Context(),
		nil,
		nil,
		"",
		"",
		"entry",
		"catalog",
		entryManifest,
		nil,
		"",
		"https://obot.example.com",
		mcp.ValidationOptions{},
	)
	require.NoError(t, err)
	assert.Equal(t, "true", previewConfig.AuditLogMetadata[mcp.AuditLogIgnore])

	normalConfig, missingFields, err := mcp.ServerToServerConfig(
		server,
		server.ValidConnectURLs("https://obot.example.com"),
		"temp",
		"temp",
		"catalog",
		nil,
	)
	require.NoError(t, err)
	assert.Empty(t, missingFields)
	assert.Equal(t, "false", normalConfig.AuditLogMetadata[mcp.AuditLogIgnore])
}

func TestVMCPComponentToolPreviewConfigMissingResourcesAreSafe(t *testing.T) {
	vmcp := vmcpToolPreviewTestObject("vmcp1missing-component")
	storage := clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(vmcp).
		Build()
	handler := NewMCPCatalogHandler("", "https://obot.example.com", "", nil, nil, newHandlerTestGateway(t), nil, "")

	t.Run("missing vmcp", func(t *testing.T) {
		missingStorage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
		req := httptest.NewRequest(http.MethodPost, "/api/vmcps/vmcp1missing/components/component-a/generate-tool-previews/oauth-url", nil)
		req.SetPathValue("vmcp_id", "vmcp1missing")
		req.SetPathValue("component_id", "component-a")
		err := handler.GenerateVMCPComponentToolPreviewsOAuthURL(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        req,
			Storage:        missingStorage,
			User:           testUser("user"),
			GatewayClient:  handler.gatewayClient,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get vMCP")
	})

	t.Run("missing component", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcp.Name+"/components/missing/generate-tool-previews/oauth-url", nil)
		req.SetPathValue("vmcp_id", vmcp.Name)
		req.SetPathValue("component_id", "missing")
		err := handler.GenerateVMCPComponentToolPreviewsOAuthURL(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        req,
			Storage:        storage,
			User:           testUser("user"),
			GatewayClient:  handler.gatewayClient,
		})
		require.Error(t, err)
		assert.True(t, types.IsNotFound(err))
		assert.Contains(t, err.Error(), "vMCP component not found")
	})
}

func vmcpToolPreviewTestObject(name string) *v1.VMCP {
	component := func(id, entryID, displayName, url string) types.VMCPComponent {
		return types.VMCPComponent{
			ID:                      id,
			Name:                    displayName,
			MCPCatalogID:            "catalog-snapshot",
			MCPServerCatalogEntryID: entryID,
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{
				Manifest: types.MCPServerCatalogEntryManifest{
					Name:    displayName,
					Runtime: types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{
						FixedURL: url,
					},
					Env: []types.MCPEnv{
						{MCPHeader: types.MCPHeader{Key: "TOKEN", Required: true}},
						{MCPHeader: types.MCPHeader{Key: "USER"}},
						{MCPHeader: types.MCPHeader{Key: "DENIED"}},
					},
				},
			},
			Configuration: []types.VMCPConfigurationPolicy{
				{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed},
				{Key: "USER", Policy: types.VMCPConfigurationPolicyUserAllowed},
				{Key: "DENIED", Policy: types.VMCPConfigurationPolicyProhibited},
			},
		}
	}

	return &v1.VMCP{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: system.DefaultNamespace},
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				DisplayName: "Snapshot preview vMCP",
				Components: []types.VMCPComponent{
					component("component-a", "entry-snapshot-a", "Snapshot A", "https://snapshot.example/mcp"),
					component("component-b", "entry-snapshot-b", "Snapshot B", "https://other-snapshot.example/mcp"),
				},
			},
		},
	}
}
