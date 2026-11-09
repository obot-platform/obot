package oauth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestToolPreviewAuthDetectsLateChallenge(t *testing.T) {
	for _, outcome := range []string{"oauth", "success", "failure"} {
		t.Run(outcome, func(t *testing.T) {
			server := gomcp.NewServer(&gomcp.Implementation{Name: "preview", Version: "test"}, nil)
			server.AddTool(&gomcp.Tool{Name: "test", InputSchema: map[string]any{"type": "object"}}, nil)
			transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
			var base string
			var toolCalls atomic.Int32
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/metadata":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"resource":              base + "/mcp",
						"authorization_servers": []string{base},
					})
					return
				case "/.well-known/oauth-authorization-server":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"issuer":                   base,
						"response_types_supported": []string{"code"},
						"authorization_endpoint":   base + "/authorize",
						"token_endpoint":           base + "/token",
					})
					return
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				var message struct{ Method string }
				_ = json.Unmarshal(body, &message)
				if message.Method == "tools/list" {
					toolCalls.Add(1)
					switch outcome {
					case "oauth":
						w.Header().Set("WWW-Authenticate", "Bearer resource_metadata=\""+base+"/metadata\", scope=\"tools:read\"")
						http.Error(w, "login required", http.StatusUnauthorized)
						return
					case "failure":
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
				}
				transport.ServeHTTP(w, r)
			}))
			t.Cleanup(remote.Close)
			base = remote.URL
			storage := vmcpConsentStorage()
			gateway := vmcpConsentGateway(t)
			require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: system.MCPOAuthCredentialName("source"),
				Name:    system.StaticOAuthCredentialName,
				Secrets: map[string]string{"CLIENT_ID": "preview-client", "CLIENT_SECRET": "secret"},
			}))
			tokens := mcp.NewGlobalTokenStore(gateway)
			manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "http://obot.example", 8080,
				mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
				&rest.Config{Host: "http://kubernetes.invalid"}, storage, storage, storage, gateway, system.DefaultNamespace, nil)
			require.NoError(t, err)
			factory := NewMCPOAuthHandlerFactory("http://obot.example", manager, storage, gateway, tokens, "", false)
			req := vmcpComponentRequest(storage, "vmcp", "component")
			req.GatewayClient = gateway
			preview := v1.MCPServer{
				Name: "vmcp-tool-preview-test",
				Spec: v1.MCPServerSpec{Manifest: types.MCPServerManifest{Runtime: types.RuntimeRemote}},
			}
			config := mcp.ServerConfig{
				Runtime:             types.RuntimeRemote,
				URL:                 base + "/mcp",
				MCPServerName:       preview.Name,
				MCPCatalogEntryName: "source",
			}
			// Initialization alone succeeds, reproducing the old preview preflight.
			authURL, err := factory.CheckForMCPAuth(req, preview, config, "system", preview.Name, "")
			require.NoError(t, err)
			require.Empty(t, authURL)

			authURL, err = factory.CheckForMCPToolAuth(req, preview, config, "system", preview.Name, "")
			if outcome == "failure" {
				require.ErrorContains(t, err, "failed to list tools")
				require.Empty(t, authURL)
			} else {
				require.NoError(t, err)
				if outcome == "oauth" {
					parsed, err := url.Parse(authURL)
					require.NoError(t, err)
					require.Equal(t, base+"/authorize", parsed.Scheme+"://"+parsed.Host+parsed.Path)
					require.Equal(t, "tools:read", parsed.Query().Get("scope"))
					require.Equal(t, "preview-client", parsed.Query().Get("client_id"))
				} else {
					require.Empty(t, authURL)
				}
			}
			require.Positive(t, toolCalls.Load())
		})
	}
}
