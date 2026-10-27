package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	mmmcpcomponent "github.com/obot-platform/mmmcp/component"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/rest"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestResponseChallengesSelectFirstAlternative(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		headers []string
		count   int
	}{
		{
			name:   "ordinary forbidden",
			status: http.StatusForbidden,
			count:  0,
		},
		{
			name:   "JSON-RPC error alone",
			status: http.StatusOK,
			count:  0,
		},
		{
			name:    "forbidden invalid token",
			status:  http.StatusForbidden,
			headers: []string{`Bearer error="invalid_token"`},
			count:   0,
		},
		{
			name:    "basic challenge still requires Obot OAuth",
			status:  http.StatusUnauthorized,
			headers: []string{`Basic realm="legacy"`},
			count:   1,
		},
		{
			name:   "missing metadata",
			status: http.StatusUnauthorized,
			count:  1,
		},
		{
			name:    "multiple providers",
			status:  http.StatusForbidden,
			headers: []string{`Basic realm="legacy"`, `Bearer realm="one", error="insufficient_scope", error_uri="https://one.test/error", resource_metadata="https://one.test/meta", scope="read"`, `Bearer error="insufficient_scope", resource_metadata="https://two.test/meta", scope="write"`},
			count:   1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode: test.status,
				Header:     make(http.Header),
			}

			for _, value := range test.headers {
				response.Header.Add("WWW-Authenticate", value)
			}

			req := api.Context{User: &user.DefaultInfo{Groups: []string{types.GroupCompositeMCP}}}
			require.NoError(t, rewriteUpstreamOAuthResponse(req, response))

			challenges := mmmcpcomponent.AuthorizationChallenges(response)

			require.Len(t, challenges, test.count)

			if test.name == "multiple providers" {
				require.Equal(t, "https://one.test/meta", challenges[0].ResourceMetadata)
				require.Equal(t, "read", challenges[0].Scope)
				require.Equal(t, "one", challenges[0].Realm)
				require.Equal(t, "https://one.test/error", challenges[0].ErrorURI)
			}
		})
	}
}

