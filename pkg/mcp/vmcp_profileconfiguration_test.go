package mcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// TestSharedComponentHeadersStayWithTheirConnection checks that the header values an agent is given for a shared
// component never reach another caller of the same shared server: each caller's request to it carries only the
// headers of the caller's own connection, a caller cannot use another's connection, and the MCP clients of different
// callers are never shared, while the server itself is.
func TestSharedComponentHeadersStayWithTheirConnection(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp1sharedheaders",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Shared headers",
			Components: []types.VMCPComponent{{
				ID:            "one",
				Name:          "one",
				Configuration: []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}},
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Runtime:      types.RuntimeRemote,
					RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://one.example.com/mcp"},
					Config:       []types.MCPConfig{{Key: "X-Token", Required: true, Usage: types.Header}},
				}},
			}},
			Profiles: []types.VMCPProfile{
				{ID: "vp1seven", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
				{ID: "vp1agents", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
				{ID: "vp1users", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
			},
		}},
	}
	require.True(t, vmcpaccess.IsMultiUser(vmcp.Spec.Manifest.Components[0]), "the component must share one server")
	shared := vmcpComponentServer("ms1sharedheaders", "", "", "one", "one", "https://one.example.com/mcp")
	shared.Spec.VMCPID = vmcp.Name
	shared.Spec.Manifest.Config = vmcpaccess.ComponentConfig(vmcp.Spec.Manifest.Components[0])

	callers := []struct {
		name   string
		caller kuser.Info
		token  string
	}{
		{
			name:   "agent with its own profile",
			caller: asAgent(7, 11),
			token:  "agent-seven-token",
		},
		{
			name:   "agent of the wildcard profile",
			caller: asAgent(8, 12),
			token:  "agent-eight-token",
		},
		{
			name:   "user with the first agent's numeric ID",
			caller: principaltest.AsUser(&kuser.DefaultInfo{UID: "7"}),
			token:  "user-seven-token",
		},
	}
	objects := []kclient.Object{vmcp, shared}
	connections := make([]string, len(callers))
	type credential struct {
		context, name string
		secrets       map[string]string
	}
	var credentials []credential
	for i, caller := range callers {
		owner, err := principal.CallerOwner(caller.caller)
		require.NoError(t, err)
		instance := syncedVMCPInstanceForUser(&v1.VMCPInstance{
			Name:      "vmcpi1headers" + string(rune('a'+i)),
			Namespace: system.DefaultNamespace,
			Spec:      v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
		}, vmcp, caller.caller)
		connection := syncedVMCPConnection(t, &v1.MCPServerInstance{
			Name:      "msi1headers" + string(rune('a'+i)),
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:          owner,
				VMCPInstanceID:  instance.Name,
				VMCPComponentID: "one",
				MCPServerName:   shared.Name,
			},
		}, vmcp, instance)
		objects = append(objects, instance, connection)
		connections[i] = connection.Name
		// What mcpserverinstance.SyncVMCPConfiguration copies from the instance, which holds an agent's profile
		// configuration and a user's own entries alike.
		credentials = append(credentials, credential{
			context: serverInstanceCredentialContext(*connection),
			name:    connection.Name,
			secrets: map[string]string{"X-Token": caller.token},
		})
	}
	storage := newVMCPTestStorage(objects...)
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), db, storage, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })
	for _, credential := range credentials {
		require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: credential.context, Name: credential.name, Secrets: credential.secrets}))
	}
	sm := &SessionManager{
		backend:                   &kubernetesBackend{},
		baseURL:                   "https://obot.example.com",
		storageClient:             storage,
		gatewayClient:             gateway,
		remoteURLValidationConfig: RemoteMCPURLValidationConfig{AllowLocalhostMCP: true},
	}
	t.Cleanup(sm.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	configs := make([]ServerConfig, len(callers))
	for i, caller := range callers {
		t.Run(caller.name, func(t *testing.T) {
			// Through the caller's connection, and through the shared server, which resolves to that connection.
			for _, id := range []string{connections[i], shared.Name} {
				_, cfg, err := sm.ServerForAction(ctx, id, caller.caller)
				require.NoError(t, err)
				require.Equal(t, connections[i], cfg.MCPServerInstanceID)
				require.Equal(t, []string{"X-Token"}, cfg.PassthroughHeaderNames)
				require.Equal(t, []string{caller.token}, cfg.PassthroughHeaderValues)
				configs[i] = cfg
			}
			// Another caller's connection is refused, so its headers are never sent for this caller.
			for j, other := range connections {
				if j == i {
					continue
				}
				_, _, err := sm.ServerForAction(ctx, other, caller.caller)
				var httpErr *types.ErrHTTP
				require.True(t, errors.As(err, &httpErr), "caller %s used connection %s: %v", caller.name, other, err)
			}
		})
	}
	for i := range configs {
		for j := range configs {
			if i == j {
				continue
			}
			require.Equal(t, shared.Name, configs[i].MCPServerName, "the shared component must be served by its one server")
			require.NotEqual(t, clientID(configs[i], "scope"), clientID(configs[j], "scope"), "%s and %s share an MCP client", callers[i].name, callers[j].name)
		}
	}
}

