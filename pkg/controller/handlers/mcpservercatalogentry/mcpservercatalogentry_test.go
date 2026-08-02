package mcpservercatalogentry

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDetectCompositeDriftMarksEntryNeedingUpdateWhenMultiUserComponentDrifts(t *testing.T) {
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:           "Shared Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{
				{
					MCPServerID: "shared-server",
					Manifest:    componentSnapshot,
				},
			},
		},
	})
	sharedServer := newMCPServer("shared-server", types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:2.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	client := newFakeClient(compositeEntry, sharedServer)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.True(t, updated.Status.NeedsUpdate)
}

func TestStaticOAuthControllerCleanupRemovesCredentialProofsAndGrants(t *testing.T) {
	for _, tt := range []struct {
		name           string
		runtime        types.Runtime
		staticRequired bool
		cleanup        func(*Handler, router.Request) error
	}{
		{
			name:           "provider no longer requires static OAuth",
			runtime:        types.RuntimeRemote,
			staticRequired: false,
			cleanup: func(handler *Handler, req router.Request) error {
				return handler.CleanupUnusedOAuthCredentials(req, &router.ResponseWrapper{})
			},
		},
		{
			name:    "provider changed to a non-remote runtime",
			runtime: types.RuntimeContainerized,
			cleanup: func(handler *Handler, req router.Request) error {
				return handler.CleanupUnusedOAuthCredentials(req, &router.ResponseWrapper{})
			},
		},
		{
			name:           "catalog entry deletion",
			runtime:        types.RuntimeRemote,
			staticRequired: true,
			cleanup: func(handler *Handler, req router.Request) error {
				return handler.RemoveOAuthCredentials(req, &router.ResponseWrapper{})
			},
		},
		{
			name:    "catalog entry deletion after a non-remote transition",
			runtime: types.RuntimeContainerized,
			cleanup: func(handler *Handler, req router.Request) error {
				return handler.RemoveOAuthCredentials(req, &router.ResponseWrapper{})
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			manifest := types.MCPServerCatalogEntryManifest{Runtime: tt.runtime}
			if tt.runtime == types.RuntimeRemote {
				manifest.RemoteConfig = &types.RemoteCatalogConfig{
					FixedURL:            "https://mcp.example/api",
					StaticOAuthRequired: tt.staticRequired,
				}
			} else {
				manifest.ContainerizedConfig = &types.ContainerizedRuntimeConfig{Image: "example/mcp:latest"}
			}
			entry := newMCPServerCatalogEntry("entry-1", manifest)
			server := newMCPServer("server-1", types.MCPServerManifest{Runtime: types.RuntimeRemote})
			server.Spec.MCPServerCatalogEntryName = entry.Name
			instance := &v1.MCPServerInstance{
				ObjectMeta: metav1.ObjectMeta{Name: "instance-1", Namespace: entry.Namespace},
				Spec:       v1.MCPServerInstanceSpec{MCPServerCatalogEntryName: entry.Name},
			}
			storageClient := newFakeClient(entry, server, instance)

			services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
			require.NoError(t, err)
			database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
			require.NoError(t, err)
			require.NoError(t, database.AutoMigrate())
			gateway := gatewayclient.New(t.Context(), database, storageClient, nil, nil, nil, nil, time.Hour, 10, 0, 0, false)
			t.Cleanup(func() { require.NoError(t, gateway.Close()) })

			require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
				Context: system.MCPOAuthCredentialName(entry.Name),
				Name:    "oauth",
				Secrets: map[string]string{"CLIENT_ID": "client", "CLIENT_SECRET": "secret"},
			}))
			require.NoError(t, services.DB.DB.Create(&[]gatewaytypes.MCPOAuthToken{
				{MCPID: "fenced-grant", UserID: "user-1", CatalogEntryName: entry.Name},
				{MCPID: server.Name, UserID: "user-2"},
				{MCPID: instance.Name, UserID: "user-3"},
				{MCPID: "unrelated", UserID: "user-4", CatalogEntryName: "other-entry"},
			}).Error)
			require.NoError(t, services.DB.DB.Create(&gatewaytypes.MCPOAuthPendingState{
				HashedState: "pending-hash", MCPID: entry.Name, StaticOAuthTest: true,
			}).Error)

			req := router.Request{Client: storageClient, Ctx: t.Context(), Object: entry, Namespace: entry.Namespace, Name: entry.Name}
			require.NoError(t, tt.cleanup(NewHandler(gateway), req))

			_, err = gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(entry.Name)}, "oauth")
			require.Error(t, err)
			var targetedGrants int64
			require.NoError(t, services.DB.DB.Model(&gatewaytypes.MCPOAuthToken{}).
				Where("catalog_entry_name = ? OR mcp_id IN ?", entry.Name, []string{server.Name, instance.Name}).
				Count(&targetedGrants).Error)
			require.Zero(t, targetedGrants)
			var pendingProofs int64
			require.NoError(t, services.DB.DB.Model(&gatewaytypes.MCPOAuthPendingState{}).
				Where("mcp_id = ? AND static_o_auth_test = ?", entry.Name, true).
				Count(&pendingProofs).Error)
			require.Zero(t, pendingProofs)
			var unrelatedGrants int64
			require.NoError(t, services.DB.DB.Model(&gatewaytypes.MCPOAuthToken{}).
				Where("mcp_id = ?", "unrelated").Count(&unrelatedGrants).Error)
			require.EqualValues(t, 1, unrelatedGrants)
		})
	}
}

