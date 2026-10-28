package tunnel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	"github.com/obot-platform/mmmcp/catalog"
	"github.com/obot-platform/mmmcp/config"
	"github.com/stretchr/testify/require"
)

func TestCompositeAuthorizationLinksAcrossReplicas(t *testing.T) {
	for _, protocol := range []string{"2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			var authorized atomic.Bool
			component := mcp.NewServer(&mcp.Implementation{Name: "Gmail", Version: "test"}, nil)
			component.AddTool(&mcp.Tool{Name: "gmail", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "connected"}}}, nil
			})
			transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return component }, nil)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !authorized.Load() {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				transport.ServeHTTP(w, r)
			}))
			defer upstream.Close()
			composite, err := mmmcp.New(ctx, &config.Config{
				DeferAuthorization: true,
				Servers:            []config.Server{{Name: "Gmail", URL: upstream.URL}},
			}, mmmcp.Options{AuthorizationURLs: func(context.Context) ([]mmmcp.AuthorizationURL, error) {
				return []mmmcp.AuthorizationURL{{Message: "Connect Gmail", URL: "https://obot.example/auth/mcp/composite/vmcpi1user"}}, nil
			}})
			require.NoError(t, err)
			defer composite.Close()
			owner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				composite.HTTPHandler().ServeHTTP(w, r.WithContext(mmmcp.ContextWithConfigID(r.Context(), "claude-user")))
			})
			first, firstServer := newCompositeManagerTestServer(ctx, t, owner)
			defer firstServer.Close()
			defer first.Close()
			second, secondServer := newCompositeManagerTestServer(ctx, t, http.NotFoundHandler())
			defer secondServer.Close()
			defer second.Close()
			configureTestPeer(first, "replica-a", "authorization-peer")
			configureTestPeer(second, "replica-b", "authorization-peer")
			first.ReconcilePeers(map[string]string{"replica-b": strings.Replace(secondServer.URL, "http://", "ws://", 1) + PeerConnectPath})
			second.ReconcilePeers(map[string]string{"replica-a": strings.Replace(firstServer.URL, "http://", "ws://", 1) + PeerConnectPath})
			key := CompositeSessionKey("resolved-server")
			require.NoError(t, first.ClaimCompositeSession(ctx, ctx, key))
			waitForTestCondition(t, 10*time.Second, func() bool { return second.HasCompositeSession(key) }, "second replica did not learn owner")

			var requests atomic.Int32
			ingress := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				manager := first
				if requests.Add(1)%2 == 0 {
					manager = second
				}
				if err := manager.ForwardComposite(w, r, key, "resolved-server"); err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
				}
			}))
			defer ingress.Close()
			var prompts atomic.Int32
			client := mcp.NewClient(&mcp.Implementation{Name: "Claude", Version: "test"}, &mcp.ClientOptions{
				Capabilities: &mcp.ClientCapabilities{Elicitation: &mcp.ElicitationCapabilities{URL: &mcp.URLElicitationCapabilities{}}},
				ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					require.Equal(t, "url", req.Params.Mode)
					require.Equal(t, "https://obot.example/auth/mcp/composite/vmcpi1user", req.Params.URL)
					prompts.Add(1)
					return &mcp.ElicitResult{Action: "accept"}, nil
				},
			})
			session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ingress.URL, HTTPClient: ingress.Client()}, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
			require.NoError(t, err)
			defer session.Close()
			pending, err := session.CallTool(ctx, &mcp.CallToolParams{Name: catalog.AuthorizationToolName})
			require.NoError(t, err)
			require.False(t, pending.IsError)
			require.Zero(t, prompts.Load())
			require.False(t, pending.NeedsInput())
			require.Empty(t, pending.InputRequests)
			require.Contains(t, pending.Content[0].(*mcp.TextContent).Text, "<https://obot.example/auth/mcp/composite/vmcpi1user>")

			authorized.Store(true)
			// Pending authorization retries are coalesced and briefly throttled.
			require.Eventually(t, func() bool {
				refreshed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: catalog.AuthorizationToolName})
				return err == nil && !refreshed.IsError && strings.Contains(refreshed.Content[0].(*mcp.TextContent).Text, "Authorization complete")
			}, 5*time.Second, 100*time.Millisecond)
			listed, err := session.ListTools(ctx, nil)
			require.NoError(t, err)
			require.Len(t, listed.Tools, 1)
			require.Equal(t, "gmail", listed.Tools[0].Name)
			called, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "gmail"})
			require.NoError(t, err)
			require.False(t, called.IsError)
			require.Greater(t, requests.Load(), int32(2))
		})
	}
}
