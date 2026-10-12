package mcpserver

import (
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(t *testing.T, objects ...kclient.Object) kclient.WithWatch {
	t.Helper()

	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithIndex(&v1.MCPNetworkPolicy{}, "spec.mcpServerName", func(obj kclient.Object) []string {
			policy := obj.(*v1.MCPNetworkPolicy)
			if policy.Spec.MCPServerName == "" {
				return nil
			}
			return []string{policy.Spec.MCPServerName}
		}).
		WithObjects(objects...).
		Build()
}

func newMCPServer(name string) *v1.MCPServer {
	return &v1.MCPServer{
		APIVersion: v1.SchemeGroupVersion.String(),
		Kind:       "MCPServer",
		Name:       name,
		Namespace:  "default",
	}
}

func newTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gatewayClient := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	return gatewayClient
}

func TestShouldSyncOAuthMetadata(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name            string
		lastRequestTime metav1.Time
		lastSyncTime    metav1.Time
		expected        bool
	}{
		{
			name:     "skips server with no requests",
			expected: false,
		},
		{
			name:            "syncs after request with no previous sync",
			lastRequestTime: metav1.NewTime(now.Add(-5 * time.Minute)),
			expected:        true,
		},
		{
			name:            "skips when last request predates sync",
			lastRequestTime: metav1.NewTime(now.Add(-2 * time.Hour)),
			lastSyncTime:    metav1.NewTime(now.Add(-90 * time.Minute)),
			expected:        false,
		},
		{
			name:            "skips when sync interval has not elapsed",
			lastRequestTime: metav1.NewTime(now.Add(-10 * time.Minute)),
			lastSyncTime:    metav1.NewTime(now.Add(-30 * time.Minute)),
			expected:        false,
		},
		{
			name:            "syncs after interval and newer request",
			lastRequestTime: metav1.NewTime(now.Add(-10 * time.Minute)),
			lastSyncTime:    metav1.NewTime(now.Add(-2 * time.Hour)),
			expected:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newMCPServer("test-server")
			server.Status.LastRequestTime = tt.lastRequestTime
			server.Status.LastOAuthMetadataSync = tt.lastSyncTime

			assert.Equal(t, tt.expected, shouldSyncOAuthMetadata(server, now))
		})
	}
}

func TestShutdownIdleServersSetsLastRequestTimeForOlderServers(t *testing.T) {
	server := newMCPServer("older-server")
	server.CreationTimestamp = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{}).ShutdownIdleServers(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServer
	require.NoError(t, client.Get(t.Context(), router.Key(server.Namespace, server.Name), &updated))
	assert.False(t, updated.Status.LastRequestTime.IsZero())
	assert.WithinDuration(t, time.Now(), updated.Status.LastRequestTime.Time, 5*time.Second)
}

func TestShutdownIdleServersSkipsRecentlyCreatedServersWithoutLastRequestTime(t *testing.T) {
	server := newMCPServer("new-server")
	server.CreationTimestamp = metav1.NewTime(time.Now().Add(-30 * time.Second))

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{}).ShutdownIdleServers(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServer
	require.NoError(t, client.Get(t.Context(), router.Key(server.Namespace, server.Name), &updated))
	assert.True(t, updated.Status.LastRequestTime.IsZero())
}

func TestShutdownIdleServersSchedulesRetryUsingServerSpecificInterval(t *testing.T) {
	server := newMCPServer("custom-interval")
	server.Spec.Manifest.IdleShutdownIntervalHours = 5
	server.Status.LastRequestTime = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	req := router.Request{
		Client:    newFakeClient(t, server),
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}
	resp := &router.ResponseWrapper{}

	err := (&Handler{
		singleUserIdleShutdownDelay: 15 * time.Hour,
		multiUserIdleShutdownDelay:  20 * time.Hour,
	}).ShutdownIdleServers(req, resp)
	require.NoError(t, err)

	assert.InDelta(t, (3 * time.Hour).Seconds(), resp.Delay.Seconds(), 1)
}

func TestShutdownIdleServersUsesMultiUserDefaultIdleInterval(t *testing.T) {
	server := newMCPServer("shared-server")
	server.Spec.MCPCatalogID = "catalog-1"
	server.Status.LastRequestTime = metav1.NewTime(time.Now().Add(-2 * time.Hour))

	req := router.Request{
		Client:    newFakeClient(t, server),
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}
	resp := &router.ResponseWrapper{}

	err := (&Handler{
		singleUserIdleShutdownDelay: 15 * time.Hour,
		multiUserIdleShutdownDelay:  9 * time.Hour,
	}).ShutdownIdleServers(req, resp)
	require.NoError(t, err)

	assert.InDelta(t, (7 * time.Hour).Seconds(), resp.Delay.Seconds(), 1)
}

