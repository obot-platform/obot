package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	mmmcpcomponent "github.com/obot-platform/mmmcp/component"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newHeadlessTestGatewayClient returns a gateway client over an in-memory database, and the database, so that a test
// can count the OAuth state rows it holds.
func newHeadlessTestGatewayClient(t *testing.T) (*gatewayclient.Client, *gorm.DB) {
	t.Helper()
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	c := gatewayclient.New(t.Context(), db, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, 90, true)
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c, storageServices.DB.DB
}

// requireNoOAuthState requires that the database holds no MCP OAuth challenge or pending authorization.
func requireNoOAuthState(t *testing.T, db *gorm.DB) {
	t.Helper()
	var challenges, pending int64
	require.NoError(t, db.Model(&gatewaytypes.MCPOAuthChallenge{}).Count(&challenges).Error)
	require.NoError(t, db.Model(&gatewaytypes.MCPOAuthPendingState{}).Count(&pending).Error)
	require.Zero(t, challenges, "an MCP OAuth challenge was retained")
	require.Zero(t, pending, "an MCP OAuth authorization was started")
}

// agentLoopbackCaller returns the principal of the composite loopback token issued for agent 7's connection.
func agentLoopbackCaller(connectionID string) user.Info {
	return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, map[string][]string{
		"oauth_connection_id": {connectionID},
		"oauth_client_id":     {"downstream-client"},
	})
}

// agentKeyCaller returns the principal of agent 7's API key.
func agentKeyCaller() user.Info {
	return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, []string{types.GroupMCP, types.GroupAuthenticated}, map[string][]string{
		"authorized_mcp_ids": {"*"},
	})
}

func readAgentFailure(t *testing.T, body io.Reader) vmcpaccess.AgentReadinessFailure {
	t.Helper()
	var failure vmcpaccess.AgentReadinessFailure
	require.NoError(t, json.NewDecoder(body).Decode(&failure))
	return failure
}

func unsignedToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("not the server's key"))
	require.NoError(t, err)
	return token
}

// TestHeadlessCallersAreNeverChallenged checks every source of the gateway's MCP authorization challenge: an agent,
// a composite loopback acting for one, and a request that presents an agent's credential that was refused all get a
// plain 401, while users keep their challenge.
func TestHeadlessCallersAreNeverChallenged(t *testing.T) {
	anonymous := &user.DefaultInfo{Name: "anonymous", UID: "anonymous"}
	for _, tt := range []struct {
		name          string
		caller        user.Info
		authorization string
		apiKeyHeader  string
		wantChallenge bool
	}{
		{
			name:          "anonymous user",
			caller:        anonymous,
			wantChallenge: true,
		},
		{
			name:          "refused user key",
			caller:        anonymous,
			authorization: "Bearer ok1-7-1-secret",
			wantChallenge: true,
		},
		{
			name: "user",
			caller: &user.DefaultInfo{UID: "7", Groups: []string{types.GroupAuthenticated}, Extra: map[string][]string{
				principal.KindExtra: {string(principal.KindUser)},
			}},
			wantChallenge: true,
		},
		{
			// A user's loopback token that failed to authenticate still sends the user into login.
			name:          "refused user loopback token",
			caller:        anonymous,
			authorization: "Bearer " + unsignedToken(t, jwt.MapClaims{"sub": "7", "obot_principal_kind": "user"}),
			wantChallenge: true,
		},
		{
			name:   "agent",
			caller: agentKeyCaller(),
		},
		{
			name:   "agent composite loopback",
			caller: agentLoopbackCaller("vmcpi1agent"),
		},
		{
			name:          "refused agent key",
			caller:        anonymous,
			authorization: "Bearer oak1-7-1-secret",
		},
		{
			name:         "refused agent key in X-API-Key",
			caller:       anonymous,
			apiKeyHeader: "oak1-7-1-secret",
		},
		{
			name:          "refused agent loopback token",
			caller:        anonymous,
			authorization: "Bearer " + unsignedToken(t, jwt.MapClaims{"sub": "agent:7", "obot_principal_kind": "agent"}),
		},
		{
			name:          "refused loopback token with an agent subject",
			caller:        anonymous,
			authorization: "Bearer " + unsignedToken(t, jwt.MapClaims{"sub": "agent:7"}),
		},
		{
			// The no-auth agent token guard refuses this token as an agent's (persistent.ClaimsAgentToken), so it is
			// not sent into a login flow either.
			name:          "refused loopback token with a malformed agent subject",
			caller:        anonymous,
			authorization: "Bearer " + unsignedToken(t, jwt.MapClaims{"sub": "agent:not-a-number"}),
		},
		{
			// A token with which an agent acts on behalf of a user has the user as its subject, but it is an agent's
			// credential, which no authenticator accepts yet: it is not sent into the user's login flow either.
			name:   "refused token acting on behalf of a user",
			caller: anonymous,
			authorization: "Bearer " + unsignedToken(t, jwt.MapClaims{
				"sub":                 "7",
				"obot_principal_kind": "user",
				"act":                 map[string]any{"sub": "agent:7"},
				"obot_delegation_id":  "adl1test",
			}),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect/vmcp1test", nil)
			request.SetPathValue("mcp_id", "vmcp1test")
			if tt.authorization != "" {
				request.Header.Set("Authorization", tt.authorization)
			}
			if tt.apiKeyHeader != "" {
				request.Header.Set("X-API-Key", tt.apiKeyHeader)
			}
			req := api.Context{Request: request, APIBaseURL: "https://obot.example/api", User: tt.caller}

			for _, requiresConfig := range []bool{false, true} {
				response := mcpAuthRequiredResponse(req, requiresConfig)
				require.Equal(t, http.StatusUnauthorized, response.status)
				require.Equal(t, tt.wantChallenge, response.headers.Get("WWW-Authenticate") != "", "challenge header %q", response.headers.Get("WWW-Authenticate"))
			}

			if tt.caller == anonymous {
				// The gateway answers an unauthenticated request before it resolves any server.
				recorder := httptest.NewRecorder()
				req.ResponseWriter = recorder
				require.NoError(t, (&Handler{}).Proxy(req))
				require.Equal(t, http.StatusUnauthorized, recorder.Code)
				require.Equal(t, tt.wantChallenge, recorder.Header().Get("WWW-Authenticate") != "")
			}
		})
	}
}