// TestAgentConnectFollowsProfileConfiguration checks that an agent is refused while its profiles lack a required
// value, that its connect waits for its instance to hold the profiles' current configuration before it is checked, and
// that a value supplied by the wildcard profile is attributed to it as a fallback.
func TestAgentConnectFollowsProfileConfiguration(t *testing.T) {
	vmcp, shared := sharedComponentVMCP("vmcp1profileconfig",
		types.VMCPProfile{
			ID:          "vp1agent",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "5"}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
		types.VMCPProfile{
			ID:          "vp1agents",
			Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}},
			Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
		},
	)
	component := &vmcp.Spec.Manifest.Components[0]
	component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Token", Usage: types.Header, Required: true}}
	component.Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Token", Policy: types.VMCPConfigurationPolicyUserAllowed}}
	storageClient := newVMCPTestStorage(vmcp, shared)
	runVMCPInstanceController(t, storageClient, vmcp.DeepCopy())
	manager := newAgentTestManager(storageClient)

	// No profile sets the token, so the agent is refused, to be fixed in its own profile.
	_, err := manager.ServerConfigForVMCP(t.Context(), vmcp.Name, asAgent(5, 11))
	notReady, ok := errors.AsType[*vmcpaccess.AgentNotReadyError](err)
	require.True(t, ok, "agent connect error = %v, want an AgentNotReadyError", err)
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error: vmcpaccess.AgentConfigurationRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{
			ComponentID: "one",
			Code:        types.AgentReadinessConfigurationRequired,
			Field:       "X-Token",
			ProfileID:   "vp1agent",
		}},
	}, notReady.Failure())

	// An administrator sets the token on the wildcard profile. Until the instance controller copies it to the agent's
	// instance, the agent's connect waits rather than going on with the instance's old configuration.
	var current v1.VMCP
	require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &current))
	current.Status.ProfileConfigurationRevisions = map[string]v1.VMCPProfileConfigurationRevision{
		"vp1agents": {Name: "configuration-agents", Keys: []string{vmcpaccess.ConfigurationKey("one", "X-Token")}},
	}
	require.NoError(t, storageClient.Update(t.Context(), &current))
	type result struct {
		cfg ServerConfig
		err error
	}
	connected := make(chan result, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		cfg, err := manager.ServerConfigForVMCP(ctx, vmcp.Name, asAgent(5, 11))
		connected <- result{cfg: cfg, err: err}
	}()
	select {
	case r := <-connected:
		t.Fatalf("the agent's connect did not wait for its profile configuration: %#v, %v", r.cfg, r.err)
	case <-time.After(300 * time.Millisecond):
	}

	var instances v1.VMCPInstanceList
	require.NoError(t, storageClient.List(t.Context(), &instances))
	require.Len(t, instances.Items, 1)
	syncedVMCPInstanceForUser(&instances.Items[0], &current, asAgent(5, 0))
	require.Equal(t, map[string]string{"vp1agents": "configuration-agents"}, instances.Items[0].Status.AgentConfigurationRevisions)
	require.NoError(t, storageClient.Update(t.Context(), &instances.Items[0]))

	r := <-connected
	require.NoError(t, r.err)
	cfg := r.cfg
	require.Equal(t, instances.Items[0].Name, cfg.MCPServerName)
	require.Equal(t, "vp1agent,vp1agents", cfg.AuditLogMetadata[AuditLogProfiles])
	require.Equal(t, "vp1agents", cfg.AuditLogMetadata[AuditLogFallbackProfiles])
}