func TestShutdownIdleServersSkipsWhenShutdownDisabled(t *testing.T) {
	server := newMCPServer("disabled-shutdown")
	server.Spec.Manifest.IdleShutdownIntervalHours = -1
	server.Status.LastRequestTime = metav1.NewTime(time.Now().Add(-24 * time.Hour))

	req := router.Request{
		Client:    newFakeClient(t, server),
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}
	resp := &router.ResponseWrapper{}

	err := (&Handler{
		singleUserIdleShutdownDelay: 15 * time.Hour,
	}).ShutdownIdleServers(req, resp)
	require.NoError(t, err)
	assert.Zero(t, resp.Delay)
}

func TestEnsureMCPNetworkPolicyCreatesPolicy(t *testing.T) {
	server := newMCPServer("egress-server")
	server.Spec.Manifest.Runtime = types.RuntimeNPX
	server.Spec.Manifest.NPXConfig = &types.NPXRuntimeConfig{
		Package:       "@test/package",
		EgressDomains: []string{"api.example.com", "*.google.com"},
	}

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{networkPolicyProviderEnabled: true}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(t.Context(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Len(t, policies.Items, 1)
	policy := policies.Items[0]
	assert.True(t, strings.HasPrefix(policy.Name, "mnp1"))
	assert.Equal(t, server.Name, policy.Spec.MCPServerName)
	assert.Equal(t, map[string]string{"app": server.Name}, policy.Spec.PodSelector)
	assert.Equal(t, []string{"*.google.com", "api.example.com"}, policy.Spec.EgressDomains)
	assert.False(t, policy.Spec.DenyAllEgress)
}

func TestEnsureMCPNetworkPolicyCreatesDenyAllPolicy(t *testing.T) {
	server := newMCPServer("deny-all-server")
	server.Spec.Manifest.Runtime = types.RuntimeUVX
	server.Spec.Manifest.UVXConfig = &types.UVXRuntimeConfig{
		Package:       "test-package",
		DenyAllEgress: new(true),
	}

	client := newFakeClient(t, server)
	req := router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{networkPolicyProviderEnabled: true}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(t.Context(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Len(t, policies.Items, 1)
	assert.Empty(t, policies.Items[0].Spec.EgressDomains)
	assert.True(t, policies.Items[0].Spec.DenyAllEgress)
}

func TestEnsureMCPNetworkPolicyOpenAPI(t *testing.T) {
	for _, tt := range []struct {
		name              string
		config            types.OpenAPIRuntimeConfig
		defaultDenyAll    bool
		wantDomains       []string
		wantDenyAllEgress bool
	}{
		{
			name: "configured domains replace an existing policy",
			config: types.OpenAPIRuntimeConfig{
				EgressDomains: []string{"api.example.com", "*.example.com"},
			},
			defaultDenyAll: true,
			wantDomains:    []string{"*.example.com", "api.example.com"},
		},
		{
			name:              "global deny default applies when no domains are configured",
			defaultDenyAll:    true,
			wantDenyAllEgress: true,
		},
		{
			name: "explicit allow all overrides the deny default",
			config: types.OpenAPIRuntimeConfig{
				DenyAllEgress: new(false),
			},
			defaultDenyAll: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := newMCPServer("openapi-server")
			server.Spec.Manifest.Runtime = types.RuntimeOpenAPI
			server.Spec.Manifest.OpenAPIConfig = &tt.config

			existing := &v1.MCPNetworkPolicy{
				Name:      "existing-policy",
				Namespace: server.Namespace,
				Spec: v1.MCPNetworkPolicySpec{
					MCPServerName: server.Name,
					DenyAllEgress: true,
				},
			}
			client := newFakeClient(t, server, existing)
			req := router.Request{
				Client:    client,
				Ctx:       t.Context(),
				Object:    server,
				Namespace: server.Namespace,
				Name:      server.Name,
			}

			err := (&Handler{
				networkPolicyProviderEnabled: true,
				defaultDenyAllEgress:         tt.defaultDenyAll,
			}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
			require.NoError(t, err)

			var policies v1.MCPNetworkPolicyList
			require.NoError(t, client.List(t.Context(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
				"spec.mcpServerName": server.Name,
			}))
			require.Len(t, policies.Items, 1)
			assert.Equal(t, tt.wantDomains, policies.Items[0].Spec.EgressDomains)
			assert.Equal(t, tt.wantDenyAllEgress, policies.Items[0].Spec.DenyAllEgress)
			assert.Equal(t, map[string]string{"app": server.Name}, policies.Items[0].Spec.PodSelector)
		})
	}
}

func TestEnsureMCPNetworkPolicyDeletesPolicyWhenProviderDisabled(t *testing.T) {
	server := newMCPServer("no-provider-server")
	existing := &v1.MCPNetworkPolicy{
		Name:      server.Name,
		Namespace: server.Namespace,
		Spec: v1.MCPNetworkPolicySpec{
			MCPServerName: server.Name,
		},
	}

	client := newFakeClient(t, server, existing)
	req := router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(t.Context(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Empty(t, policies.Items)
}

func TestEnsureMCPNetworkPolicyDeletesPolicyForUnsupportedRuntime(t *testing.T) {
	server := newMCPServer("remote-server")
	server.Spec.Manifest.Runtime = types.RuntimeRemote
	server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"}

	existing := &v1.MCPNetworkPolicy{
		Name:      server.Name,
		Namespace: server.Namespace,
		Spec: v1.MCPNetworkPolicySpec{
			MCPServerName: server.Name,
		},
	}

	client := newFakeClient(t, server, existing)
	req := router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}

	err := (&Handler{networkPolicyProviderEnabled: true}).EnsureMCPNetworkPolicy(req, &router.ResponseWrapper{})
	require.NoError(t, err)

	var policies v1.MCPNetworkPolicyList
	require.NoError(t, client.List(t.Context(), &policies, kclient.InNamespace(server.Namespace), kclient.MatchingFields{
		"spec.mcpServerName": server.Name,
	}))
	require.Empty(t, policies.Items)
}

func TestEnsureMCPCatalogIDRecordsScopeForSnapshotBackedServers(t *testing.T) {
	const entryName = "default-everything-c001b50cc6rtk"

	tests := []struct {
		name              string
		entrySpec         v1.MCPServerCatalogEntrySpec
		existingStatusID  string
		expectedCatalogID string
	}{
		{
			name:              "catalog-scoped entry records its catalog",
			entrySpec:         v1.MCPServerCatalogEntrySpec{MCPCatalogName: "default"},
			existingStatusID:  "",
			expectedCatalogID: "default",
		},
		{
			name:              "workspace-scoped entry records its workspace",
			entrySpec:         v1.MCPServerCatalogEntrySpec{PowerUserWorkspaceID: "puw1-test"},
			existingStatusID:  "",
			expectedCatalogID: "puw1-test",
		},
		{
			name:              "unscoped entry leaves the status untouched",
			entrySpec:         v1.MCPServerCatalogEntrySpec{},
			existingStatusID:  "",
			expectedCatalogID: "",
		},
		{
			name:              "legacy entry-name status is replaced by the workspace",
			entrySpec:         v1.MCPServerCatalogEntrySpec{PowerUserWorkspaceID: "puw1-test"},
			existingStatusID:  entryName,
			expectedCatalogID: "puw1-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &v1.MCPServerCatalogEntry{
				Name:      entryName,
				Namespace: "default",
				Spec:      tt.entrySpec,
			}
			server := &v1.MCPServer{
				Name:      "ms1-vmcp-component",
				Namespace: "default",
				Spec: v1.MCPServerSpec{
					MCPServerCatalogEntryName: entryName,
					VMCPInstanceID:            "vmcpi1-test",
				},
				Status: v1.MCPServerStatus{MCPCatalogID: tt.existingStatusID},
			}

			client := newFakeClient(t, entry, server)
			require.NoError(t, (&Handler{}).EnsureMCPCatalogID(router.Request{
				Client:    client,
				Ctx:       t.Context(),
				Object:    server,
				Namespace: server.Namespace,
				Name:      server.Name,
			}, &router.ResponseWrapper{}))

			var updated v1.MCPServer
			require.NoError(t, client.Get(t.Context(), router.Key(server.Namespace, server.Name), &updated))
			assert.Equal(t, tt.expectedCatalogID, updated.Status.MCPCatalogID)
		})
	}
}
