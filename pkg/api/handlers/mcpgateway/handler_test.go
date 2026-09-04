package mcpgateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/pkg/api"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/safehttp"
	"golang.org/x/oauth2"
)

func TestCompositeUpstreamClientIdentity(t *testing.T) {
	identities := make(chan gomcp.Implementation, 2)
	server := gomcp.NewServer(&gomcp.Implementation{Name: "component", Version: "test"}, nil)
	server.AddTool(&gomcp.Tool{Name: "echo", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		if info := req.ClientInfo(); info != nil {
			identities <- *info
		}
		return &gomcp.CallToolResult{}, nil
	})
	upstream := httptest.NewServer(gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil))
	defer upstream.Close()
	handler, err := NewHandler(t.Context(), nil, nil, nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	cfg := &mmmcpconfig.Config{Servers: []mmmcpconfig.Server{{Name: "component", URL: upstream.URL}}}
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.composite.HTTPHandler().ServeHTTP(w, r.WithContext(mmmcp.ContextWithConfig(r.Context(), cfg)))
	}))
	defer frontend.Close()
	for _, want := range []gomcp.Implementation{
		{Name: "first-client", Version: "1.2.3"},
		{Name: "second-client", Version: "4.5.6"},
	} {
		client := gomcp.NewClient(&want, nil)
		session, err := client.Connect(t.Context(), &gomcp.StreamableClientTransport{Endpoint: frontend.URL, DisableStandaloneSSE: true}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		if _, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: "echo"}); err != nil {
			t.Fatal(err)
		}
		select {
		case identity := <-identities:
			if identity.Name != want.Name || identity.Version != want.Version {
				t.Fatalf("upstream identity = %#v, want %#v", identity, want)
			}
		default:
			t.Fatal("no component client identity observed")
		}
	}
}

func TestProxyRewritesUpstreamUnauthorized(t *testing.T) {
	for _, tc := range []struct {
		path   string
		id     string
		status int
	}{
		{
			path:   "/mcp-connect/ms1remote",
			id:     "ms1remote",
			status: http.StatusUnauthorized,
		},
		{
			path:   "/mcp-connect/vmcp1shared",
			id:     "vmcp1shared",
			status: http.StatusUnauthorized,
		},
		{
			path:   "/mcp-connect-composite/vmcpi1legacy",
			id:     "vmcpi1legacy",
			status: http.StatusUnauthorized,
		},
		{
			path:   "/mcp-connect/ms1remote",
			id:     "ms1remote",
			status: http.StatusForbidden,
		},
	} {
		t.Run(tc.path+http.StatusText(tc.status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://upstream.example/metadata"`)
				w.Header().Set("Set-Cookie", "upstream-secret=value")
				http.Error(w, "upstream authentication details", tc.status)
			}))
			defer upstream.Close()
			u, err := url.Parse(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, tc.path, nil)
			r.SetPathValue("mcp_id", tc.id)
			w := httptest.NewRecorder()
			ctx := api.Context{Request: r, ResponseWriter: w, APIBaseURL: "https://obot.example/api"}
			proxy := httputil.NewSingleHostReverseProxy(u)
			proxy.ModifyResponse = func(resp *http.Response) error {
				rewriteMCPAuthResponse(ctx, resp)
				return nil
			}
			proxy.ServeHTTP(w, r)
			resp := w.Result()
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if tc.status == http.StatusUnauthorized {
				want := `Bearer realm="Obot MCP Gateway", resource_metadata="https://obot.example/.well-known/oauth-protected-resource` + tc.path + `"`
				if resp.Header.Get("WWW-Authenticate") != want {
					t.Fatalf("challenge = %q, want %q", resp.Header.Get("WWW-Authenticate"), want)
				}
				if string(body) != "MCP server requires authentication\n" || resp.Header.Get("Set-Cookie") != "" {
					t.Fatalf("upstream response leaked: headers=%v body=%q", resp.Header, body)
				}
			} else if string(body) != "upstream authentication details\n" {
				t.Fatalf("non-401 response changed: %q", body)
			}
		})
	}
}