func TestCompositeCapturePersistsOriginalConnectionAndRewritesAtGateway(t *testing.T) {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "deferred",
		Version: "test",
	}, nil)
	server.AddTool(&gomcp.Tool{
		Name:        "tool",
		InputSchema: map[string]any{"type": "object"},
	}, nil)
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
	var listCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var message struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &message)
		if message.Method == "tools/list" {
			listCalls.Add(1)
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://provider.test/metadata", scope="write"`)
			w.Header().Set("Set-Cookie", "secret=do-not-forward")
			http.Error(w, "private provider error", http.StatusUnauthorized)

			return
		}

		transport.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	gateway := newLocalAgentAuditLogTestGatewayClient(t)
	storage := newMCPProxyTestStorage()
	component := &v1.MCPServer{
		Name:      "ms1deferred",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			UserID: "42",
			Manifest: types.MCPServerManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: upstream.URL},
			},
		},
	}
	require.NoError(t, storage.Create(t.Context(), component))

	tokens := mcp.NewGlobalTokenStore(gateway)
	manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "https://obot.example", 8080,
		mcp.Options{
			MCPRuntimeBackend: mcp.RuntimeBackendKubernetes,
			MCPNamespace:      "mcp",
		}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, storage, storage, storage, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)

	cfg := mcp.ServerConfig{
		Runtime: types.RuntimeVMCP,
		Components: []mcp.ComponentServer{{
			Name:        component.Name,
			DisplayName: "component",
			URL:         upstream.URL,
		}},
	}
	composite, err := mmmcp.New(t.Context(), &mmmcpconfig.Config{}, mmmcp.Options{AuthorizationResponse: compositeAuthorizationResponse(manager)})
	require.NoError(t, err)

	defer composite.Close()
	request := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect-composite/vmcp1backing", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
	request.SetPathValue("mcp_id", "vmcp1backing")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Method", "tools/list")
	request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	request.Header.Set("X-Mmmcp-Authorization-Capture", "forged")
	recorder := httptest.NewRecorder()
	req := api.Context{
		Request:        request,
		ResponseWriter: recorder,
		APIBaseURL:     "https://obot.example/api",
		Storage:        storage,
		GatewayClient:  gateway,
		User: &user.DefaultInfo{
			UID:    "42",
			Groups: []string{types.GroupAuthenticated, types.GroupCompositeMCP},
			Extra: map[string][]string{
				"oauth_connection_id": {"vmcpi1original"},
				"oauth_client_id":     {"downstream-client"},
			},
		},
	}
	ctx := context.WithValue(request.Context(), compositeAPIContextKey{}, req)
	ctx = context.WithValue(ctx, upstreamCaptureConfigKey{}, cfg)
	ctx = mmmcp.ContextWithConfig(ctx, mcp.MMMCPConfig(cfg, nil))
	composite.HTTPHandler().ServeHTTP(recorder, request.WithContext(ctx))

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())

	challenges := mmmcpcomponent.AuthorizationChallenges(recorder.Result())

	require.Len(t, challenges, 1)
	require.Equal(t, "insufficient_scope", challenges[0].ErrorCode)

	scope := challenges[0].Scope

	require.True(t, strings.HasPrefix(scope, client.UpstreamOAuthScopePrefix))

	batch, err := gateway.ClaimMCPOAuthChallenge(t.Context(), strings.TrimPrefix(scope, client.UpstreamOAuthScopePrefix), "42", system.DefaultNamespace, "vmcpi1original", "downstream-client", "https://obot.example/mcp-connect/vmcpi1original", "oar1test")
	require.NoError(t, err)
	require.Len(t, batch.Requirements, 1)
	require.Equal(t, component.Name, batch.Requirements[0].ConnectID)
	require.Equal(t, http.StatusUnauthorized, batch.Requirements[0].StatusCode)
	require.Equal(t, "write", batch.Requirements[0].Scope)
	require.Equal(t, "https://provider.test/metadata", batch.Requirements[0].ResourceMetadata)
	require.Equal(t, int32(1), listCalls.Load())

	response := recorder.Result()
	outerRequest := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect/vmcpi1original", nil)
	outerRequest.SetPathValue("mcp_id", "vmcpi1original")
	outer := req
	outer.Request = outerRequest
	outer.User = &user.DefaultInfo{
		UID:    "42",
		Groups: []string{types.GroupAuthenticated},
	}
	require.NoError(t, rewriteUpstreamOAuthResponse(outer, response))
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.Equal(t, "insufficient_scope", mmmcpcomponent.AuthorizationChallenges(response)[0].ErrorCode)
	require.Equal(t, downstreamChallenge(outer, scope), response.Header.Get("WWW-Authenticate"))
	require.Empty(t, response.Header.Get("Set-Cookie"))

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "private provider error")

	// Component identity comes from its connection URL, even after a display-name change.
	cfg.Components[0].DisplayName = "renamed"
	challenge := &mmmcpcomponent.AuthorizationError{
		Component:        "old display name",
		URL:              upstream.URL,
		StatusCode:       http.StatusUnauthorized,
		ResourceMetadata: "https://provider.test/metadata",
	}
	var repeated []*mmmcpcomponent.AuthorizationError
	for range 9 {
		repeated = append(repeated, challenge)
	}

	_, err = upstreamOAuthResponse(req, cfg, manager, repeated)
	require.NoError(t, err)

	t.Run("alternatives remain independent across components", func(t *testing.T) {
		second := component.DeepCopy()
		second.Name = "ms1second"
		second.ResourceVersion = ""
		second.UID = ""
		second.Spec.Manifest.RemoteConfig.URL = "https://second.test/mcp"
		require.NoError(t, storage.Create(t.Context(), second))

		multi := cfg
		multi.Components = append(slices.Clone(cfg.Components), mcp.ComponentServer{
			Name: second.Name,
			URL:  second.Spec.Manifest.RemoteConfig.URL,
		})

		var selected []*mmmcpcomponent.AuthorizationError
		for _, target := range multi.Components {
			response := &http.Response{
				StatusCode: http.StatusUnauthorized,
				Header: http.Header{"Www-Authenticate": {
					`Basic realm="legacy", Bearer resource_metadata="https://first.test/meta", scope="read"`,
					`Bearer resource_metadata="https://alternative.test/meta", scope="write"`,
				}},
			}
			require.NoError(t, rewriteUpstreamOAuthResponse(req, response))

			challenges := mmmcpcomponent.AuthorizationChallenges(response)
			require.Len(t, challenges, 1)
			challenges[0].URL = target.URL
			selected = append(selected, challenges[0])
		}

		response, err := upstreamOAuthResponse(req, multi, manager, selected)
		require.NoError(t, err)

		challenges := mmmcpcomponent.AuthorizationChallenges(&http.Response{
			StatusCode: response.status,
			Header:     response.headers,
		})
		require.Len(t, challenges, 1)

		batch, err := gateway.ClaimMCPOAuthChallenge(t.Context(), strings.TrimPrefix(challenges[0].Scope, client.UpstreamOAuthScopePrefix), "42", system.DefaultNamespace, "vmcpi1original", "downstream-client", "https://obot.example/mcp-connect/vmcpi1original", "oar1alternatives")
		require.NoError(t, err)
		require.Len(t, batch.Requirements, 2)
		for i, requirement := range batch.Requirements {
			require.Equal(t, multi.Components[i].Name, requirement.ConnectID)
			require.Equal(t, "https://first.test/meta", requirement.ResourceMetadata)
			require.Equal(t, "read", requirement.Scope)
		}
	})

	challenge.URL = "https://unrelated.test/mcp"
	_, err = upstreamOAuthResponse(req, cfg, manager, []*mmmcpcomponent.AuthorizationError{challenge})
	require.ErrorContains(t, err, "does not belong to this connection")
}

