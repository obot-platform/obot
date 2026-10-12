package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tunnel"
	"golang.org/x/oauth2"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	errStopAfterDiscovery = errors.New("stop after discovery")
)

// stopAfterDiscoveryCallbackHandler records that OAuth discovery succeeded and stops the flow
// before the user would be sent to the authorization server.
type stopAfterDiscoveryCallbackHandler struct {
	mu          sync.Mutex
	reached     bool
	resourceURL string
}

func TestHTTPClientRoutesOAuthDiscoveryThroughTunnel(t *testing.T) {
	const tunnelName = "office"

	var (
		requestsMu         sync.Mutex
		requests           = make(map[string]int)
		bridgeRequestCount int
		targetBaseURL      string
	)
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestsMu.Lock()
		requests[request.Method+" "+request.URL.Path]++
		requestsMu.Unlock()

		status := http.StatusOK
		header := make(http.Header)
		body := ""
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/mcp":
			status = http.StatusUnauthorized
			header.Set("WWW-Authenticate", "Bearer")
			body = "unauthorized"
		case request.Method == http.MethodGet && request.URL.Path == "/.well-known/oauth-protected-resource/mcp":
			status = http.StatusNotFound
		case request.Method == http.MethodGet && request.URL.Path == "/.well-known/oauth-protected-resource":
			body = `{"resource":"` + targetBaseURL + `","authorization_servers":["` + targetBaseURL + `/issuer"]}`
		case request.Method == http.MethodGet && request.URL.Path == "/.well-known/oauth-authorization-server/issuer":
			body = `{"issuer":"` + targetBaseURL + `/issuer","authorization_endpoint":"` + targetBaseURL +
				`/authorize","token_endpoint":"` + targetBaseURL + `/token","registration_endpoint":"` +
				targetBaseURL + `/register","response_types_supported":["code"]}`
		default:
			status = http.StatusNotFound
		}
		if body != "" {
			header.Set("Content-Type", "application/json")
		}
		for key, values := range header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	targetBaseURL = targetServer.URL
	defer targetServer.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	token, credential, err := tunnel.NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	credentialID, err := tunnel.CredentialID(credential)
	if err != nil {
		t.Fatal(err)
	}
	tunnelConfig := &v1.MCPTunnel{
		Name:      tunnelName,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPTunnelSpec{
			Manifest: types.MCPTunnelManifest{
				DisplayName: "Office",
				AllowedURLs: []string{"*"},
			},
			Credential:   credential,
			CredentialID: credentialID,
		},
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(tunnelConfig).
		Build()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := tunnel.NewManager(ctx, "http://"+listener.Addr().String(), storageClient, tunnel.PeerConfig{})
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	defer manager.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /tunnel/connect", func(w http.ResponseWriter, request *http.Request) {
		manager.ServeConnect(w, request, tunnelName)
	})
	mux.HandleFunc("GET "+tunnel.PeerConnectPath, manager.ServePeer)
	mux.HandleFunc("/tunnel/bridge/{target}", func(w http.ResponseWriter, request *http.Request) {
		requestsMu.Lock()
		bridgeRequestCount++
		requestsMu.Unlock()
		manager.ServeBridge(w, request)
	})
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()

	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- tunnel.Run(ctx, server.URL, token)
	}()
	waitForOAuthTunnelConnection(t, manager, tunnelName)

	metadata, err := (&SessionManager{tunnelManager: manager}).GetOAuthMetadata(
		t.Context(),
		ServerConfig{TunnelName: tunnelName, URL: targetBaseURL + "/mcp"},
		"Obot Test MCP OAuth Client",
		"https://obot.example.com/oauth/mcp/callback",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	var authServer AuthorizationServerMetadata
	if err := json.Unmarshal(metadata.AuthorizationServerMetadata, &authServer); err != nil {
		t.Fatal(err)
	}
	if authServer.AuthorizationEndpoint != targetBaseURL+"/authorize" ||
		authServer.TokenEndpoint != targetBaseURL+"/token" ||
		authServer.RegistrationEndpoint != targetBaseURL+"/register" {
		t.Fatalf("unexpected authorization server metadata: %#v", authServer)
	}

	expectedRequests := []string{
		"POST /mcp",
		"GET /.well-known/oauth-protected-resource/mcp",
		"GET /.well-known/oauth-protected-resource",
		"GET /.well-known/oauth-authorization-server/issuer",
	}
	requestsMu.Lock()
	for _, request := range expectedRequests {
		forwardedRequests := requests[request]
		if forwardedRequests == 0 {
			t.Errorf("OAuth discovery did not send %s through the tunnel", request)
		}
	}
	if bridgeRequestCount != len(expectedRequests) {
		t.Errorf("OAuth discovery sent %d bridge requests, want %d", bridgeRequestCount, len(expectedRequests))
	}
	requestsMu.Unlock()

	cancel()
	select {
	case err := <-serveErrors:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("tunnel client stopped with error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("tunnel client did not stop")
	}
}

func waitForOAuthTunnelConnection(t *testing.T, manager *tunnel.Manager, name string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		for _, connection := range manager.Connections() {
			if connection.Name == name {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("tunnel session did not connect")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (*stopAfterDiscoveryCallbackHandler) HandleAuthURL(context.Context, string, string) (bool, error) {
	return false, nil
}

func (h *stopAfterDiscoveryCallbackHandler) NewState(_ context.Context, _ *oauth2.Config, resourceURL, _ string) (string, <-chan CallbackPayload, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reached = true
	h.resourceURL = resourceURL
	return "", nil, errStopAfterDiscovery
}

func TestOAuthCheckClientAcceptsTunneledServerResource(t *testing.T) {
	const tunnelName = "office"

	var targetBaseURL string
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/mcp":
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+targetBaseURL+`/.well-known/oauth-protected-resource/mcp"`)
			w.WriteHeader(http.StatusUnauthorized)
		case request.Method == http.MethodGet && request.URL.Path == "/.well-known/oauth-protected-resource/mcp":
			// The server names its own URL, not the bridge URL Obot reaches it through.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"resource":"` + targetBaseURL + `/mcp","authorization_servers":["` + targetBaseURL + `/issuer"]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/.well-known/oauth-authorization-server/issuer":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issuer":"` + targetBaseURL + `/issuer","authorization_endpoint":"` + targetBaseURL +
				`/authorize","token_endpoint":"` + targetBaseURL + `/token","response_types_supported":["code"]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	targetBaseURL = targetServer.URL
	defer targetServer.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	token, credential, err := tunnel.NewCredential()
	if err != nil {
		t.Fatal(err)
	}
	credentialID, err := tunnel.CredentialID(credential)
	if err != nil {
		t.Fatal(err)
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.MCPTunnel{
			Name:      tunnelName,
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPTunnelSpec{
				Manifest: types.MCPTunnelManifest{
					DisplayName: "Office",
					AllowedURLs: []string{"*"},
				},
				Credential:   credential,
				CredentialID: credentialID,
			},
		}).
		Build()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := tunnel.NewManager(ctx, "http://"+listener.Addr().String(), storageClient, tunnel.PeerConfig{})
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	defer manager.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /tunnel/connect", func(w http.ResponseWriter, request *http.Request) {
		manager.ServeConnect(w, request, tunnelName)
	})
	mux.HandleFunc("GET "+tunnel.PeerConnectPath, manager.ServePeer)
	mux.HandleFunc("/tunnel/bridge/{target}", manager.ServeBridge)
	server := httptest.NewUnstartedServer(mux)
	server.Listener = listener
	server.Start()
	defer server.Close()

	go func() {
		_ = tunnel.Run(ctx, server.URL, token)
	}()
	waitForOAuthTunnelConnection(t, manager, tunnelName)

	callback := &stopAfterDiscoveryCallbackHandler{}
	sm := &SessionManager{tunnelManager: manager, baseURL: "https://obot.example.com"}
	_, err = sm.ClientForMCPServerForOAuthCheck(t.Context(), ServerConfig{
		MCPServerName: "tunneled-server",
		UserID:        "user-1",
		Runtime:       types.RuntimeRemote,
		TunnelName:    tunnelName,
		URL:           targetBaseURL + "/mcp",
	}, ClientOption{
		ClientName:      "Obot Test",
		TokenStorage:    &recordingTokenStorage{},
		CallbackHandler: callback,
		ClientLookup:    &oauthTestClientCredLookup{clientID: "static-client"},
	})
	if err == nil {
		t.Fatal("expected the OAuth flow to stop after discovery")
	}

	callback.mu.Lock()
	defer callback.mu.Unlock()
	if !callback.reached {
		t.Fatalf("OAuth discovery did not succeed for the tunneled server: %v", err)
	}
	if callback.resourceURL != targetBaseURL+"/mcp" {
		t.Fatalf("resource = %q, want %q", callback.resourceURL, targetBaseURL+"/mcp")
	}
}
