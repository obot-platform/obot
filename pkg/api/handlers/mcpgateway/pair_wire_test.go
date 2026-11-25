package mcpgateway

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpserverinstance"
	"github.com/obot-platform/obot/pkg/controller/handlers/vmcpinstance"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewayserver "github.com/obot-platform/obot/pkg/gateway/server"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/attribution"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// pairCaller is a caller of the vMCP in the pair fixture, and what the upstream must see on its requests.
type pairCaller struct {
	name  string
	outer user.Info
	// token is the X-Token value its requests carry, and bearer the access token, or none.
	token, bearer string
}

// pairWire is a shared vMCP whose one component is a shared remote server that takes an optional X-Token header from
// its users, reached by agent 7 for itself, by the agent on behalf of users 9 and 10, and by user 9 directly, over
// the whole path a request takes, as the wire fixture is: the outer gateway, the composite loopback and the component
// gateway, each over HTTP. Unlike the wire fixture, tokens are Obot's own (persistent.TokenService), and every hop
// authenticates them as the server does: an agent acting on behalf of a user with the authenticator of delegated
// tokens, which checks the delegation, the agent, its key and the user on every request.
type pairWire struct {
	upstream     *wireUpstream
	storage      storage.Client
	gateway      *gatewayclient.Client
	db           *gorm.DB
	tokens       *persistent.TokenService
	manager      *mcp.SessionManager
	agent        *gatewaytypes.Agent
	key          *gatewaytypes.APIKeyCreateResponse
	compositeURL string
	grants       *vmcpaccess.PreconfiguredOAuthStore
}

// pairWireOptions shape the fixture: whether the vMCP holds preconfigured OAuth for the component, and whether the
// instances, connections, values and accounts of the callers are set up already, as the controllers and the setup API
// leave them.
type pairWireOptions struct {
	grant  bool
	preset bool
	// strict makes the upstream refuse every request without an account it accepts, even the agent's own, which has
	// none without the vMCP's grant.
	strict bool
}