func TestStaticOAuthControllerCleanupPreservesRestoredProviderState(t *testing.T) {
	staleEntry := newMCPServerCatalogEntry("entry-1", types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL: "https://mcp.example/api",
		},
	})
	storageClient := newFakeClient(staleEntry)
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, storageClient, nil, nil, nil, nil, time.Hour, 10, 0, 0, false)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })

	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(staleEntry.Name),
		Name:    "oauth",
		Secrets: map[string]string{"CLIENT_ID": "client", "CLIENT_SECRET": "secret"},
	}))
	require.NoError(t, services.DB.DB.Create(&gatewaytypes.MCPOAuthToken{
		MCPID: "deployment-1", UserID: "user-1", CatalogEntryName: staleEntry.Name,
	}).Error)

	releaseCatalogMutationLock, err := gateway.AcquireCredentialLock(t.Context(), system.MCPStaticOAuthCatalogMutationLock)
	require.NoError(t, err)
	defer releaseCatalogMutationLock()

	cleanupDone := make(chan error, 1)
	go func() {
		cleanupDone <- NewHandler(gateway).CleanupUnusedOAuthCredentials(router.Request{
			Client: storageClient, Ctx: t.Context(), Object: staleEntry,
			Namespace: staleEntry.Namespace, Name: staleEntry.Name,
		}, &router.ResponseWrapper{})
	}()

	restoredEntry := staleEntry.DeepCopy()
	restoredEntry.Spec.Manifest.RemoteConfig.StaticOAuthRequired = true
	require.NoError(t, storageClient.Update(t.Context(), restoredEntry))
	releaseCatalogMutationLock()
	require.NoError(t, <-cleanupDone)

	_, err = gateway.RevealCredential(t.Context(), []string{system.MCPOAuthCredentialName(staleEntry.Name)}, "oauth")
	require.NoError(t, err)
	var retainedGrants int64
	require.NoError(t, services.DB.DB.Model(&gatewaytypes.MCPOAuthToken{}).
		Where("catalog_entry_name = ?", staleEntry.Name).Count(&retainedGrants).Error)
	require.EqualValues(t, 1, retainedGrants)
}

func TestEnsureOAuthCredentialStatusFailsClosedForIncompleteCredential(t *testing.T) {
	entry := newMCPServerCatalogEntry("entry-1", types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL: "https://mcp.example/api", StaticOAuthRequired: true,
		},
	})
	entry.Status.OAuthCredentialConfigured = true
	storageClient := newFakeClient(entry)
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, storageClient, nil, nil, nil, nil, time.Hour, 10, 0, 0, false)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })
	require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPOAuthCredentialName(entry.Name), Name: "oauth",
		Secrets: map[string]string{
			"CLIENT_ID": "client", "MCP_URL": entry.Spec.Manifest.RemoteConfig.FixedURL, "GENERATION": "generation-1",
		},
	}))

	req := router.Request{Client: storageClient, Ctx: t.Context(), Object: entry, Namespace: entry.Namespace, Name: entry.Name}
	require.NoError(t, NewHandler(gateway).EnsureOAuthCredentialStatus(req, &router.ResponseWrapper{}))

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, storageClient.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &updated))
	require.False(t, updated.Status.OAuthCredentialConfigured)
}

func TestDetectCompositeDriftIgnoresCatalogOnlyComponentFields(t *testing.T) {
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:    "Catalog Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{{
				CatalogEntryID: "component-entry",
				Manifest:       componentSnapshot,
			}},
		},
	})
	compositeEntry.Status.NeedsUpdate = true
	componentEntry := newMCPServerCatalogEntry("component-entry", types.MCPServerCatalogEntryManifest{
		EntryKey:       "catalog-only-entry-key",
		Name:           "Catalog Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeSingleUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	client := newFakeClient(compositeEntry, componentEntry)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.False(t, updated.Status.NeedsUpdate)
}

func TestDetectCompositeDriftIgnoresAdminAddedSecretBindings(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "admin-secret", Key: "api-key", AdminAdded: true}
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:           "Shared Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
		Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
			Key:       "API_KEY",
			Name:      "API Key",
			Required:  true,
			Sensitive: true,
		}}},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{{
				MCPServerID: "shared-server",
				Manifest:    componentSnapshot,
			}},
		},
	})
	compositeEntry.Status.NeedsUpdate = true
	sharedServer := newMCPServer("shared-server", types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
		Env: []types.MCPEnv{{MCPHeader: types.MCPHeader{
			Key:           "API_KEY",
			Name:          "API Key",
			Required:      true,
			Sensitive:     true,
			SecretBinding: binding,
		}}},
	})
	client := newFakeClient(compositeEntry, sharedServer)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.False(t, updated.Status.NeedsUpdate)
}

