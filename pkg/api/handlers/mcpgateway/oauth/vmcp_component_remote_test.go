package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestCheckVMCPComponentAuthProbesOnlySelectedRemote(t *testing.T) {
	var selectedCalls, otherCalls atomic.Int32
	var fail atomic.Bool
	server := gomcp.NewServer(&gomcp.Implementation{Name: "selected", Version: "test"}, nil)
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/other" {
			otherCalls.Add(1)
			http.Error(w, "unrelated server must not be probed", http.StatusServiceUnavailable)
			return
		}
		selectedCalls.Add(1)
		if fail.Load() {
			http.Error(w, "selected server is unavailable", http.StatusServiceUnavailable)
			return
		}
		transport.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	vmcp := vmcpComponentVMCP(true)
	vmcp.Spec.Manifest.Components = append(vmcp.Spec.Manifest.Components, types.VMCPComponent{ID: "other"})
	instance := vmcpComponentInstance("vmcpi1selected", vmcp.Name)
	component := vmcpComponentServer("ms1selected", instance.Name, "")
	component.Spec.Manifest.Runtime = types.RuntimeRemote
	component.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{URL: remote.URL + "/selected"}
	other := component.DeepCopy()
	other.Name = "ms1other"
	other.Spec.VMCPComponentID = "other"
	other.Spec.Manifest.RemoteConfig.URL = remote.URL + "/other"
	storage := vmcpConsentStorage(vmcp, instance, component, other)
	gateway := vmcpConsentGateway(t)
	tokens := mcp.NewGlobalTokenStore(gateway)
	// The fake Kubernetes client lets remote OAuth run without a runtime daemon.
	manager, err := mcp.NewSessionManager(t.Context(), false, tokens, nil, "http://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, storage, storage, storage, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	h := &handler{oauthChecker: NewMCPOAuthHandlerFactory("http://obot.example", manager, storage, gateway, tokens, "", false)}
	req := vmcpComponentRequest(storage, instance.Name, component.Name)
	req.GatewayClient = gateway
	recorder := httptest.NewRecorder()
	req.ResponseWriter = recorder
	require.NoError(t, h.checkVMCPComponentAuth(req))
	var response componentAuthStatus
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.Empty(t, response.AuthURL)
	require.Positive(t, selectedCalls.Load())
	require.Zero(t, otherCalls.Load())

	fail.Store(true)
	recorder = httptest.NewRecorder()
	req.ResponseWriter = recorder
	require.Error(t, h.checkVMCPComponentAuth(req))
	require.Empty(t, recorder.Body.String())
	require.Zero(t, otherCalls.Load())
}
