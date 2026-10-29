package mcpgateway

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/attribution"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

type headerIdentityReader struct {
	identities []gatewaytypes.Identity
	err        error
	calls      int
	userID     uint
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

func TestProxyStripsAllReservedAttributionHeaders(t *testing.T) {
	for _, selected := range [][]types.ObotHeader{nil, {types.ObotHeaderAPIKeyName}} {
		t.Run(fmt.Sprint(selected), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				for _, name := range allObotHeaders() {
					want := ""
					if name == types.ObotHeaderAPIKeyName && len(selected) > 0 {
						want = "jira-writer"
					}
					require.Equal(t, want, req.Header.Get(string(name)))
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()
			target, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			derived, err := attribution.ForCaller(t.Context(), &headerIdentityReader{}, types.RuntimeRemote, selected, headerTestCaller("42", "jira-writer"))
			require.NoError(t, err)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attribution.Strip(r.Header)
				(&httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
					rewriteProxyRequest(r, target)
					maps.Copy(r.Out.Header, derived)
				}}).ServeHTTP(w, r)
			}))
			defer proxy.Close()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, proxy.URL, nil)
			require.NoError(t, err)
			for _, name := range allObotHeaders() {
				req.Header.Add(string(name), "spoof")
				req.Header.Add(string(name), "another spoof")
			}
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
		})
	}
}

func TestCompositeTokensPreserveKeyAttribution(t *testing.T) {
	for _, name := range []string{"jira-writer", ""} {
		t.Run(name, func(t *testing.T) {
			caller := headerTestCaller("42", name)
			claims := compositeLoopbackTokenContext(caller, "https://obot.example/mcp-connect-composite/vmcp", "vmcp", []string{"component"}, time.Now())
			token, _, err := newCompositeLoopbackToken(t.Context(), fakeCompositeTokens{}, claims, time.Now())
			require.NoError(t, err)
			decoded, err := (fakeCompositeTokens{}).DecodeToken(t.Context(), token)
			require.NoError(t, err)
			require.Equal(t, "42", decoded.APIKeyID)
			require.Equal(t, name, decoded.APIKeyRawName)
			require.Equal(t, name, decoded.APIKeyName)
		})
	}
}

// A single stateful frontend can switch credentials between calls. Its pooled
// component connections must not retain stale key or user attribution.
func TestVMCPAttributionChangesWithCallerOnStatefulSession(t *testing.T) {
	tokens := fakeCompositeTokens{}
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
		caller := headerTestCaller(claims.APIKeyID, claims.APIKeyRawName)
		caller.UID = claims.UserID
		caller.Name = claims.UserName
		caller.Extra["email"] = []string{claims.UserEmail}
		caller.Extra["auth_provider_name"] = []string{claims.AuthProviderName}
		caller.Extra["auth_provider_namespace"] = []string{claims.AuthProviderNamespace}
		// The real token authenticator reloads memberships from the database for each request.
		active := callers[current.Load()]
		caller.Extra["auth_provider_groups"] = active.Extra["auth_provider_groups"]
		reader := &headerIdentityReader{identities: []gatewaytypes.Identity{{ProviderUserID: firstExtra(active.Extra, "test_idp_user_id")}}}
		derived, err := attribution.ForCaller(r.Context(), reader, types.RuntimeRemote, allObotHeaders(), caller)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		attribution.Strip(r.Header)
		maps.Copy(r.Header, derived)
		backendHandler.ServeHTTP(w, r)
	}))
	defer componentGateway.Close()
	composite, err := mmmcp.New(t.Context(), &mmmcpconfig.Config{}, mmmcp.Options{
		OAuth:      compositeComponentAuth{ctx: t.Context(), tokens: tokens},
		HTTPClient: newCompositeHTTPClient(),
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
	renamed := headerTestCaller("42", "first-writer")
	renamed.Name = "alice-renamed"
	changedEmail := headerTestCaller("42", "first-writer")
	changedEmail.Extra["email"] = []string{"alice-new@example.com"}
	ordinaryRenamed := headerTestCaller("", "")
	ordinaryRenamed.Name = "alice-renamed"
	ordinaryChangedEmail := headerTestCaller("", "")
	ordinaryChangedEmail.Extra["email"] = []string{"alice-new@example.com"}
	otherUser := headerTestCaller("", "")
	otherUser.UID = "8"
	changedIDP := headerTestCaller("42", "first-writer")
	changedIDP.Extra["test_idp_user_id"] = []string{"00u-new-identity"}
	changedGroups := headerTestCaller("42", "first-writer")
	changedGroups.Extra["auth_provider_groups"] = []string{"okta:engineering", "okta:admins"}
	callers = append(callers, renamed, changedEmail, ordinaryRenamed, ordinaryChangedEmail, otherUser, changedIDP, changedGroups)
	frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller := callers[current.Load()]
		claims := compositeLoopbackTokenContext(caller, "https://obot.example/vmcp", "vmcp", []string{"component"}, time.Now())
		token, _, err := newCompositeLoopbackToken(r.Context(), tokens, claims, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		r.Header.Set("Authorization", "Bearer "+token)
		ctx := mmmcp.ContextWithConfig(r.Context(), compositeConfig(serverConfig, caller))
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
	for _, i := range []int{0, 5, 6, 1, 2, 7, 8, 9, 3, 4, 10, 11, 0} {
		current.Store(int32(i))
		result, err := session.CallTool(t.Context(), &gomcp.CallToolParams{Name: "actor"})
		require.NoError(t, err)
		require.False(t, result.IsError)
		require.Len(t, result.Content, 1)
		var got http.Header
		require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*gomcp.TextContent).Text), &got))
		want, err := attribution.ForCaller(t.Context(), &headerIdentityReader{identities: []gatewaytypes.Identity{{ProviderUserID: firstExtra(callers[i].Extra, "test_idp_user_id")}}}, types.RuntimeRemote, allObotHeaders(), callers[i])
		require.NoError(t, err)
		require.Equal(t, want, got, "attribution for caller %d must reflect the current request", i)
	}
}

func (r *headerIdentityReader) FindIdentitiesForUser(_ context.Context, userID uint) ([]gatewaytypes.Identity, error) {
	r.calls++
	r.userID = userID
	return r.identities, r.err
}

func TestCompositeLoopbackPreservesIdentityProvider(t *testing.T) {
	caller := headerTestCaller("42", "writer")
	caller.Extra["auth_provider_name"] = []string{"okta"}
	caller.Extra["auth_provider_namespace"] = []string{"default"}
	claims := compositeLoopbackTokenContext(caller, "https://obot.example", "component", nil, time.Now())
	require.Equal(t, "okta", claims.AuthProviderName)
	require.Equal(t, "default", claims.AuthProviderNamespace)
}