func TestProxyStripsInboundGatewayCredentials(t *testing.T) {
	tests := []struct {
		name                      string
		configuredUpstreamHeaders http.Header
		tokenSource               oauth2.TokenSource
		wantAuthorization         string
		wantAPIKey                string
	}{
		{
			name: "non-authorization upstream credential",
			configuredUpstreamHeaders: http.Header{
				"X-Filesapi-Key": {"files-api-key"},
			},
		},
		{
			name: "configured upstream authorization",
			configuredUpstreamHeaders: http.Header{
				"Authorization":  {"Bearer configured-upstream-token"},
				"X-API-Key":      {"configured-upstream-api-key"},
				"X-Filesapi-Key": {"files-api-key"},
			},
			wantAuthorization: "Bearer configured-upstream-token",
			wantAPIKey:        "configured-upstream-api-key",
		},
		{
			name: "upstream OAuth authorization",
			configuredUpstreamHeaders: http.Header{
				"X-Filesapi-Key": {"files-api-key"},
			},
			tokenSource:       oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "oauth-upstream-token"}),
			wantAuthorization: "Bearer oauth-upstream-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedHeaders := make(chan http.Header, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				receivedHeaders <- req.Header.Clone()
				w.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()

			upstreamURL, err := url.Parse(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}

			proxy := httptest.NewServer(&httputil.ReverseProxy{
				Transport: safehttp.NewSafeTransport(safehttp.Options{
					Headers:     tt.configuredUpstreamHeaders,
					TokenSource: tt.tokenSource,
				}),
				Rewrite: func(req *httputil.ProxyRequest) {
					rewriteProxyRequest(req, upstreamURL)
				},
			})
			defer proxy.Close()

			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, proxy.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer obot-gateway-jwt")
			request.Header.Set("Cookie", "obot_access_token=local-session-secret")
			request.Header.Set("Proxy-Authorization", "Bearer obot-proxy-token")
			request.Header.Set("X-API-Key", "obot-api-key")

			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusNoContent)
			}

			got := <-receivedHeaders
			if got.Get("X-FilesAPI-Key") != "files-api-key" {
				t.Fatalf("X-FilesAPI-Key = %q, want files-api-key", got.Get("X-FilesAPI-Key"))
			}
			if got.Get("Authorization") != tt.wantAuthorization {
				t.Fatalf("Authorization = %q, want %q", got.Get("Authorization"), tt.wantAuthorization)
			}
			if got.Get("Cookie") != "" {
				t.Fatalf("Cookie = %q, want empty", got.Get("Cookie"))
			}
			if got.Get("Proxy-Authorization") != "" {
				t.Fatalf("Proxy-Authorization = %q, want empty", got.Get("Proxy-Authorization"))
			}
			if got.Get("X-API-Key") != tt.wantAPIKey {
				t.Fatalf("X-API-Key = %q, want %q", got.Get("X-API-Key"), tt.wantAPIKey)
			}
		})
	}
}

func TestMCPJSONRPCErrorPropagatesThroughTransport(t *testing.T) {
	deploymentErr := errors.New("MCP server is not healthy: container repeatedly crashed (exit code 1, 4 restarts)")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !writeMCPJSONRPCError(w, req, deploymentErr) {
			http.Error(w, deploymentErr.Error(), http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := gomcp.NewClient(&gomcp.Implementation{Name: "test", Version: "test"}, nil)
	_, err := client.Connect(t.Context(), &gomcp.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           server.Client(),
		DisableStandaloneSSE: true,
	}, nil)
	if err == nil {
		t.Fatal("Connect() error = nil, want deployment health error")
	}
	if !strings.Contains(err.Error(), deploymentErr.Error()) {
		t.Fatalf("Connect() error = %q, want it to contain %q", err, deploymentErr)
	}
}

func TestCompositeLoopbackURLsUsesBackendTargetAndPublicAudience(t *testing.T) {
	const (
		serverURL       = "https://obot.example.com/"
		mcpServerName   = "mcp-composite"
		internalBaseURL = "http://obot.obot-system.svc.cluster.local"
	)

	var transformedURL string
	audienceURL, targetURL := compositeLoopbackURLs(serverURL, mcpServerName, func(rawURL string) string {
		transformedURL = rawURL
		return internalBaseURL + "/mcp-connect-composite/" + mcpServerName
	})

	wantAudienceURL := "https://obot.example.com/mcp-connect-composite/mcp-composite"
	if audienceURL != wantAudienceURL {
		t.Fatalf("audience URL = %q, want %q", audienceURL, wantAudienceURL)
	}
	if transformedURL != wantAudienceURL {
		t.Fatalf("URL passed to backend transform = %q, want %q", transformedURL, wantAudienceURL)
	}
	wantTargetURL := "http://obot.obot-system.svc.cluster.local/mcp-connect-composite/mcp-composite"
	if targetURL != wantTargetURL {
		t.Fatalf("target URL = %q, want %q", targetURL, wantTargetURL)
	}
}

func TestCompositeSessionKeyUsesResolvedServerName(t *testing.T) {
	firstAlias := obotmcp.ServerConfig{MCPServerName: "resolved-composite", UserID: "user1"}
	secondAlias := obotmcp.ServerConfig{MCPServerName: "resolved-composite", UserID: "user2"}
	if compositeSessionKey(firstAlias) != compositeSessionKey(secondAlias) {
		t.Fatal("different URL IDs resolving to one MCP server name did not share an affinity key")
	}
	if compositeSessionKey(firstAlias) == compositeSessionKey(obotmcp.ServerConfig{MCPServerName: "other-composite"}) {
		t.Fatal("different MCP server names shared an affinity key")
	}
}
