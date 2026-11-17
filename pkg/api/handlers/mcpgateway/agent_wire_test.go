package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/attribution"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/authentication/user"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	wireVMCP      = "vmcp1wire"
	wireComponent = "one"
	wireHeader    = "X-Token"
)

// wireSeen is what the upstream received in one request.
type wireSeen struct {
	authorization, token, method string
}

// wireRefusal is how the upstream answers every request while it is set.
type wireRefusal struct {
	status    int
	challenge string
}

// wireUpstream is the shared component's upstream MCP server. Its whoami tool answers with the Authorization and
// X-Token headers of the request that called it, it records those headers for every request it receives, and it can
// be told to refuse every request with an authorization failure.
type wireUpstream struct {
	server *httptest.Server
	// bearers are the access tokens it accepts.
	bearers []string

	lock    sync.Mutex
	seen    []wireSeen
	refusal atomic.Pointer[wireRefusal]
}

// wireCaller is a caller of the vMCP with its own connection to the shared component.
type wireCaller struct {
	name string
	// outer is the caller as the outer vMCP gateway authenticates it.
	outer      user.Info
	instance   string
	connection string
	// token is the X-Token value that the caller's connection carries.
	token string
	// bearer is the upstream access token that the caller's requests must carry.
	bearer string
}

// wireFixture is a shared vMCP whose one component is a shared remote server that takes a user-supplied X-Token
// header, with an agent profile for agent 7, a wildcard agent profile and a profile for every user. Agent 7, agent 8
// and user 7 are each connected, and each connection carries its own X-Token value.
//
// It serves the whole path a caller's MCP request takes, each hop over HTTP: an outer gateway that resolves the
// caller's vMCP configuration with the session manager and mints its composite loopback token; the composite
// (mmmcp), which reaches each component connection with that token; and the component gateway, the MCP gateway's own
// proxy handler, which selects the connection's headers and OAuth token and proxies to the upstream. The outer and
// composite hops stand in for the gateway's own vMCP path, which needs a token service and a tunnel manager; the token
// is a test token that carries the same claims, and is authenticated at each hop as the loopback authenticator does.
type wireFixture struct {
	upstream  *wireUpstream
	storage   storage.Client
	gateway   *gatewayclient.Client
	db        *gorm.DB
	manager   *mcp.SessionManager
	composite *mmmcp.Composite
	// compositeURL is the base URL of the composite loopback.
	compositeURL string
	callers      map[string]*wireCaller
}

// wireInitialEventsStorage supplies the initial events of a watch, which the Kubernetes API server sends and the
// fake client does not, so that the session manager's waits see existing objects.
type wireInitialEventsStorage struct {
	storage.Client
}

// wireResponses records the HTTP responses an MCP client receives.
type wireResponses struct {
	lock      sync.Mutex
	responses []wireResponse
}

type wireResponse struct {
	status    int
	challenge []string
	body      string
}

func newWireUpstream(t *testing.T, bearers ...string) *wireUpstream {
	t.Helper()
	u := &wireUpstream{bearers: bearers}
	server := gomcp.NewServer(&gomcp.Implementation{Name: "upstream", Version: "test"}, nil)
	server.AddTool(&gomcp.Tool{Name: "whoami", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		body, err := json.Marshal(map[string]string{
			"authorization": req.GetExtra().Header.Get("Authorization"),
			"token":         req.GetExtra().Header.Get(wireHeader),
		})
		if err != nil {
			return nil, err
		}
		return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: string(body)}}}, nil
	})
	transport := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil)
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			// Something on the test host may probe listening ports.
			transport.ServeHTTP(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var message struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &message)
		u.lock.Lock()
		u.seen = append(u.seen, wireSeen{authorization: r.Header.Get("Authorization"), token: r.Header.Get(wireHeader), method: message.Method})
		u.lock.Unlock()

		if refusal := u.refusal.Load(); refusal != nil {
			w.Header().Set("WWW-Authenticate", refusal.challenge)
			http.Error(w, "private provider error", refusal.status)
			return
		}
		if !slices.Contains(u.bearers, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://provider.test/metadata"`)
			http.Error(w, "private provider error", http.StatusUnauthorized)
			return
		}
		transport.ServeHTTP(w, r)
	}))
	t.Cleanup(u.server.Close)
	return u
}