func newPairWire(t *testing.T, options pairWireOptions) (*pairWire, map[string]*pairCaller) {
	t.Helper()
	ctx := t.Context()
	bearers := []string{"vmcp-grant", "pair-nine", "pair-ten", "user-nine-own"}
	if !options.grant && !options.strict {
		// The agent's own connection has no account to send without the vMCP's grant.
		bearers = append(bearers, "")
	}
	upstream := newWireUpstream(t, bearers...)

	gateway, db := newHeadlessTestGatewayClient(t)
	_, jwk, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	require.NoError(t, gateway.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: system.JWKCredentialContext,
		Name:    system.JWKCredentialContext,
		Secrets: map[string]string{"JWK_KEY": base64.StdEncoding.EncodeToString(jwk)},
	}))
	tokens, err := persistent.NewTokenService("https://obot.example", gateway)
	require.NoError(t, err)
	for _, id := range []uint{9, 10} {
		require.NoError(t, db.Create(&gatewaytypes.User{ID: id, Username: fmt.Sprintf("user%d", id), HashedUsername: fmt.Sprintf("hashed-%d", id), Email: fmt.Sprintf("user%d@example.com", id), Role: types.RoleBasic}).Error)
	}
	agent, err := gateway.CreateAgent(ctx, gatewaytypes.Agent{
		DisplayName:        "assistant",
		AllowedReturnHosts: []string{"agent.example.com"},
		Profiles:           []types.AgentProfile{{Name: "everyone", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}}},
	})
	require.NoError(t, err)
	key, err := gateway.CreateAgentAPIKey(ctx, agent.ID, gatewaytypes.AgentAPIKeyCreateRequest{Name: "assistant", MCPServerIDs: []string{"*"}})
	require.NoError(t, err)
	agentOwner := principal.Agent(agent.ID).String()

	component := types.VMCPComponent{
		ID:            wireComponent,
		Name:          "one",
		Configuration: []types.VMCPConfigurationPolicy{{Key: wireHeader, Policy: types.VMCPConfigurationPolicyUserAllowed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{FixedURL: upstream.server.URL},
			Config:       []types.MCPConfig{{Key: wireHeader, Usage: types.Header}},
		}},
	}
	vmcp := &v1.VMCP{Name: wireVMCP, Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		DisplayName: "Wire",
		Components:  []types.VMCPComponent{component},
		Profiles: []types.VMCPProfile{
			{ID: "vp1agent", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: strconv.FormatUint(uint64(agent.ID), 10)}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
			{ID: "vp1users", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
		},
	}}}
	// Named as the vMCP controller names a shared component server, which the VMCPInstance controller connects to.
	shared := &v1.MCPServer{Name: vmcpaccess.ComponentServerName(vmcp.Name, wireComponent), Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{
		VMCPID:          vmcp.Name,
		VMCPComponentID: wireComponent,
		Manifest: types.MCPServerManifest{
			Name:         "one",
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteRuntimeConfig{URL: upstream.server.URL},
			Config:       vmcpaccess.ComponentConfig(component),
		},
	}}

	userExtra := map[string][]string{}
	principal.RecordKind(userExtra, principal.KindUser)
	f := &pairWire{upstream: upstream, gateway: gateway, db: db, tokens: tokens, agent: agent, key: key}
	callers := map[string]*pairCaller{
		"agent": {
			name: "agent",
			outer: gatewayclient.AgentUserInfo(agent, []string{types.GroupMCP, types.GroupAuthenticated}, map[string][]string{
				"authorized_mcp_ids": {"*"},
			}),
		},
		"user 9": {
			name:   "user 9",
			outer:  &user.DefaultInfo{UID: "9", Name: "user9", Groups: []string{types.GroupAuthenticated}, Extra: userExtra},
			token:  "user-nine-value",
			bearer: "user-nine-own",
		},
		"agent for user 9":  {name: "agent for user 9", token: "pair-nine-value", bearer: "pair-nine"},
		"agent for user 10": {name: "agent for user 10", token: "pair-ten-value", bearer: "pair-ten"},
	}
	if options.grant {
		// The vMCP's preconfigured OAuth is used in place of any account, by everyone who connects, a user directly
		// included, and wherever the component runs.
		callers["agent"].bearer = "vmcp-grant"
		callers["user 9"].bearer = "vmcp-grant"
		callers["agent for user 9"].bearer = "vmcp-grant"
		callers["agent for user 10"].bearer = "vmcp-grant"
	}

	objects := []kclient.Object{vmcp, shared}
	type preset struct {
		owner, token string
		bearer       string
	}
	var presets []preset
	if options.preset {
		presets = []preset{
			{owner: agentOwner},
			{owner: "9", token: "user-nine-value", bearer: "user-nine-own"},
			{owner: principal.DelegatedOwner{AgentID: agent.ID, UserID: 9}.String(), token: "pair-nine-value", bearer: "pair-nine"},
			{owner: principal.DelegatedOwner{AgentID: agent.ID, UserID: 10}.String(), token: "pair-ten-value", bearer: "pair-ten"},
		}
	}
	for i, p := range presets {
		instance := &v1.VMCPInstance{
			Name:      "vmcpi1pairwire" + strconv.Itoa(i),
			Namespace: system.DefaultNamespace,
			Spec:      v1.VMCPInstanceSpec{UserID: p.owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
		}
		components := vmcpaccess.ComponentsForInstance(*vmcp, *instance)
		if _, ok := vmcpaccess.PairOwner(*instance); ok {
			components = vmcpaccess.PairInstanceComponents(components)
		} else {
			owner := callers["user 9"].outer
			if p.owner == agentOwner {
				owner = callers["agent"].outer
			}
			components = vmcpaccess.EnabledComponents(owner, *vmcp, components)
		}
		// What the VMCPInstance controller records once it has synchronized the instance.
		instance.Status.ConfigurationCheckHash = vmcpaccess.ConfigurationCheckHash(components, "")
		instance.Status.Configured = true
		config := vmcpaccess.ConnectionConfiguration(component)
		connection := &v1.MCPServerInstance{
			Name:      vmcpaccess.ComponentConnectionName(instance.Name, wireComponent),
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:          p.owner,
				VMCPInstanceID:  instance.Name,
				VMCPComponentID: wireComponent,
				MCPServerName:   shared.Name,
				Config:          config,
			},
		}
		// What the MCPServerInstance controller records once it has copied the connection's configuration.
		connection.Status.VMCPConfigurationHash = vmcpaccess.ConnectionConfigurationHash(config, *instance)
		objects = append(objects, instance, connection)

		secrets := map[string]string{}
		if p.token != "" {
			secrets[wireHeader] = p.token
		}
		require.NoError(t, gateway.UpsertCredential(ctx, gatewaytypes.Credential{Context: p.owner + "-" + connection.Name, Name: connection.Name, Secrets: secrets}))
		if p.bearer != "" {
			// The account each connected for its own connection: the user directly, and each pair through the agent.
			require.NoError(t, gateway.ReplaceMCPOAuthToken(ctx, p.owner, connection.Name, upstream.server.URL, "", &oauth2.Config{ClientID: "client"},
				&oauth2.Token{AccessToken: p.bearer, TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}))
		}
	}
	if options.grant {
		require.NoError(t, gateway.ReplaceMCPOAuthGrant(ctx, principal.PreconfiguredOAuthOwner(vmcp.Name), vmcpaccess.PreconfiguredOAuthMCPID(vmcp.Name, wireComponent), upstream.server.URL, "grant-vmcp", "1",
			&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "vmcp-grant", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}))
	}

	f.storage = &wireInitialEventsStorage{Client: fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.VMCPInstance{}, "metadata.name", func(obj kclient.Object) []string { return []string{obj.GetName()} }).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.LegacySlug} }).
		WithIndex(&v1.VMCP{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCP).Spec.LegacySlug} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "metadata.name", func(obj kclient.Object) []string { return []string{obj.GetName()} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerName", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.MCPServerName} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.UserID} }).
		WithIndex(&v1.UserGroupChange{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.UserGroupChange).Get("spec.userID")} }).
		WithIndex(&v1.UserRoleChange{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.UserRoleChange).Get("spec.userID")} }).
		WithIndex(&v1.AgentIdentityChange{}, "spec.agentID", func(obj kclient.Object) []string { return []string{obj.(*v1.AgentIdentityChange).Get("spec.agentID")} }).
		WithIndex(&v1.AgentIdentityDelete{}, "spec.agentID", func(obj kclient.Object) []string { return []string{obj.(*v1.AgentIdentityDelete).Get("spec.agentID")} }).
		WithStatusSubresource(&v1.MCPClientSession{}, &v1.MCPServer{}, &v1.VMCP{}, &v1.VMCPInstance{}, &v1.MCPServerInstance{}).
		WithObjects(objects...).
		Build()}

	// The component gateway, which the session manager names as each component's local URL.
	authenticator := gatewayserver.NewDelegatedTokenAuthenticator(gateway, tokens)
	authenticate := func(r *http.Request) (user.Info, error) {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if persistent.ClaimsDelegatedToken(bearer) {
			response, ok, err := authenticator.AuthenticateRequest(r)
			if err != nil || !ok {
				return nil, fmt.Errorf("the delegated loopback token is refused: %v", err)
			}
			return response.User, nil
		}
		claims, err := tokens.DecodeToken(r.Context(), bearer)
		if err != nil {
			return nil, err
		}
		return wireLoopbackCaller(claims), nil
	}
	var handler *Handler
	componentGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.URL.Path, "/mcp-connect/")
		caller, err := authenticate(r)
		if !ok || err != nil || !slices.Contains(caller.GetExtra()["authorized_mcp_ids"], id) {
			http.Error(w, "the composite loopback token does not authorize this connection", http.StatusForbidden)
			return
		}
		r.SetPathValue("mcp_id", id)
		if err := handler.Proxy(api.Context{Request: r, ResponseWriter: w, Storage: f.storage, GatewayClient: gateway, APIBaseURL: "https://obot.example/api", User: caller}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	t.Cleanup(componentGateway.Close)
	gatewayURL, err := url.Parse(componentGateway.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(gatewayURL.Port())
	require.NoError(t, err)

	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion, appsv1.SchemeGroupVersion})
	for _, kind := range []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind("Secret"),
		corev1.SchemeGroupVersion.WithKind("Service"),
		corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaim"),
		appsv1.SchemeGroupVersion.WithKind("Deployment"),
	} {
		mapper.Add(kind, meta.RESTScopeNamespace)
	}
	cluster := fake.NewClientBuilder().WithScheme(k8sscheme.Scheme).WithRESTMapper(mapper).Build()
	globalTokens := mcp.NewGlobalTokenStore(gateway)
	manager, err := mcp.NewSessionManager(ctx, false, false, globalTokens, nil, "https://obot.example", port,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, cluster, cluster, f.storage, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	f.grants = vmcpaccess.NewPreconfiguredOAuthStore(gateway)
	manager.SetPreconfiguredOAuth(f.grants)
	f.manager = manager
	handler = &Handler{ctx: ctx, mcpSessionManager: manager, globalTokenStore: globalTokens, auditLogCollector: new(recordingProxyAuditCollector)}

	composite, err := mmmcp.New(ctx, &mmmcpconfig.Config{}, mmmcp.Options{
		ForwardClientInfo:     true,
		OAuth:                 compositeComponentAuth{ctx: ctx, tokens: tokens},
		AuthorizationResponse: compositeAuthorizationResponse(manager),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = composite.Close() })

	compositeLoopback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.URL.Path, "/mcp-connect-composite/")
		caller, err := authenticate(r)
		if !ok || err != nil {
			http.Error(w, "the composite loopback token is refused", http.StatusUnauthorized)
			return
		}
		r.SetPathValue("mcp_id", id)
		req := api.Context{Request: r, ResponseWriter: w, Storage: f.storage, GatewayClient: gateway, APIBaseURL: "https://obot.example/api", User: caller}
		serverConfig, err := manager.ServerConfigForVMCP(r.Context(), id, caller)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		maps.Copy(req.Request.Header, attribution.Configured(nil, serverConfig.PassthroughHeaderNames, serverConfig.PassthroughHeaderValues))
		cfg, err := compositeConfig(req, serverConfig)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cctx := context.WithValue(r.Context(), compositeAPIContextKey{}, req)
		cctx = context.WithValue(cctx, upstreamCaptureConfigKey{}, serverConfig)
		cctx = mmmcp.ContextWithConfig(cctx, cfg)
		cctx = mmmcp.ContextWithConfigID(cctx, compositeConfigID(serverConfig, req.User))
		composite.HTTPHandler().ServeHTTP(w, r.WithContext(cctx))
	}))
	t.Cleanup(compositeLoopback.Close)
	f.compositeURL = compositeLoopback.URL

	if !options.preset {
		f.runControllers(t)
	}
	callers["agent for user 9"].outer = f.onBehalfOf(t, 9, "adl1nine", true)
	callers["agent for user 10"].outer = f.onBehalfOf(t, 10, "adl1ten", true)
	return f, callers
}