func TestCompositeLoopbackChallengeWithoutTicketRemainsUnauthorized(t *testing.T) {
	req := api.Context{
		Request:    httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect/vmcpi1test", nil),
		APIBaseURL: "https://obot.example/api",
		User:       &user.DefaultInfo{UID: "42"},
	}
	req.SetPathValue("mcp_id", "vmcpi1test")

	response := &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header: http.Header{
			"Www-Authenticate": {`Bearer resource_metadata="http://internal/metadata"`},
			"Set-Cookie":       {"private=secret"},
		},
		Body: io.NopCloser(strings.NewReader("internal failure")),
	}
	require.NoError(t, rewriteUpstreamOAuthResponse(req, response))
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Equal(t, mcpAuthChallenge(req), response.Header.Get("WWW-Authenticate"))
	require.Empty(t, response.Header.Get("Set-Cookie"))
}

func TestCompositeOAuthRemovalInvalidatesDiscovery(t *testing.T) {
	gateway := newLocalAgentAuditLogTestGatewayClient(t)
	saveToken := func() {
		t.Helper()
		require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), "42", "ms1remote", "https://provider.test/mcp", "grant1", &oauth2.Config{}, &oauth2.Token{AccessToken: "token"}))
	}
	saveToken()

	server := gomcp.NewServer(&gomcp.Implementation{Name: "protected", Version: "test"}, nil)
	server.AddTool(&gomcp.Tool{Name: "protected", InputSchema: map[string]any{"type": "object"}}, nil)
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
	var listCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var message struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &message); err == nil && message.Method == "tools/list" {
			listCalls.Add(1)
			if _, err := gateway.GetMCPOAuthToken(r.Context(), "42", "ms1remote", "https://provider.test/mcp"); err != nil {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://provider.test/metadata"`)
				http.Error(w, "authorization required", http.StatusUnauthorized)
				return
			}
		}

		transport.ServeHTTP(w, r)
	}))
	defer upstream.Close()

	composite, err := mmmcp.New(t.Context(), &mmmcpconfig.Config{}, mmmcp.Options{})
	require.NoError(t, err)
	defer composite.Close()
	serverConfig := mcp.ServerConfig{
		Runtime: types.RuntimeVMCP,
		Components: []mcp.ComponentServer{{
			Name:        "ms1remote",
			DisplayName: "remote",
			URL:         upstream.URL,
		}},
	}
	list := func() *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "http://obot.test/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Mcp-Method", "tools/list")
		request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
		cfg, err := compositeConfig(api.Context{
			Request:       request,
			GatewayClient: gateway,
			User:          &user.DefaultInfo{UID: "42"},
		}, serverConfig)
		require.NoError(t, err)

		recorder := httptest.NewRecorder()
		composite.HTTPHandler().ServeHTTP(recorder, request.WithContext(mmmcp.ContextWithConfig(request.Context(), cfg)))
		return recorder
	}

	require.Equal(t, http.StatusOK, list().Code)
	require.Equal(t, http.StatusOK, list().Code)
	require.Equal(t, int32(1), listCalls.Load(), "unchanged grants should reuse discovery")

	require.NoError(t, gateway.DeleteMCPOAuthTokens(t.Context(), "42", "ms1remote"))
	response := list()
	require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
	require.Equal(t, int32(2), listCalls.Load(), "removed credentials must cause rediscovery")

	saveToken()
	require.Equal(t, http.StatusOK, list().Code)
}