// requests returns what the upstream received since the last call, and forgets it.
func (u *wireUpstream) requests() []wireSeen {
	u.lock.Lock()
	defer u.lock.Unlock()
	seen := u.seen
	u.seen = nil
	return seen
}

func (s *wireInitialEventsStorage) Watch(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) (watch.Interface, error) {
	upstream, err := s.Client.Watch(ctx, list, opts...)
	if err != nil {
		return nil, err
	}
	initial := list.DeepCopyObject().(kclient.ObjectList)
	listOptions := &kclient.ListOptions{}
	listOptions.ApplyOptions(opts)
	listOptions.Raw = nil
	if err := s.List(ctx, initial, listOptions); err != nil {
		upstream.Stop()
		return nil, err
	}
	objects, err := meta.ExtractList(initial)
	if err != nil {
		upstream.Stop()
		return nil, err
	}
	result := make(chan watch.Event)
	proxy := watch.NewProxyWatcher(result)
	go func() {
		defer close(result)
		defer upstream.Stop()
		for _, object := range objects {
			select {
			case result <- watch.Event{Type: watch.Added, Object: object}:
			case <-proxy.StopChan():
				return
			}
		}
		for {
			select {
			case event, ok := <-upstream.ResultChan():
				if !ok {
					return
				}
				select {
				case result <- event:
				case <-proxy.StopChan():
					return
				}
			case <-proxy.StopChan():
				return
			}
		}
	}()
	return proxy, nil
}

// wireLoopbackCaller is the principal a composite loopback token authenticates as.
func wireLoopbackCaller(claims *persistent.TokenContext) user.Info {
	extra := map[string][]string{
		"oauth_connection_id": {claims.OAuthConnectionID},
		"authorized_mcp_ids":  []string(claims.AuthorizedMCPIDs),
	}
	if claims.OAuthClientID != "" {
		extra["oauth_client_id"] = []string{claims.OAuthClientID}
	}
	if claims.PrincipalKind == string(principal.KindAgent) {
		p, _ := principal.Parse(claims.UserID)
		id, _ := p.AgentID()
		return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: id}, []string(claims.UserGroups), extra)
	}
	principal.RecordKind(extra, principal.KindUser)
	return &user.DefaultInfo{UID: claims.UserID, Name: claims.UserName, Groups: []string(claims.UserGroups), Extra: extra}
}