// TestAgentComponentChallengeBecomesConflict runs a component's OAuth challenge through the composite for an agent's
// composite loopback, and through the outer gateway for the agent itself: the agent gets a 409 that names the
// component, with no challenge header, and no challenge or pending authorization is stored.
func TestAgentComponentChallengeBecomesConflict(t *testing.T) {
	server := gomcp.NewServer(&gomcp.Implementation{Name: "oauth", Version: "test"}, nil)
	server.AddTool(&gomcp.Tool{Name: "tool", InputSchema: map[string]any{"type": "object"}}, nil)
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		if strings.Contains(string(body), `"tools/list"`) {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://provider.test/metadata", scope="write"`)
			http.Error(w, "private provider error", http.StatusUnauthorized)
			return
		}
		transport.ServeHTTP(w, r)
	}))
	defer upstream.Close()

	gateway, db := newHeadlessTestGatewayClient(t)
	storageClient := newMCPProxyTestStorage()
	tokens := mcp.NewGlobalTokenStore(gateway)
	manager, err := mcp.NewSessionManager(t.Context(), false, false, tokens, nil, "https://obot.example", 8080,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, storageClient, storageClient, storageClient, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)

	cfg := mcp.ServerConfig{
		Runtime: types.RuntimeVMCP,
		Components: []mcp.ComponentServer{{
			ComponentID: "remote",
			Name:        "ms1remote",
			DisplayName: "remote",
			URL:         upstream.URL,
		}},
	}
	composite, err := mmmcp.New(t.Context(), &mmmcpconfig.Config{}, mmmcp.Options{AuthorizationResponse: compositeAuthorizationResponse(manager)})
	require.NoError(t, err)
	defer composite.Close()

	request := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect-composite/vmcpi1agent", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
	request.SetPathValue("mcp_id", "vmcpi1agent")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Method", "tools/list")
	request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	recorder := httptest.NewRecorder()
	req := api.Context{
		Request:        request,
		ResponseWriter: recorder,
		APIBaseURL:     "https://obot.example/api",
		Storage:        storageClient,
		GatewayClient:  gateway,
		User:           agentLoopbackCaller("vmcpi1agent"),
	}
	ctx := context.WithValue(request.Context(), compositeAPIContextKey{}, req)
	ctx = context.WithValue(ctx, upstreamCaptureConfigKey{}, cfg)
	ctx = mmmcp.ContextWithConfig(ctx, mcp.MMMCPConfig(cfg, nil))
	composite.HTTPHandler().ServeHTTP(recorder, request.WithContext(ctx))

	require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
	require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
	require.NotContains(t, recorder.Body.String(), "private provider error")
	response := recorder.Result()
	failure := readAgentFailure(t, bytes.NewReader(recorder.Body.Bytes()))
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error: vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{
			ComponentID: "remote",
			Code:        types.AgentReadinessOAuthRequired,
		}},
	}, failure)
	requireNoOAuthState(t, db)

	// The outer gateway passes the composite's answer on to the agent unchanged.
	response.Body = io.NopCloser(bytes.NewReader(recorder.Body.Bytes()))
	outerRequest := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect/vmcp1agents", nil)
	outerRequest.SetPathValue("mcp_id", "vmcp1agents")
	outer := req
	outer.Request = outerRequest
	outer.User = agentKeyCaller()
	require.NoError(t, rewriteUpstreamOAuthResponse(outer, response))
	require.Equal(t, http.StatusConflict, response.StatusCode)
	require.Empty(t, response.Header.Values("WWW-Authenticate"))
	requireNoOAuthState(t, db)

	// Tool search resolves calls at the outer gateway, where the same challenge reaches upstreamOAuthResponse with the
	// agent itself as the caller.
	challenge := &mmmcpcomponent.AuthorizationError{
		URL:              upstream.URL,
		StatusCode:       http.StatusForbidden,
		ErrorCode:        "insufficient_scope",
		Scope:            "write",
		ResourceMetadata: "https://provider.test/metadata",
	}
	conflict, err := upstreamOAuthResponse(outer, cfg, manager, []*mmmcpcomponent.AuthorizationError{challenge})
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, conflict.status)
	require.Empty(t, conflict.headers.Values("WWW-Authenticate"))
	require.Equal(t, failure, readAgentFailure(t, bytes.NewReader(conflict.body)))
	requireNoOAuthState(t, db)

	// A challenge for another connection's component is still refused.
	challenge.URL = "https://unrelated.test/mcp"
	_, err = upstreamOAuthResponse(outer, cfg, manager, []*mmmcpcomponent.AuthorizationError{challenge})
	require.ErrorContains(t, err, "does not belong to this connection")
	requireNoOAuthState(t, db)
}

