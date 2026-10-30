package mcpgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/attribution"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

type headerIdentityReader struct {
	identities []gatewaytypes.Identity
}

func headerTestCaller(id, name string) *user.DefaultInfo {
	return &user.DefaultInfo{
		UID:  "7",
		Name: "alice",
		Extra: map[string][]string{
			"email":                      {"alice@example.com"},
			principal.APIKeyIDExtra:      {id},
			principal.APIKeyNameExtra:    {name},
			principal.APIKeyRawNameExtra: {name},
		},
	}
}

func allObotHeaders() []types.ObotHeader {
	return types.ObotHeaders()
}

func TestVMCPAttributionChangesWithCallerOnStatefulSession(t *testing.T) {
	for _, selected := range [][]types.ObotHeader{nil, {types.ObotHeaderAPIKeyName}, allObotHeaders()} {
		t.Run(fmt.Sprint(selected), func(t *testing.T) {
			testVMCPAttributionPassthrough(t, selected)
		})
	}
}

func testVMCPAttributionPassthrough(t *testing.T, selected []types.ObotHeader) {
	t.Helper()
	tokens := fakeCompositeTokens{}
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	var current atomic.Int32
	var callers []*user.DefaultInfo
	backend := gomcp.NewServer(&gomcp.Implementation{Name: "component", Version: "test"}, nil)
	backend.AddTool(&gomcp.Tool{Name: "actor", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		headers := make(http.Header)
		for _, name := range allObotHeaders() {
			if value := req.GetExtra().Header.Get(string(name)); value != "" {
				headers.Set(string(name), value)
			}
		}
		body, err := json.Marshal(headers)
		if err != nil {
			return nil, err
		}
		return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: string(body)}}}, nil
	})
	backendHandler := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return backend }, nil)
	componentGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		claims, err := tokens.DecodeToken(r.Context(), bearer)
		if err != nil {
			http.Error(w, "invalid internal token", http.StatusUnauthorized)
			return
		}
		caller := &user.DefaultInfo{
			Groups: claims.UserGroups,
			Extra: map[string][]string{
				"authorized_mcp_ids": claims.AuthorizedMCPIDs,
			},
		}
		require.Equal(t, "42", claims.APIKeyID, "connection authentication keeps the opening caller's token")
		if !authz.IsCompositeMCPRequest(caller, "component") {
			attribution.Strip(r.Header)
		}
		derived := attribution.Selected(r.Header, types.RuntimeRemote, selected)
		attribution.Strip(r.Header)
		maps.Copy(r.Header, derived)
		backendHandler.ServeHTTP(w, r)
	}))
	defer componentGateway.Close()
	composite, err := mmmcp.New(t.Context(), &mmmcpconfig.Config{}, mmmcp.Options{
		OAuth: compositeComponentAuth{ctx: t.Context(), tokens: tokens},
	})
	require.NoError(t, err)
	defer composite.Close()
	serverConfig := obotmcp.ServerConfig{
		Runtime:       types.RuntimeVMCP,
		MCPServerName: "vmcp",
		Components: []obotmcp.ComponentServer{
			{
				DisplayName: "component",
				URL:         componentGateway.URL,
			},
		},
	}
	callers = []*user.DefaultInfo{
		headerTestCaller("42", "first-writer"),
		headerTestCaller("43", "second-writer"),
		headerTestCaller("", ""),
		headerTestCaller("42", "renamed-writer"),
		headerTestCaller("42", ""),
	}
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller := callers[current.Load()]
		claims := compositeLoopbackTokenContext(caller, "https://obot.example/vmcp", "vmcp", []string{"component"}, "vmcpi1connection", time.Now())
		token, _, err := newCompositeLoopbackToken(r.Context(), tokens, claims, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		attribution.Strip(r.Header)
		derived, err := attribution.ForCaller(r.Context(), &headerIdentityReader{identities: []gatewaytypes.Identity{{ProviderUserID: firstExtra(caller.Extra, "test_idp_user_id")}}}, allObotHeaders(), caller)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		maps.Copy(r.Header, derived)
		r.Header.Set("Authorization", "Bearer "+token)
		cfg, err := compositeConfig(api.Context{Request: r, GatewayClient: gatewayClient, User: caller}, serverConfig)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ctx := mmmcp.ContextWithConfig(r.Context(), cfg)
		composite.HTTPHandler().ServeHTTP(w, r.WithContext(ctx))
	}))
	defer frontend.Close()
	session, err := gomcp.NewClient(&gomcp.Implementation{Name: "client", Version: "test"}, nil).Connect(t.Context(), &gomcp.StreamableClientTransport{
		Endpoint:             frontend.URL,
		DisableStandaloneSSE: true,
	}, &gomcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	require.NoError(t, err)
	require.NotEmpty(t, session.ID(), "exercise a stateful connection rather than the SDK default stateless protocol")
	defer session.Close()
	for _, i := range []int{0, 1, 2, 3, 4, 0} {
		current.Store(int32(i))
		result, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: "actor"})
		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Len(t, result.Content, 1)
		var got http.Header
		require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*gomcp.TextContent).Text), &got))
		want, err := attribution.ForCaller(t.Context(), &headerIdentityReader{identities: []gatewaytypes.Identity{{ProviderUserID: firstExtra(callers[i].Extra, "test_idp_user_id")}}}, selected, callers[i])
		require.NoError(t, err)
		require.Equal(t, want, got, "attribution for caller %d must reflect the current request", i)
	}
}

func (r *headerIdentityReader) FindIdentitiesForUser(_ context.Context, _ uint) ([]gatewaytypes.Identity, error) {
	return r.identities, nil
}

func TestCompositeLoopbackPreservesAttribution(t *testing.T) {
	caller := headerTestCaller("42", "writer")
	caller.Extra["auth_provider_name"] = []string{"okta"}
	caller.Extra["auth_provider_namespace"] = []string{"default"}
	claims := compositeLoopbackTokenContext(caller, "https://obot.example", "component", nil, "vmcpi1connection", time.Now())
	require.Equal(t, "42", claims.APIKeyID)
	require.Equal(t, "writer", claims.APIKeyName)
	require.Equal(t, "writer", claims.APIKeyRawName)
	require.Equal(t, "okta", claims.AuthProviderName)
	require.Equal(t, "default", claims.AuthProviderNamespace)
}