func newWireFixture(t *testing.T) *wireFixture {
	t.Helper()
	upstream := newWireUpstream(t, "seven-grant", "agents-grant", "user-seven-own")
	component := types.VMCPComponent{
		ID:            wireComponent,
		Name:          "one",
		Configuration: []types.VMCPConfigurationPolicy{{Key: wireHeader, Policy: types.VMCPConfigurationPolicyUserAllowed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{FixedURL: upstream.server.URL},
			Config:       []types.MCPConfig{{Key: wireHeader, Required: true, Usage: types.Header}},
		}},
	}
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	vmcp := &v1.VMCP{Name: wireVMCP, Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		DisplayName: "Wire",
		Components:  []types.VMCPComponent{component},
		Profiles: []types.VMCPProfile{
			profile("vp1seven", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			profile("vp1agents", types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
			profile("vp1users", types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}),
		},
	}}}
	require.True(t, vmcpaccess.IsMultiUser(component), "the component must share one server")
	shared := &v1.MCPServer{Name: "ms1wire", Namespace: system.DefaultNamespace, Spec: v1.MCPServerSpec{
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
	callers := []*wireCaller{
		{
			name: "agent 7",
			outer: gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, []string{types.GroupMCP, types.GroupAuthenticated}, map[string][]string{
				"authorized_mcp_ids": {"*"},
			}),
			token:  "agent-seven-value",
			bearer: "seven-grant",
		},
		{
			name: "agent 8",
			outer: gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 8}, []string{types.GroupMCP, types.GroupAuthenticated}, map[string][]string{
				"authorized_mcp_ids": {"*"},
			}),
			token:  "agent-eight-value",
			bearer: "agents-grant",
		},
		{
			// The user with agent 7's numeric ID.
			name:   "user 7",
			outer:  &user.DefaultInfo{UID: "7", Name: "alice", Groups: []string{types.GroupAuthenticated}, Extra: userExtra},
			token:  "user-seven-value",
			bearer: "user-seven-own",
		},
	}
	objects := []kclient.Object{vmcp, shared}
	for i, caller := range callers {
		owner, err := principal.CallerOwner(caller.outer)
		require.NoError(t, err)
		instance := &v1.VMCPInstance{
			Name:      "vmcpi1wire" + strconv.Itoa(i),
			Namespace: system.DefaultNamespace,
			Spec:      v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
		}
		connection := &v1.MCPServerInstance{
			Name:      "msi1wire" + strconv.Itoa(i),
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:          owner,
				VMCPInstanceID:  instance.Name,
				VMCPComponentID: wireComponent,
				MCPServerName:   shared.Name,
				Config:          vmcpaccess.ConnectionConfiguration(component),
			},
		}
		caller.instance, caller.connection = instance.Name, connection.Name
		objects = append(objects, instance, connection)
	}
	storageClient := &wireInitialEventsStorage{Client: fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.VMCPInstance{}, "metadata.name", func(obj kclient.Object) []string { return []string{obj.GetName()} }).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.LegacySlug} }).
		WithIndex(&v1.VMCP{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCP).Spec.LegacySlug} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServer).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerName", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.MCPServerName} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.MCPServerInstance).Spec.UserID} }).
		WithStatusSubresource(&v1.MCPClientSession{}, &v1.MCPServer{}, &v1.VMCP{}).
		WithObjects(objects...).
		Build()}

	gateway, db := newHeadlessTestGatewayClient(t)

	// An administrator configures the header for the agents' profiles: agent 7's own profile, and the wildcard profile,
	// which agent 8 falls back to.
	configurations := vmcpaccess.NewProfileConfigurationStore(gateway)
	for _, profile := range []struct{ id, value string }{{id: "vp1seven", value: callers[0].token}, {id: "vp1agents", value: callers[1].token}} {
		_, err := configurations.Update(t.Context(), storageClient, kclient.ObjectKeyFromObject(vmcp), profile.id, map[string]map[string]*string{wireComponent: {wireHeader: new(profile.value)}})
		require.NoError(t, err)
	}
	require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), vmcp))
	for _, caller := range callers {
		// What the VMCPInstance controller records once it has synchronized the instance, including, for an agent, its
		// profiles' current configuration.
		var instance v1.VMCPInstance
		require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: caller.instance}, &instance))
		if principal.IsAgent(caller.outer) {
			configuration := vmcpaccess.ResolveAgentConfiguration(*vmcp, instance, caller.outer)
			instance.Status.AgentConfigurationRevision = configuration.Revision
			instance.Status.AgentConfigurationRevisions = configuration.Revisions
		}
		instance.Status.ConfigurationCheckHash = vmcpaccess.ConfigurationCheckHash(vmcpaccess.EnabledComponents(caller.outer, *vmcp, vmcpaccess.ComponentsForInstance(*vmcp, instance)), "")
		require.NoError(t, storageClient.Update(t.Context(), &instance))
		// What the MCPServerInstance controller records once it has copied the connection's configuration.
		var connection v1.MCPServerInstance
		require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: caller.connection}, &connection))
		connection.Status.VMCPConfigurationHash = vmcpaccess.ConnectionConfigurationHash(connection.Spec.Config, instance)
		require.NoError(t, storageClient.Update(t.Context(), &connection))
	}
	for _, caller := range callers {
		owner, _ := principal.CallerOwner(caller.outer)
		// What the MCPServerInstance controller copies from the instance's configuration, which for an agent comes from
		// its profiles and for a user from the user's own entries (SyncVMCPConfiguration).
		require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: owner + "-" + caller.connection,
			Name:    caller.connection,
			Secrets: map[string]string{wireHeader: caller.token},
		}))
	}
	// The grants an administrator authorized for the agents' profiles, and the user's own token for the user's
	// connection.
	conf := &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: "https://provider.test/token"}}
	for _, grant := range []struct{ profile, token string }{{profile: "vp1seven", token: "seven-grant"}, {profile: "vp1agents", token: "agents-grant"}} {
		require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), principal.ProfileGrant(vmcp.Name, grant.profile), vmcpaccess.ProfileOAuthGrantMCPID(vmcp.Name, wireComponent), upstream.server.URL, "grant-"+grant.profile, "1",
			conf, &oauth2.Token{AccessToken: grant.token, TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}))
	}
	require.NoError(t, gateway.ReplaceMCPOAuthToken(t.Context(), "7", callers[2].connection, upstream.server.URL, "", conf,
		&oauth2.Token{AccessToken: "user-seven-own", TokenType: "Bearer", Expiry: time.Now().Add(time.Hour)}))

	// The component gateway, which the session manager names as each component's local URL, and which proxies to the
	// upstream as the caller its composite loopback token authenticates.
	var handler *Handler
	tokens := fakeCompositeTokens{}
	componentGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.URL.Path, "/mcp-connect/")
		claims, err := tokens.DecodeToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if !ok || err != nil || !slices.Contains(claims.AuthorizedMCPIDs, id) {
			http.Error(w, "the composite loopback token does not authorize this connection", http.StatusForbidden)
			return
		}
		r.SetPathValue("mcp_id", id)
		if err := handler.Proxy(api.Context{
			Request:        r,
			ResponseWriter: w,
			Storage:        storageClient,
			GatewayClient:  gateway,
			APIBaseURL:     "https://obot.example/api",
			User:           wireLoopbackCaller(claims),
		}); err != nil {
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
	manager, err := mcp.NewSessionManager(t.Context(), false, false, globalTokens, nil, "https://obot.example", port,
		mcp.Options{MCPRuntimeBackend: mcp.RuntimeBackendKubernetes, MCPNamespace: "mcp"}, nil,
		&rest.Config{Host: "http://kubernetes.invalid"}, cluster, cluster, storageClient, gateway, system.DefaultNamespace, nil)
	require.NoError(t, err)
	grants := vmcpaccess.NewProfileOAuthGrantStore(gateway)
	manager.SetProfileOAuthGrants(grants)
	handler = &Handler{ctx: t.Context(), mcpSessionManager: manager, globalTokenStore: globalTokens, auditLogCollector: new(recordingProxyAuditCollector)}

	composite, err := mmmcp.New(t.Context(), &mmmcpconfig.Config{}, mmmcp.Options{
		ForwardClientInfo:     true,
		OAuth:                 compositeComponentAuth{ctx: t.Context(), tokens: tokens},
		AuthorizationResponse: compositeAuthorizationResponse(manager),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = composite.Close() })

	f := &wireFixture{
		upstream:  upstream,
		storage:   storageClient,
		gateway:   gateway,
		db:        db,
		manager:   manager,
		composite: composite,
		callers:   map[string]*wireCaller{},
	}
	for _, caller := range callers {
		f.callers[caller.name] = caller
	}

	// The composite loopback, as the gateway serves it at /mcp-connect-composite/{mcp_id} for the caller its token
	// authenticates.
	compositeLoopback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := strings.CutPrefix(r.URL.Path, "/mcp-connect-composite/")
		claims, err := tokens.DecodeToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if !ok || err != nil {
			http.Error(w, "the composite loopback token is refused", http.StatusUnauthorized)
			return
		}
		r.SetPathValue("mcp_id", id)
		req := api.Context{Request: r, ResponseWriter: w, Storage: storageClient, GatewayClient: gateway, APIBaseURL: "https://obot.example/api", User: wireLoopbackCaller(claims)}
		serverConfig, err := manager.ServerConfigForVMCP(r.Context(), id, req.User)
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
		ctx := context.WithValue(r.Context(), compositeAPIContextKey{}, req)
		ctx = context.WithValue(ctx, upstreamCaptureConfigKey{}, serverConfig)
		ctx = mmmcp.ContextWithConfig(ctx, cfg)
		ctx = mmmcp.ContextWithConfigID(ctx, compositeConfigID(serverConfig))
		composite.HTTPHandler().ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(compositeLoopback.Close)
	f.compositeURL = compositeLoopback.URL
	return f
}