// TestHeadlessUpstreamChallengesAreRewritten checks the gateway's rewrite of a challenge in a proxied response. At a
// component, the composite loopback acting for an agent gets a bare 401 that the composite still reads as the
// component's authorization failure. At the outer gateway, an agent gets a plain 401 for a refused loopback, and a
// 409 for a scope challenge; neither carries a challenge.
func TestHeadlessUpstreamChallengesAreRewritten(t *testing.T) {
	for _, tt := range []struct {
		name        string
		caller      user.Info
		status      int
		header      string
		wantStatus  int
		wantAuthErr bool
	}{
		{
			name:        "component challenge for an agent's loopback",
			caller:      agentLoopbackCaller("vmcpi1agent"),
			status:      http.StatusUnauthorized,
			header:      `Bearer resource_metadata="https://provider.test/metadata", scope="write"`,
			wantStatus:  http.StatusUnauthorized,
			wantAuthErr: true,
		},
		{
			name:        "component scope challenge for an agent's loopback",
			caller:      agentLoopbackCaller("vmcpi1agent"),
			status:      http.StatusForbidden,
			header:      `Bearer error="insufficient_scope", scope="admin"`,
			wantStatus:  http.StatusUnauthorized,
			wantAuthErr: true,
		},
		{
			name:       "refused loopback at the outer gateway",
			caller:     agentKeyCaller(),
			status:     http.StatusUnauthorized,
			header:     `Bearer realm="Obot MCP Gateway"`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "scope challenge at the outer gateway",
			caller:     agentKeyCaller(),
			status:     http.StatusForbidden,
			header:     `Bearer error="insufficient_scope", scope="obot:upstream:abc"`,
			wantStatus: http.StatusConflict,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect/vmcpi1agent", nil)
			request.SetPathValue("mcp_id", "vmcpi1agent")
			req := api.Context{Request: request, APIBaseURL: "https://obot.example/api", User: tt.caller}
			response := &http.Response{
				StatusCode: tt.status,
				Header:     http.Header{"Www-Authenticate": {tt.header}, "Set-Cookie": {"secret=1"}},
				Body:       io.NopCloser(strings.NewReader("private provider error")),
			}
			require.NoError(t, rewriteUpstreamOAuthResponse(req, response))
			require.Equal(t, tt.wantStatus, response.StatusCode)
			require.Empty(t, response.Header.Values("WWW-Authenticate"))
			require.Empty(t, response.Header.Values("Set-Cookie"))
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NotContains(t, string(body), "private provider error")
			if tt.wantAuthErr {
				require.Len(t, mmmcpcomponent.AuthorizationChallenges(response), 1, "the composite no longer reads the component's authorization failure")
			}
		})
	}
}

// newAgentReadinessTestStorage returns storage indexed as the vMCP connect path reads it.
func newAgentReadinessTestStorage(objects ...kclient.Object) storage.Client {
	return storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		WithStatusSubresource(&v1.MCPClientSession{}).
		WithObjects(objects...).
		Build())
}

