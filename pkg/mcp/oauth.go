package mcp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/nanobot/pkg/safehttp"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// RequiresStaticOAuth reports whether a remote MCP server is configured to
// require static OAuth and has not yet been authenticated by the user.
func RequiresStaticOAuth(server v1.MCPServer) bool {
	return server.Spec.Manifest.Runtime == types.RuntimeRemote &&
		server.Spec.Manifest.RemoteConfig != nil &&
		server.Spec.Manifest.RemoteConfig.StaticOAuthRequired &&
		!server.Status.UserHasAuthenticated
}

// GetOAuthMetadata discovers OAuth metadata for a remote MCP server. When
// assumeOAuthRequired is true, discovery proceeds even if initialize succeeds.
// This supports servers that defer their OAuth challenge until tools/call.
func (m *SessionManager) GetOAuthMetadata(ctx context.Context, serverConfig ServerConfig, clientName, redirectURL string, assumeOAuthRequired bool) (nmcp.OAuthMetadata, error) {
	var httpClient *http.Client
	if serverConfig.TunnelName != "" {
		var err error
		httpClient, err = m.HTTPClientForServer(serverConfig, 5*time.Second)
		if err != nil {
			return nmcp.OAuthMetadata{}, err
		}
	} else {
		blockingConfig := m.RemoteMCPURLValidationConfig()
		httpClient = safehttp.NewClientWithTimeout(
			!blockingConfig.AllowLocalhostMCP,
			!blockingConfig.AllowPrivateIPMCP,
			!blockingConfig.AllowLinkLocalMCP,
			5*time.Second,
		)
	}

	return getOAuthMetadataWithClient(ctx, httpClient, serverConfig, clientName, redirectURL, assumeOAuthRequired)
}

func getOAuthMetadataWithClient(ctx context.Context, httpClient *http.Client, serverConfig ServerConfig, clientName, redirectURL string, assumeOAuthRequired bool) (nmcp.OAuthMetadata, error) {
	if assumeOAuthRequired {
		cloned := *httpClient
		transport := cloned.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		cloned.Transport = &assumeOAuthRequiredTransport{base: transport}
		httpClient = &cloned
	}

	return nmcp.GetOAuthMetadataWithClient(ctx, httpClient, nmcp.Server{
		BaseURL: serverConfig.URL,
		Headers: serverConfigHeaders(serverConfig),
	}, clientName, redirectURL)
}

func serverConfigHeaders(serverConfig ServerConfig) map[string]string {
	result := make(map[string]string, len(serverConfig.PassthroughHeaderNames)+len(serverConfig.Headers))
	for i, key := range serverConfig.PassthroughHeaderNames {
		if i < len(serverConfig.PassthroughHeaderValues) {
			result[key] = serverConfig.PassthroughHeaderValues[i]
		}
	}
	for _, header := range serverConfig.Headers {
		key, value, ok := strings.Cut(header, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

// assumeOAuthRequiredTransport synthesizes the initialize challenge used to
// begin standard OAuth metadata discovery. All subsequent metadata requests use
// the original, policy-enforcing transport.
type assumeOAuthRequiredTransport struct {
	base http.RoundTripper
	mu   sync.Mutex
	done bool
}

func (t *assumeOAuthRequiredTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	if !t.done && req.Method == http.MethodPost {
		t.done = true
		t.mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Status:     "401 Unauthorized",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	}
	t.mu.Unlock()
	return t.base.RoundTrip(req)
}