// frontend returns the URL at which caller reaches the vMCP: the outer gateway, which resolves the caller's
// configuration, readiness included, issues its composite loopback token, proxies to the composite loopback, and rewrites
// any authorization challenge in the answer, as the gateway's vMCP path does.
func (f *wireFixture) frontend(t *testing.T, caller *wireCaller) string {
	t.Helper()
	target, err := url.Parse(f.compositeURL)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("mcp_id", wireVMCP)
		req := api.Context{Request: r, ResponseWriter: w, Storage: f.storage, GatewayClient: f.gateway, APIBaseURL: "https://obot.example/api", User: caller.outer}
		serverConfig, err := f.manager.ServerConfigForVMCP(r.Context(), wireVMCP, caller.outer)
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
		now := time.Now()
		token, _, err := newCompositeLoopbackToken(r.Context(), fakeCompositeTokens{}, compositeLoopbackTokenContext(caller.outer, f.compositeURL+"/mcp-connect-composite/"+serverConfig.MCPServerName, serverConfig.MCPServerName, authorized, wireVMCP, now), now)
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

func (r *wireResponses) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil || req.Method != http.MethodPost {
		return resp, err
	}
	recorded := wireResponse{status: resp.StatusCode, challenge: resp.Header.Values("WWW-Authenticate")}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		recorded.body = string(body)
	}
	r.lock.Lock()
	r.responses = append(r.responses, recorded)
	r.lock.Unlock()
	return resp, nil
}