// runControllers runs, in the background until the test ends, what the VMCPInstance and MCPServerInstance controllers
// do for every instance and connection in storage: they create an instance's connections, record its configuration
// status, and copy its configuration to its connections, as they do for everyone's instances, a pair's included.
func (f *pairWire) runControllers(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-done
	})
	instances, connections := vmcpinstance.New(f.gateway), mcpserverinstance.New(f.gateway)
	request := func(obj kclient.Object) router.Request {
		return router.Request{Ctx: ctx, Client: f.storage, Object: obj, Namespace: system.DefaultNamespace, Name: obj.GetName()}
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			var list v1.VMCPInstanceList
			if err := f.storage.List(ctx, &list); err != nil {
				continue
			}
			for i := range list.Items {
				for _, handle := range []func(router.Request, router.Response) error{instances.EnsureMCPServerInstances, instances.SyncUserConfigurationHash} {
					var current v1.VMCPInstance
					if f.storage.Get(ctx, kclient.ObjectKeyFromObject(&list.Items[i]), &current) == nil {
						_ = handle(request(&current), nil)
					}
				}
			}
			var connectionList v1.MCPServerInstanceList
			if err := f.storage.List(ctx, &connectionList); err != nil {
				continue
			}
			for i := range connectionList.Items {
				_ = connections.SyncVMCPConfiguration(request(&connectionList.Items[i]), nil)
			}
		}
	}()
}

