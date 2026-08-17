package mcpgateway

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/pkg/safehttp"
	"golang.org/x/oauth2"
)

func TestProxyStripsInboundGatewayAuthorization(t *testing.T) {
	tests := []struct {
		name                      string
		configuredUpstreamHeaders http.Header
		tokenSource               oauth2.TokenSource
		wantAuthorization         string
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
				"X-Filesapi-Key": {"files-api-key"},
			},
			wantAuthorization: "Bearer configured-upstream-token",
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