// count returns how many responses the client received.
func (r *wireResponses) count() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return len(r.responses)
}

// last returns the last response the client received.
func (r *wireResponses) last(t *testing.T) wireResponse {
	t.Helper()
	r.lock.Lock()
	defer r.lock.Unlock()
	require.NotEmpty(t, r.responses)
	return r.responses[len(r.responses)-1]
}

// connect opens a stateful MCP session of caller with the vMCP, and returns it with the responses its client receives.
func (f *wireFixture) connect(t *testing.T, caller *wireCaller) (*gomcp.ClientSession, *wireResponses) {
	t.Helper()
	responses := &wireResponses{}
	session, err := gomcp.NewClient(&gomcp.Implementation{Name: "client", Version: "test"}, nil).Connect(t.Context(), &gomcp.StreamableClientTransport{
		Endpoint:             f.frontend(t, caller),
		HTTPClient:           &http.Client{Transport: responses},
		DisableStandaloneSSE: true,
	}, &gomcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if err != nil {
		t.Fatalf("%s could not connect: %v: %+v", caller.name, err, responses.responses)
	}
	require.NotEmpty(t, session.ID(), "the session must be stateful")
	t.Cleanup(func() { _ = session.Close() })
	return session, responses
}

// whoami calls the upstream's whoami tool through session, and returns the headers the upstream saw.
func whoami(ctx context.Context, session *gomcp.ClientSession) (map[string]string, error) {
	result, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "whoami"})
	if err != nil {
		return nil, err
	}
	if result.IsError || len(result.Content) != 1 {
		return nil, fmt.Errorf("unexpected result: %#v", result)
	}
	text, ok := result.Content[0].(*gomcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("unexpected content: %#v", result.Content[0])
	}
	var seen map[string]string
	return seen, json.Unmarshal([]byte(text.Text), &seen)
}

func (f *wireFixture) challenges(t *testing.T) []gatewaytypes.MCPOAuthChallenge {
	t.Helper()
	var challenges []gatewaytypes.MCPOAuthChallenge
	require.NoError(t, f.db.Find(&challenges).Error)
	return challenges
}

