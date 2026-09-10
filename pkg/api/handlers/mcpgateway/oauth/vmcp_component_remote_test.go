package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// This fixture only watches objects already created by the test.
type vmcpOAuthInitialEventsClient struct{ kclient.WithWatch }

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

func TestVMCPReconnectDoesNotReuseSharedServerTokens(t *testing.T) {
	var authenticated, anonymous atomic.Int32
	server := gomcp.NewServer(&gomcp.Implementation{Name: "component", Version: "test"}, nil)
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			anonymous.Add(1)
		} else {
			authenticated.Add(1)
		}
		transport.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	vmcp := vmcpComponentVMCP(false)
	component := vmcpComponentServer("ms1shared", "", vmcp.Name)
	component.Spec.Manifest.Runtime = types.RuntimeRemote
	component.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{URL: remote.URL}
	vmcp.Spec.Manifest.Components[0].CatalogEntry.Manifest = types.MCPServerCatalogEntryManifest{
		Runtime:      types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{FixedURL: remote.URL},
	}
	storage := &vmcpOAuthInitialEventsClient{WithWatch: vmcpConsentStorage(vmcp, component)}
	gateway := vmcpConsentGateway(t)
	tokens := mcp.NewGlobalTokenStore(gateway)
	// Leave a legacy token under the shared server ID: no connection may reuse it.
	require.NoError(t, tokens.ForUserAndMCP("42", component.Name, remote.URL).SetTokenConfig(t.Context(),
		&oauth2.Config{ClientID: "legacy"}, &oauth2.Token{AccessToken: "legacy-token", TokenType: "Bearer"}))
	manager, err := mcp.NewSessionManager(t.Context(), false, tokens, nil, "http://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, storage, storage, storage, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	h := &handler{oauthChecker: NewMCPOAuthHandlerFactory("http://obot.example", manager, storage, gateway, tokens, "", false)}
	for _, suffix := range []string{"old", "new"} {
		instance := vmcpComponentInstance("vmcpi1"+suffix, vmcp.Name)
		connection := &v1.MCPServerInstance{
			Name:      "msi1" + suffix,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				VMCPInstanceID:  instance.Name,
				VMCPComponentID: "component",
				MCPServerName:   component.Name,
				UserID:          "42",
			},
		}
		require.NoError(t, storage.Create(t.Context(), instance))
		require.NoError(t, storage.Create(t.Context(), connection))
		if suffix == "old" {
			require.NoError(t, tokens.ForUserAndMCP("42", connection.Name, remote.URL).SetTokenConfig(t.Context(),
				&oauth2.Config{ClientID: "connection"}, &oauth2.Token{AccessToken: "connection-token", TokenType: "Bearer"}))
		}
		req := vmcpComponentRequest(storage, instance.Name, component.Name)
		req.GatewayClient = gateway
		_, config, err := manager.ServerForAction(t.Context(), connection.Name, "42")
		require.NoError(t, err)
		require.Equal(t, connection.Name, config.MCPServerInstanceID, "gateway must select the connection's token")
		aggregate, aggregateConfig, err := manager.ServerForAction(t.Context(), instance.Name, "42")
		require.NoError(t, err)
		for _, check := range []func() error{
			func() error { return h.checkVMCPComponentAuth(req) },
			func() error { return h.checkVMCPAuth(req) },
			func() error {
				_, err := h.oauthChecker.CheckForMCPAuth(req, aggregate, aggregateConfig, "42", instance.Name, "")
				return err
			},
		} {
			authenticated.Store(0)
			anonymous.Store(0)
			require.NoError(t, check())
			if suffix == "old" {
				require.Positive(t, authenticated.Load())
				require.Zero(t, anonymous.Load())
			} else {
				require.Zero(t, authenticated.Load(), "recreated connection reused an old token")
				require.Positive(t, anonymous.Load())
			}
		}
		// Simulate cascading deletion and the connection's token finalizer.
		require.NoError(t, storage.Delete(t.Context(), instance))
		require.NoError(t, storage.Delete(t.Context(), connection))
		require.NoError(t, gateway.DeleteMCPOAuthTokenForAllUsers(t.Context(), connection.Name))
		var remaining v1.MCPServer
		require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(component), &remaining))
	}
}

func (c *vmcpOAuthInitialEventsClient) Watch(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) (watch.Interface, error) {
	options := &kclient.ListOptions{}
	options.ApplyOptions(opts)
	options.Raw = nil
	if err := c.List(ctx, list, options); err != nil {
		return nil, err
	}
	objects, err := meta.ExtractList(list)
	if err != nil {
		return nil, err
	}
	w := watch.NewRaceFreeFake()
	for _, object := range objects {
		w.Add(object)
	}
	return w, nil
}
