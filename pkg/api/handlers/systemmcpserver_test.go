package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestConfigureSystemMCPServerPersistsRenderedURL(t *testing.T) {
	server := v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "system-server", Namespace: system.DefaultNamespace},
		Spec: v1.SystemMCPServerSpec{Manifest: types.SystemMCPServerManifest{
			Runtime: types.RuntimeRemote,
			Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
				Key:      "REGION",
				Required: true,
				Options:  []types.MCPConfigurationOption{{Value: "us", Name: "United States"}},
			}}},
			RemoteConfig: &types.RemoteRuntimeConfig{
				IsTemplate:  true,
				URLTemplate: "https://example.com/mcp/${REGION}",
			},
		}},
	}
	storage := newFakeStorage(t, &server)
	gatewayClient := newHandlerTestGateway(t)
	body, err := json.Marshal(map[string]string{"REGION": "us"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/system-mcp-servers/system-server/configure", bytes.NewReader(body))
	req.SetPathValue("id", server.Name)

	err = NewSystemMCPServerHandler(&mcp.SessionManager{}, "").Configure(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        storage,
		GatewayClient:  gatewayClient,
	})
	require.NoError(t, err)

	var updated v1.SystemMCPServer
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: server.Name}, &updated))
	require.NotNil(t, updated.Spec.Manifest.RemoteConfig)
	assert.Equal(t, "https://example.com/mcp/us", updated.Spec.Manifest.RemoteConfig.URL)
}

func TestConfigureSystemMCPServerInvalidRenderedURLPreservesCredential(t *testing.T) {
	server := v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "system-server", Namespace: system.DefaultNamespace},
		Spec: v1.SystemMCPServerSpec{Manifest: types.SystemMCPServerManifest{
			Runtime: types.RuntimeRemote,
			Env:     []types.MCPEnv{{MCPHeader: types.MCPHeader{Key: "HOST", Required: true}}},
			RemoteConfig: &types.RemoteRuntimeConfig{
				IsTemplate:  true,
				URLTemplate: "mailto:${HOST}",
			},
		}},
	}
	storage := newFakeStorage(t, &server)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: server.Name,
		Name:    server.Name,
		Secrets: map[string]string{"HOST": "old.example.com"},
	}))
	body, err := json.Marshal(map[string]string{"HOST": "new.example.com"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/system-mcp-servers/system-server/configure", bytes.NewReader(body))
	req.SetPathValue("id", server.Name)

	err = NewSystemMCPServerHandler(&mcp.SessionManager{}, "").Configure(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		Storage:        storage,
		GatewayClient:  gatewayClient,
	})
	require.Error(t, err)

	credential, revealErr := gatewayClient.RevealCredential(t.Context(), []string{server.Name}, server.Name)
	require.NoError(t, revealErr)
	assert.Equal(t, "old.example.com", credential.Secrets["HOST"])
}