// TestSharedComponentHeadersOnTheWire runs agents and a user through the whole vMCP path at once, each over its own
// stateful MCP session, to a shared component whose only user-supplied input is a header. The upstream must see each
// caller's header value, and the OAuth token selected for it, only on that caller's requests: agent 7's value with
// its explicit profile's grant, agent 8's with the wildcard profile's grant, and the value of user 7, whose numeric ID
// is agent 7's, with the user's own token. Every request the upstream receives carries exactly one caller's pair.
func TestSharedComponentHeadersOnTheWire(t *testing.T) {
	f := newWireFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	sessions := map[string]*gomcp.ClientSession{}
	for name, caller := range f.callers {
		sessions[name], _ = f.connect(t, caller)
	}

	var (
		wait sync.WaitGroup
		lock sync.Mutex
		errs []error
	)
	for name, caller := range f.callers {
		for i := range 6 {
			wait.Go(func() {
				seen, err := whoami(ctx, sessions[name])
				if err == nil && (seen["token"] != caller.token || seen["authorization"] != "Bearer "+caller.bearer) {
					err = fmt.Errorf("%s call %d reached the upstream with %v", name, i, seen)
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

	// Every request on the wire, discovery and session setup included, carried one caller's header with that caller's
	// token, and each caller's pair was seen.
	pairs := map[wireSeen]bool{}
	for _, caller := range f.callers {
		pairs[wireSeen{authorization: "Bearer " + caller.bearer, token: caller.token}] = false
	}
	requests := f.upstream.requests()
	require.NotEmpty(t, requests)
	for _, seen := range requests {
		pair := wireSeen{authorization: seen.authorization, token: seen.token}
		_, ok := pairs[pair]
		require.True(t, ok, "the upstream received %q with token %q in a %s request", seen.token, seen.authorization, seen.method)
		pairs[pair] = true
	}
	for pair, seen := range pairs {
		require.True(t, seen, "the upstream never received %v", pair)
	}
}

// TestAgentMidSessionUpstreamRefusal checks what an agent with a working session gets when the upstream starts
// refusing its profile's grant: with a 401 challenge, and then with an insufficient_scope challenge. Each time the
// agent's request is answered with a 409 that names the component's grant, in the profile that holds it, as invalid,
// without a challenge header, and no MCP OAuth challenge or pending authorization is stored. A user on the same
// component, in the same situation, is challenged, and the challenge is stored for the user's connection.
func TestAgentMidSessionUpstreamRefusal(t *testing.T) {
	f := newWireFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	agent := f.callers["agent 7"]
	for _, refusal := range []wireRefusal{
		{status: http.StatusUnauthorized, challenge: `Bearer resource_metadata="https://provider.test/metadata", error="invalid_token"`},
		{status: http.StatusForbidden, challenge: `Bearer error="insufficient_scope", scope="write", resource_metadata="https://provider.test/metadata"`},
	} {
		t.Run(strconv.Itoa(refusal.status), func(t *testing.T) {
			// A working session: the agent's request reaches the upstream with its explicit profile's grant.
			session, responses := f.connect(t, agent)
			seen, err := whoami(ctx, session)
			require.NoError(t, err)
			require.Equal(t, "Bearer seven-grant", seen["authorization"])
			require.Equal(t, agent.token, seen["token"])
			f.upstream.requests()
			before := responses.count()

			f.upstream.refusal.Store(&refusal)
			defer f.upstream.refusal.Store(nil)
			_, err = whoami(ctx, session)
			require.Error(t, err)
			require.Equal(t, before+1, responses.count(), "the refused call sent no request")
			last := responses.last(t)
			require.Equal(t, http.StatusConflict, last.status, last.body)
			require.Empty(t, last.challenge)
			require.NotContains(t, last.body, "private provider error")
			require.Equal(t, vmcpaccess.AgentReadinessFailure{
				Error:   vmcpaccess.AgentOAuthRequiredError,
				Reasons: []vmcpaccess.ReadinessReason{{ComponentID: wireComponent, Code: types.AgentReadinessOAuthInvalid, ProfileID: "vp1seven"}},
			}, readAgentFailure(t, strings.NewReader(last.body)))
			// The refused request reached the upstream with the explicit profile's grant, never the wildcard's.
			requests := f.upstream.requests()
			require.NotEmpty(t, requests)
			for _, request := range requests {
				require.Equal(t, "Bearer seven-grant", request.authorization)
			}
			requireNoOAuthState(t, f.db)
		})
	}

	// Once the upstream accepts the grant again, the agent connects and works again: the refusal changed nothing stored.
	session, _ := f.connect(t, agent)
	seen, err := whoami(ctx, session)
	require.NoError(t, err)
	require.Equal(t, "Bearer seven-grant", seen["authorization"])
	stored, err := f.gateway.GetMCPOAuthToken(t.Context(), principal.ProfileGrant(wireVMCP, "vp1seven"), vmcpaccess.ProfileOAuthGrantMCPID(wireVMCP, wireComponent), f.upstream.server.URL)
	require.NoError(t, err)
	require.Nil(t, stored.InvalidatedAt)

	// A user in the same situation is challenged, and the challenge is stored for the user's connection.
	userCaller := f.callers["user 7"]
	userSession, userResponses := f.connect(t, userCaller)
	seen, err = whoami(ctx, userSession)
	require.NoError(t, err)
	require.Equal(t, "Bearer user-seven-own", seen["authorization"])
	f.upstream.refusal.Store(&wireRefusal{status: http.StatusUnauthorized, challenge: `Bearer resource_metadata="https://provider.test/metadata", error="invalid_token"`})
	_, err = whoami(ctx, userSession)
	require.Error(t, err)
	last := userResponses.last(t)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, last.status, last.body)
	require.NotEmpty(t, last.challenge, "the user was not challenged")
	challenges := f.challenges(t)
	require.Len(t, challenges, 1)
	require.Equal(t, "7", challenges[0].UserID)
}
