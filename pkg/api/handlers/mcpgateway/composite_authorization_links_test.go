package mcpgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	"github.com/obot-platform/mmmcp/catalog"
	"github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestCompositeAuthorizationLinksPreservesRequestIdentity(t *testing.T) {
	for _, protocol := range []string{"2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			component := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
			defer component.Close()
			gateway, err := NewHandler(t.Context(), nil, nil, nil, nil, "", "", "", nil, func(req api.Context) ([]mmmcp.AuthorizationURL, error) {
				require.Equal(t, "42", req.User.GetUID())
				require.Equal(t, "vmcpi1user", req.PathValue("mcp_id"))
				return []mmmcp.AuthorizationURL{{URL: "https://obot.example/auth/mcp/composite/vmcpi1user", Message: "Connect Gmail"}}, nil
			}, nil)
			require.NoError(t, err)
			defer gateway.Close()
			cfg := &config.Config{DeferAuthorization: true, Servers: []config.Server{{Name: "Gmail", URL: component.URL}}}
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.SetPathValue("mcp_id", "vmcpi1user")
				req := api.Context{Request: r, User: &user.DefaultInfo{UID: "42", Groups: []string{types.GroupClaudeEnvironment}}}
				ctx := context.WithValue(r.Context(), compositeAPIContextKey{}, req)
				ctx = mmmcp.ContextWithConfig(ctx, cfg)
				gateway.composite.HTTPHandler().ServeHTTP(w, r.WithContext(ctx))
			}))
			defer frontend.Close()
			var prompts atomic.Int32
			client := gomcp.NewClient(&gomcp.Implementation{Name: "Claude", Version: "test"}, &gomcp.ClientOptions{
				Capabilities: &gomcp.ClientCapabilities{Elicitation: &gomcp.ElicitationCapabilities{URL: &gomcp.URLElicitationCapabilities{}}},
				ElicitationHandler: func(_ context.Context, req *gomcp.ElicitRequest) (*gomcp.ElicitResult, error) {
					prompts.Add(1)
					require.Equal(t, "url", req.Params.Mode)
					require.Equal(t, "https://obot.example/auth/mcp/composite/vmcpi1user", req.Params.URL)
					return &gomcp.ElicitResult{Action: "cancel"}, nil
				},
			})
			session, err := client.Connect(t.Context(), &gomcp.StreamableClientTransport{Endpoint: frontend.URL}, &gomcp.ClientSessionOptions{ProtocolVersion: protocol})
			require.NoError(t, err)
			defer session.Close()
			result, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: catalog.AuthorizationToolName})
			require.NoError(t, err)
			require.False(t, result.IsError)
			require.Zero(t, prompts.Load())
			require.False(t, result.NeedsInput())
			require.Empty(t, result.InputRequests)
			require.Contains(t, result.Content[0].(*gomcp.TextContent).Text, "https://obot.example/auth/mcp/composite/vmcpi1user")
		})
	}
}