// TestAgentNotReadyIsAConflict connects an agent to vMCPs it is granted but cannot use yet: the gateway answers 409
// with the reasons and no challenge, before any MCP exchange.
func TestAgentNotReadyIsAConflict(t *testing.T) {
	staticOAuth := types.VMCPComponent{
		ID:   "salesforce",
		Name: "Salesforce",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://salesforce.example.com/mcp", StaticOAuthRequired: true},
		}},
	}
	for _, tt := range []struct {
		name        string
		profiles    []types.VMCPProfile
		wantFailure vmcpaccess.AgentReadinessFailure
	}{
		{
			name: "component that needs OAuth",
			profiles: []types.VMCPProfile{{
				ID:          "vp1agent",
				Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			}},
			wantFailure: vmcpaccess.AgentReadinessFailure{
				Error: vmcpaccess.AgentOAuthRequiredError,
				Reasons: []vmcpaccess.ReadinessReason{{
					ComponentID: "salesforce",
					Code:        types.AgentReadinessOAuthRequired,
				}},
			},
		},
		{
			// The profile for every user grants the agent nothing; the reason names only the component, whose grant the
			// vMCP holds.
			name: "component that needs OAuth, beside a profile for every user",
			profiles: []types.VMCPProfile{
				{
					ID:          "vp1agent",
					Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				},
				{
					ID:          "vp1everyone",
					Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				},
			},
			wantFailure: vmcpaccess.AgentReadinessFailure{
				Error: vmcpaccess.AgentOAuthRequiredError,
				Reasons: []vmcpaccess.ReadinessReason{{
					ComponentID: "salesforce",
					Code:        types.AgentReadinessOAuthRequired,
				}},
			},
		},
		{
			// An agent named by two profiles is granted what either grants, so the component that only the second
			// grants it keeps it out, once.
			name: "component that needs OAuth, granted by the second of two profiles",
			profiles: []types.VMCPProfile{
				{
					ID:          "vp1agent",
					Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
					Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{}},
				},
				{
					ID:          "vp1second",
					Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				},
			},
			wantFailure: vmcpaccess.AgentReadinessFailure{
				Error: vmcpaccess.AgentOAuthRequiredError,
				Reasons: []vmcpaccess.ReadinessReason{{
					ComponentID: "salesforce",
					Code:        types.AgentReadinessOAuthRequired,
				}},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := agentKeyCaller()
			vmcp := &v1.VMCP{
				Name:      "vmcp1agents",
				Namespace: system.DefaultNamespace,
				Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
					DisplayName: "Agents",
					Components:  []types.VMCPComponent{staticOAuth},
					Profiles:    tt.profiles,
				}},
			}
			instance := &v1.VMCPInstance{
				Name:      "vmcpi1agent",
				Namespace: system.DefaultNamespace,
				Spec: v1.VMCPInstanceSpec{
					UserID:   "agent:7",
					Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
				},
			}
			// The instance controller has recorded its configuration status.
			instance.Status.ConfigurationCheckHash = vmcpaccess.ConfigurationCheckHash(vmcpaccess.EnabledComponents(caller, *vmcp, vmcpaccess.ComponentsForInstance(*vmcp, *instance)), "")
			storageClient := newAgentReadinessTestStorage(vmcp, instance)
			gateway, db := newHeadlessTestGatewayClient(t)
			manager, err := mcp.NewSessionManager(t.Context(), false, false, mcp.NewGlobalTokenStore(gateway), nil, "https://obot.example", 8080,
				mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
				&rest.Config{Host: "http://kubernetes.invalid"}, storageClient, storageClient, storageClient, gateway, system.DefaultNamespace, nil)
			require.NoError(t, err)
			manager.SetPreconfiguredOAuth(vmcpaccess.NewPreconfiguredOAuthStore(gateway))

			request := httptest.NewRequest(http.MethodPost, "https://obot.example/mcp-connect/"+vmcp.Name, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
			request.SetPathValue("mcp_id", vmcp.Name)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler := &Handler{ctx: t.Context(), mcpSessionManager: manager, auditLogCollector: new(recordingProxyAuditCollector)}
			require.NoError(t, handler.Proxy(api.Context{
				Request:        request,
				ResponseWriter: recorder,
				Storage:        storageClient,
				GatewayClient:  gateway,
				APIBaseURL:     "https://obot.example/api",
				User:           caller,
			}))

			require.Equal(t, http.StatusConflict, recorder.Code, recorder.Body.String())
			require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
			require.Empty(t, recorder.Header().Values("WWW-Authenticate"))
			require.Equal(t, tt.wantFailure, readAgentFailure(t, recorder.Body))
			requireNoOAuthState(t, db)

			var sessions v1.MCPClientSessionList
			require.NoError(t, storageClient.List(t.Context(), &sessions))
			require.Empty(t, sessions.Items, "a refused agent was given an MCP session")
		})
	}
}