func TestDetectCompositeDriftClearsEntryWhenMultiUserComponentMatches(t *testing.T) {
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:           "Shared Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{
				{
					MCPServerID: "shared-server",
					Manifest:    componentSnapshot,
				},
			},
		},
	})
	compositeEntry.Status.NeedsUpdate = true
	sharedServer := newMCPServer("shared-server", types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	client := newFakeClient(compositeEntry, sharedServer)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.False(t, updated.Status.NeedsUpdate)
}

func newFakeClient(objects ...kclient.Object) kclient.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPServerCatalogEntry{}).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			server := obj.(*v1.MCPServer)
			if server.Spec.MCPServerCatalogEntryName == "" {
				return nil
			}
			return []string{server.Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			instance := obj.(*v1.MCPServerInstance)
			if instance.Spec.MCPServerCatalogEntryName == "" {
				return nil
			}
			return []string{instance.Spec.MCPServerCatalogEntryName}
		}).
		WithObjects(objects...).
		Build()
}

func newMCPServerCatalogEntry(name string, manifest types.MCPServerCatalogEntryManifest) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "MCPServerCatalogEntry",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: manifest,
		},
	}
}

func TestEnsureUserCountMultiUserEntry(t *testing.T) {
	entry := newMCPServerCatalogEntry("multi-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Multi User Template",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	server1 := newMCPServer("server-1", types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
		},
	})
	server1.Spec.MCPServerCatalogEntryName = entry.Name
	server1.Spec.UserID = "admin1"
	server1.Status.MCPServerInstanceUserCount = new(2)

	server2 := newMCPServer("server-2", types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
		},
	})
	server2.Spec.MCPServerCatalogEntryName = entry.Name
	server2.Spec.UserID = "admin2"
	server2.Status.MCPServerInstanceUserCount = new(1)

	client := newFakeClient(entry, server1, server2)
	err := (&Handler{}).EnsureUserCount(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &updated))
	assert.Equal(t, 3, updated.Status.UserCount, "should sum server instance user counts across servers")
}

func TestEnsureUserCountMultiUserEntryExcludesComposite(t *testing.T) {
	entry := newMCPServerCatalogEntry("multi-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Multi User Template",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	activeServer := newMCPServer("active-server", types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
		},
	})
	activeServer.Spec.MCPServerCatalogEntryName = entry.Name
	activeServer.Spec.UserID = "admin1"
	activeServer.Status.MCPServerInstanceUserCount = new(1)

	compositeChild := newMCPServer("composite-child", types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
		},
	})
	compositeChild.Spec.MCPServerCatalogEntryName = entry.Name
	compositeChild.Spec.UserID = "admin2"
	compositeChild.Spec.CompositeName = "parent-composite"
	compositeChild.Status.MCPServerInstanceUserCount = new(1)

	client := newFakeClient(entry, activeServer, compositeChild)
	err := (&Handler{}).EnsureUserCount(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &updated))
	assert.Equal(t, 1, updated.Status.UserCount, "should only count active non-composite servers")
}

func TestEnsureUserCountSingleUserEntryCountsUniqueServerUsers(t *testing.T) {
	entry := newMCPServerCatalogEntry("single-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Single User Template",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeSingleUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	server1 := newMCPServer("server-1", types.MCPServerManifest{Runtime: types.RuntimeContainerized})
	server1.Spec.MCPServerCatalogEntryName = entry.Name
	server1.Spec.UserID = "user1"

	server2 := newMCPServer("server-2", types.MCPServerManifest{Runtime: types.RuntimeContainerized})
	server2.Spec.MCPServerCatalogEntryName = entry.Name
	server2.Spec.UserID = "user1"

	server3 := newMCPServer("server-3", types.MCPServerManifest{Runtime: types.RuntimeContainerized})
	server3.Spec.MCPServerCatalogEntryName = entry.Name
	server3.Spec.UserID = "user2"

	client := newFakeClient(entry, server1, server2, server3)
	err := (&Handler{}).EnsureUserCount(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &updated))
	assert.Equal(t, 2, updated.Status.UserCount, "should only count active non-composite server")
}

func newMCPServer(name string, manifest types.MCPServerManifest) *v1.MCPServer {
	return &v1.MCPServer{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "MCPServer",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1.MCPServerSpec{
			Manifest: manifest,
		},
	}
}