// onBehalfOf returns the agent acting on behalf of the user with userID, as the authenticator of delegated tokens
// admits it with a token minted from the delegation with delegationID, which create creates first.
func (f *pairWire) onBehalfOf(t *testing.T, userID uint, delegationID string, create bool) user.Info {
	t.Helper()
	if create {
		now := time.Now()
		require.NoError(t, f.db.Create(&gatewaytypes.AgentDelegation{ID: delegationID, AgentID: f.agent.ID, UserID: userID, APIKeyID: f.key.ID, FirstApprovedAt: now, LastApprovedAt: now}).Error)
	}
	token, _, err := f.tokens.NewDelegatedToken(t.Context(), f.agent.ID, userID, delegationID, f.key.ID, 0)
	require.NoError(t, err)
	r := httptest.NewRequest(http.MethodPost, "/mcp-connect/"+wireVMCP, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	response, ok, err := gatewayserver.NewDelegatedTokenAuthenticator(f.gateway, f.tokens).AuthenticateRequest(r)
	require.NoError(t, err)
	require.True(t, ok)
	return response.User
}

// frontend returns the URL at which caller reaches the vMCP through the outer gateway, which resolves its
// configuration, readiness included, and mints its composite loopback token as the gateway does
// (compositeLoopbackToken): from the same delegation, for an agent acting on behalf of a user.
func (f *pairWire) frontend(t *testing.T, caller user.Info) string {
	t.Helper()
	target, err := url.Parse(f.compositeURL)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mcp_id", wireVMCP)
		req := api.Context{Request: r, ResponseWriter: w, Storage: f.storage, GatewayClient: f.gateway, APIBaseURL: "https://obot.example/api", User: caller}
		serverConfig, err := f.manager.ServerConfigForVMCP(r.Context(), wireVMCP, caller)
		if notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err); ok {
			agentReadinessResponse(notReady.Failure()).write(w, nil)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		authorized := make([]string, 0, len(serverConfig.Components))
		for _, component := range serverConfig.Components {
			authorized = append(authorized, component.ConnectID())
		}
		token, err := compositeLoopbackToken(r.Context(), f.tokens, caller, f.compositeURL+"/mcp-connect-composite/"+serverConfig.MCPServerName, serverConfig.MCPServerName, authorized, wireVMCP, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		attribution.Strip(r.Header)
		r.Header.Set("Authorization", "Bearer "+token)
		(&httputil.ReverseProxy{
			Rewrite: func(p *httputil.ProxyRequest) {
				p.SetURL(target)
				p.Out.URL.Path = "/mcp-connect-composite/" + serverConfig.MCPServerName
				p.Out.Host = target.Host
			},
			ModifyResponse: func(resp *http.Response) error {
				return rewriteUpstreamOAuthResponse(req, resp)
			},
		}).ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// connect opens a stateful MCP session of caller with the vMCP.
func (f *pairWire) connect(t *testing.T, caller *pairCaller) (*gomcp.ClientSession, *wireResponses) {
	t.Helper()
	responses := &wireResponses{}
	session, err := gomcp.NewClient(&gomcp.Implementation{Name: "client", Version: "test"}, nil).Connect(t.Context(), &gomcp.StreamableClientTransport{
		Endpoint:             f.frontend(t, caller.outer),
		HTTPClient:           &http.Client{Transport: responses},
		DisableStandaloneSSE: true,
	}, &gomcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		t.Fatalf("%s could not connect: %v: %+v", caller.name, err, responses.responses)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, responses
}

// pairConnection returns the name of the pair's connection to the component, for the agent acting on behalf of the
// user with userID.
func (f *pairWire) pairConnection(t *testing.T, userID uint) (owner, connection string) {
	t.Helper()
	owner = principal.DelegatedOwner{AgentID: f.agent.ID, UserID: userID}.String()
	instance, err := vmcpaccess.FindInstance(t.Context(), f.storage, system.DefaultNamespace, wireVMCP, owner)
	require.NoError(t, err)
	require.NotNil(t, instance, "the pair of the agent and user %d has no instance", userID)
	return owner, vmcpaccess.ComponentConnectionName(instance.Name, wireComponent)
}

// requireOnTheWire calls the upstream's whoami tool as every caller at once, several times each, over each caller's own
// session, and requires that each call reached the upstream with that caller's own header value and access token, and
// that every request on the wire carried exactly one caller's pair of them.
func requireOnTheWire(t *testing.T, f *pairWire, callers map[string]*pairCaller) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	sessions := map[string]*gomcp.ClientSession{}
	for name, caller := range callers {
		sessions[name], _ = f.connect(t, caller)
	}
	var (
		wait sync.WaitGroup
		lock sync.Mutex
		errs []error
	)
	for name, caller := range callers {
		want := ""
		if caller.bearer != "" {
			want = "Bearer " + caller.bearer
		}
		for i := range 5 {
			wait.Go(func() {
				seen, err := whoami(ctx, sessions[name])
				if err == nil && (seen["token"] != caller.token || seen["authorization"] != want) {
					err = fmt.Errorf("call %d reached the upstream with %v", i, seen)
				}
				if err != nil {
					lock.Lock()
					errs = append(errs, fmt.Errorf("%s: %w", name, err))
					lock.Unlock()
				}
			})
		}
	}
	wait.Wait()
	require.Empty(t, errs)

	pairs := map[wireSeen]bool{}
	for _, caller := range callers {
		authorization := ""
		if caller.bearer != "" {
			authorization = "Bearer " + caller.bearer
		}
		pairs[wireSeen{authorization: authorization, token: caller.token}] = false
	}
	for _, seen := range f.upstream.requests() {
		pair := wireSeen{authorization: seen.authorization, token: seen.token}
		_, ok := pairs[pair]
		require.True(t, ok, "the upstream received %q with %q in a %s request", seen.token, seen.authorization, seen.method)
		pairs[pair] = true
	}
	for pair, seen := range pairs {
		require.True(t, seen, "the upstream never received %v", pair)
	}
}

// TestPairSetupOnTheWire runs the agent acting for itself, the agent acting on behalf of users 9 and 10, and user 9
// directly through the whole vMCP path at once, to a shared component that takes a header from its users. The
// upstream must see each one's own header value and account only on its own requests: none for the agent acting for
// itself, the value each user entered through the agent and the account each connected through it for the agent
// acting on their behalf, and user 9's own for user 9. With the vMCP's preconfigured OAuth, every connection, user 9's
// own included, uses the grant in place of an account, and each caller still sends its own header value.
func TestPairSetupOnTheWire(t *testing.T) {
	t.Run("pair accounts", func(t *testing.T) {
		f, callers := newPairWire(t, pairWireOptions{preset: true})
		requireOnTheWire(t, f, callers)
	})
	t.Run("the vMCP's preconfigured OAuth", func(t *testing.T) {
		f, callers := newPairWire(t, pairWireOptions{preset: true, grant: true})
		requireOnTheWire(t, f, callers)
	})
}

// tokenEndpoint is a provider's token endpoint that refuses every refresh with invalid_grant, as one does once a user
// revoked the account.
func refusingTokenEndpoint(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"revoked"}`))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// requireApprovalToolError requires that result is the tool error that tells the agent to ask the user to approve it
// again for the component.
func requireApprovalToolError(t *testing.T, result *gomcp.CallToolResult) {
	t.Helper()
	require.True(t, result.IsError, "%#v", result)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(*gomcp.TextContent)
	require.True(t, ok)
	require.NotContains(t, text.Text, "private provider error")
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentUserApprovalRequiredError,
		VMCPID:  wireVMCP,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: wireComponent, Code: types.AgentReadinessOAuthRequired}},
	}, readAgentFailure(t, strings.NewReader(text.Text)))
}

// TestPairAccountBrokenMidSession checks what the agent acting on behalf of a user gets mid-session when the account the
// user connected through the agent stops working. When the upstream refuses it, the tool call is answered with an MCP
// tool error with agent_user_approval_required that names the vMCP and the component, and the session goes on. When
// the provider refuses to refresh it (invalid_grant), the tool call gets the same tool error without reaching the
// upstream, and the account is removed, so that the user's setup shows it is to connect again; the agent's next request
// needs the user's approval too. Another user's pair, and the user's own account, are untouched, and no challenge is
// stored for anyone.
func TestPairAccountBrokenMidSession(t *testing.T) {
	f, callers := newPairWire(t, pairWireOptions{preset: true, strict: true})
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	session, responses := f.connect(t, callers["agent for user 9"])
	seen, err := whoami(ctx, session)
	require.NoError(t, err)
	require.Equal(t, "Bearer pair-nine", seen["authorization"])

	// The user revoked the account at the provider, and the upstream refuses it.
	f.upstream.refusal.Store(&wireRefusal{status: http.StatusUnauthorized, challenge: `Bearer resource_metadata="https://provider.test/metadata", error="invalid_token"`})
	result, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "whoami"})
	require.NoError(t, err, "the refused account ended the session")
	requireApprovalToolError(t, result)
	f.upstream.refusal.Store(nil)
	seen, err = whoami(ctx, session)
	require.NoError(t, err, "the session did not go on")
	require.Equal(t, "Bearer pair-nine", seen["authorization"])

	// The account's access token expired, and the provider refuses to refresh it.
	owner, connection := f.pairConnection(t, 9)
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthToken{}).Where("user_id = ? AND mcp_id = ?", owner, connection).Updates(map[string]any{
		"expiry":        time.Now().Add(-time.Hour),
		"refresh_token": "pair-nine-refresh",
		"token_url":     refusingTokenEndpoint(t),
		"auth_style":    oauth2.AuthStyleInParams,
	}).Error)
	f.upstream.requests()

	result, err = session.CallTool(ctx, &gomcp.CallToolParams{Name: "whoami"})
	require.NoError(t, err, "the broken account ended the session")
	requireApprovalToolError(t, result)
	for _, request := range f.upstream.requests() {
		require.NotEqual(t, "tools/call", request.method, "the call reached the upstream with a broken account")
	}
	_, err = f.gateway.GetMCPOAuthToken(ctx, owner, connection, f.upstream.server.URL)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "the refused account was kept")

	// Without an account, the agent's next request needs the user's approval as well.
	_, err = session.CallTool(ctx, &gomcp.CallToolParams{Name: "whoami"})
	if err == nil {
		t.Fatal("a call without the user's account succeeded")
	}
	last := responses.last(t)
	require.Equal(t, http.StatusConflict, last.status, last.body)
	require.Empty(t, last.challenge)
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentUserApprovalRequiredError,
		VMCPID:  wireVMCP,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: wireComponent, Code: types.AgentReadinessOAuthRequired}},
	}, readAgentFailure(t, strings.NewReader(last.body)))

	// Another user's pair, and the user's own connection, still work.
	for _, name := range []string{"agent for user 10", "user 9"} {
		other, _ := f.connect(t, callers[name])
		seen, err := whoami(ctx, other)
		require.NoError(t, err, name)
		require.Equal(t, "Bearer "+callers[name].bearer, seen["authorization"], name)
	}
	var challenges int64
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthChallenge{}).Count(&challenges).Error)
	require.Zero(t, challenges)
}

// TestAgentActsForAUserAfterApprovalWithSetup checks, end to end in Go, an agent acting on behalf of a user after the
// user approved it with setup: the agent asks for approval, the signed-in user enters the component's value through
// the setup API while the request is pending, and connects their upstream account (stored as the OAuth callback stores
// it for the pair), the controllers copy the value to the pair's connection, the user approves, the agent redeems the
// code and mints a token, and the agent, authenticated with it, connects for the user and calls a tool that needs both
// the user's value and their account. Before the setup, a pending request refuses nothing, and the approval page shows
// what is missing without a value.
func TestAgentActsForAUserAfterApprovalWithSetup(t *testing.T) {
	f, _ := newPairWire(t, pairWireOptions{})
	ctx := t.Context()

	// The agent asks for approval.
	secret := strings.Repeat("s", 43)
	digest := sha256.Sum256([]byte(secret))
	request, err := f.gateway.CreateAgentDelegationRequest(ctx, gatewayclient.AgentDelegationRequestInput{
		AgentID:    f.agent.ID,
		APIKeyID:   f.key.ID,
		ReturnURL:  "https://agent.example.com/callback",
		SecretHash: base64.RawURLEncoding.EncodeToString(digest[:]),
	})
	require.NoError(t, err)

	userExtra := map[string][]string{}
	principal.RecordKind(userExtra, principal.KindUser)
	signedIn := &user.DefaultInfo{UID: "9", Name: "user9", Groups: []string{types.GroupAuthenticated, types.GroupAPI}, Extra: userExtra}
	setup := handlers.NewAgentDelegationSetupHandler(f.grants, nil)
	call := func(method, path string, body any, pathValues map[string]string, handle func(api.Context) error) []byte {
		t.Helper()
		var reader *bytes.Reader
		if body != nil {
			data, err := json.Marshal(body)
			require.NoError(t, err)
			reader = bytes.NewReader(data)
		} else {
			reader = bytes.NewReader(nil)
		}
		r := httptest.NewRequest(method, path, reader)
		for name, value := range pathValues {
			r.SetPathValue(name, value)
		}
		recorder := httptest.NewRecorder()
		require.NoError(t, handle(api.Context{Request: r, ResponseWriter: recorder, Storage: f.storage, GatewayClient: f.gateway, User: signedIn}))
		require.Less(t, recorder.Code, 300, recorder.Body.String())
		return recorder.Body.Bytes()
	}
	requestPath := "/api/agent-delegation-requests/" + request.ID + "/setup"
	values := map[string]string{"request_id": request.ID, "setup_vmcp_id": wireVMCP}

	var before types.AgentDelegationSetup
	require.NoError(t, json.Unmarshal(call(http.MethodGet, requestPath, nil, values, setup.Get), &before))
	require.Len(t, before.VMCPs, 1)
	require.Len(t, before.VMCPs[0].Components, 1)
	require.False(t, before.VMCPs[0].Components[0].Covered, "the component takes the user's header")
	require.Equal(t, []types.AgentDelegationSetupField{{Key: wireHeader, Usage: types.Header}}, before.VMCPs[0].Components[0].Configuration)

	// The user enters the value through the agent, while the request is pending.
	value := "the user's value through the agent"
	body := call(http.MethodPut, requestPath+"/vmcps/"+wireVMCP+"/configuration", types.AgentDelegationSetupConfiguration{
		Components: map[string]map[string]*string{wireComponent: {wireHeader: &value}},
	}, values, setup.Configure)
	require.NotContains(t, string(body), value, "the setup API returned the value")
	var after types.AgentDelegationSetupVMCP
	require.NoError(t, json.Unmarshal(body, &after))
	require.True(t, after.Components[0].Configuration[0].Set)

	// The setup is the pair's: the controllers, which run in the background, give the pair's instance a connection to
	// the component, and copy the value to it.
	owner := principal.DelegatedOwner{AgentID: f.agent.ID, UserID: 9}.String()
	instance, err := vmcpaccess.FindInstance(ctx, f.storage, system.DefaultNamespace, wireVMCP, owner)
	require.NoError(t, err)
	require.NotNil(t, instance)
	connection := &v1.MCPServerInstance{Name: vmcpaccess.ComponentConnectionName(instance.Name, wireComponent), Namespace: system.DefaultNamespace}

	// The user connects their account through the agent, which the OAuth callback stores for the pair and its connection.
	require.NoError(t, f.gateway.ReplaceMCPOAuthToken(ctx, owner, connection.Name, f.upstream.server.URL, "", &oauth2.Config{ClientID: "client"},
		&oauth2.Token{AccessToken: "pair-nine", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}))

	// The user approves, and the agent redeems the code with the session's secret and mints a token.
	approver, ok := subjects.CallerFromUserInfo(signedIn)
	require.True(t, ok)
	_, code, err := f.gateway.ApproveAgentDelegationRequest(ctx, request.ID, approver)
	require.NoError(t, err)
	delegation, err := f.gateway.RedeemAgentDelegationCode(ctx, f.agent.ID, f.key.ID, request.ID, code, secret)
	require.NoError(t, err)
	caller := &pairCaller{name: "agent for user 9", outer: f.onBehalfOf(t, 9, delegation.ID, false)}

	// The agent connects for the user and calls the tool, which receives the user's value and account.
	session, _ := f.connect(t, caller)
	seen, err := whoami(ctx, session)
	require.NoError(t, err)
	require.Equal(t, value, seen["token"])
	require.Equal(t, "Bearer pair-nine", seen["authorization"])

	// The user's own setup, and the agent's own, have none of it.
	for _, other := range []string{"9", principal.Agent(f.agent.ID).String()} {
		found, err := vmcpaccess.FindInstance(ctx, f.storage, system.DefaultNamespace, wireVMCP, other)
		require.NoError(t, err)
		if found == nil {
			continue
		}
		_, err = f.gateway.RevealCredential(ctx, []string{vmcpaccess.InstanceConfigurationCredentialContext(found.Name)}, vmcpaccess.ConfigurationCredentialName())
		require.Error(t, err, "%s has a configuration of its own", other)
	}
}
