package mcpservercatalogentry

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDetectCompositeDriftStampsComponentNameAndIcon(t *testing.T) {
	compositeEntry := newCompositeCatalogEntry("composite-entry", types.CatalogComponentServer{CatalogEntryID: "component-entry"})
	componentEntry := newMCPServerCatalogEntry("component-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Catalog Component",
		Icon:           "https://example.com/component.svg",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeSingleUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	client := newFakeClient(compositeEntry, componentEntry)
	require.NoError(t, detectCompositeDrift(t, client, compositeEntry))

	updated := getEntry(t, client, compositeEntry)
	assert.False(t, updated.Status.NeedsUpdate)
	require.Len(t, updated.Status.Components, 1)
	assert.Equal(t, "component-entry", updated.Status.Components[0].CatalogEntryID)
	assert.Equal(t, "Catalog Component", updated.Status.Components[0].Name)
	assert.Equal(t, "https://example.com/component.svg", updated.Status.Components[0].Icon)
	assert.False(t, updated.Status.Components[0].Missing)
	assert.False(t, updated.Status.Components[0].ToolOverridesStale)
}

func TestDetectCompositeDriftKeepsStampedNameAndIconWhenUpstreamMissing(t *testing.T) {
	compositeEntry := newCompositeCatalogEntry("composite-entry", types.CatalogComponentServer{CatalogEntryID: "deleted-entry"})
	compositeEntry.Status.Components = []v1.CatalogComponentServerStatus{{CatalogEntryID: "deleted-entry", Name: "GitHub", Icon: "https://example.com/gh.svg"}}

	client := newFakeClient(compositeEntry)
	require.NoError(t, detectCompositeDrift(t, client, compositeEntry))

	updated := getEntry(t, client, compositeEntry)
	assert.True(t, updated.Status.NeedsUpdate, "a dangling reference needs the administrator's attention")
	require.Len(t, updated.Status.Components, 1)
	assert.True(t, updated.Status.Components[0].Missing)
	assert.Equal(t, "GitHub", updated.Status.Components[0].Name, "the stamped name survives so the component does not render as a bare ID")
	assert.Equal(t, "https://example.com/gh.svg", updated.Status.Components[0].Icon)
}

func TestDetectCompositeDriftMarksToolOverridesStaleWhenMultiUserComponentRuntimeMoves(t *testing.T) {
	sharedServer := newMCPServer("shared-server", types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:2.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})
	compositeEntry := newCompositeCatalogEntry("composite-entry", types.CatalogComponentServer{MCPServerID: "shared-server", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}, // Captured against example/component:1.0.0, which the server has since moved off of.
		SourceDigest: mcp.RuntimeIdentityDigest(types.MCPServerCatalogEntryManifest{
			Runtime: types.RuntimeContainerized,
			ContainerizedConfig: &types.ContainerizedRuntimeConfig{
				Image: "example/component:1.0.0",
				Port:  8080,
				Path:  "/mcp",
			},
		})})

	client := newFakeClient(compositeEntry, sharedServer)
	require.NoError(t, detectCompositeDrift(t, client, compositeEntry))

	updated := getEntry(t, client, compositeEntry)
	assert.True(t, updated.Status.NeedsUpdate)
	require.Len(t, updated.Status.Components, 1)
	assert.True(t, updated.Status.Components[0].ToolOverridesStale)
	assert.False(t, updated.Status.Components[0].Missing)
	assert.Equal(t, "Shared Component", updated.Status.Components[0].Name)
}

func TestDetectCompositeDriftIgnoresUpstreamDescriptionChange(t *testing.T) {
	componentManifest := types.MCPServerCatalogEntryManifest{
		Name:           "Catalog Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeSingleUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	compositeEntry := newCompositeCatalogEntry("composite-entry", types.CatalogComponentServer{CatalogEntryID: "component-entry", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}, SourceDigest: mcp.RuntimeIdentityDigest(componentManifest)})
	compositeEntry.Status.NeedsUpdate = true

	// None of these can change which tools the component serves.
	editedManifest := componentManifest
	editedManifest.EntryKey = "catalog-only-entry-key"
	editedManifest.Description = "a fresh description"
	editedManifest.ToolPreview = []types.MCPServerTool{{Name: "create_issue"}}
	componentEntry := newMCPServerCatalogEntry("component-entry", editedManifest)

	client := newFakeClient(compositeEntry, componentEntry)
	require.NoError(t, detectCompositeDrift(t, client, compositeEntry))

	updated := getEntry(t, client, compositeEntry)
	assert.False(t, updated.Status.NeedsUpdate)
	require.Len(t, updated.Status.Components, 1)
	assert.False(t, updated.Status.Components[0].ToolOverridesStale)
}

func TestDetectCompositeDriftIgnoresAdminAddedSecretBindings(t *testing.T) {
	componentManifest := types.MCPServerCatalogEntryManifest{
		Name:           "Shared Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
		Env: []types.MCPEnv{{
			Key:       "API_KEY",
			Name:      "API Key",
			Required:  true,
			Sensitive: true}},
	}
	compositeEntry := newCompositeCatalogEntry("composite-entry", types.CatalogComponentServer{MCPServerID: "shared-server", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}, SourceDigest: mcp.RuntimeIdentityDigest(componentManifest)})
	compositeEntry.Status.NeedsUpdate = true
	// The digest covers env keys, never their values.
	sharedServer := newMCPServer("shared-server", types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
		Env: []types.MCPEnv{{
			Key:           "API_KEY",
			Name:          "API Key",
			Required:      true,
			Sensitive:     true,
			SecretBinding: &types.MCPSecretBinding{Name: "admin-secret", Key: "api-key", AdminAdded: true}}},
	})

	client := newFakeClient(compositeEntry, sharedServer)
	require.NoError(t, detectCompositeDrift(t, client, compositeEntry))

	updated := getEntry(t, client, compositeEntry)
	assert.False(t, updated.Status.NeedsUpdate)
	require.Len(t, updated.Status.Components, 1)
	assert.False(t, updated.Status.Components[0].ToolOverridesStale)
}

func TestDetectCompositeDriftClearsEntryWhenMultiUserComponentMatches(t *testing.T) {
	serverManifest := types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	sharedServer := newMCPServer("shared-server", serverManifest)
	compositeEntry := newCompositeCatalogEntry("composite-entry", types.CatalogComponentServer{MCPServerID: "shared-server", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}, SourceDigest: mcp.RuntimeIdentityDigest(serverManifest.ConvertToCatalogEntry())})
	compositeEntry.Status.NeedsUpdate = true
	compositeEntry.Status.Components = []v1.CatalogComponentServerStatus{{MCPServerID: "shared-server", ToolOverridesStale: true}}

	client := newFakeClient(compositeEntry, sharedServer)
	require.NoError(t, detectCompositeDrift(t, client, compositeEntry))

	updated := getEntry(t, client, compositeEntry)
	assert.False(t, updated.Status.NeedsUpdate)
	require.Len(t, updated.Status.Components, 1)
	assert.False(t, updated.Status.Components[0].ToolOverridesStale)
}

func TestDetectCompositeDriftNeedsUpdateWhenAnyComponentIsStaleOrMissing(t *testing.T) {
	healthyManifest := types.MCPServerCatalogEntryManifest{
		Name:           "Healthy",
		Runtime:        types.RuntimeNPX,
		ServerUserType: types.ServerUserTypeSingleUser,
		NPXConfig:      &types.NPXRuntimeConfig{Package: "@example/healthy@1.0.0"},
	}
	staleManifest := types.MCPServerCatalogEntryManifest{
		Name:           "Stale",
		Runtime:        types.RuntimeNPX,
		ServerUserType: types.ServerUserTypeSingleUser,
		NPXConfig:      &types.NPXRuntimeConfig{Package: "@example/stale@2.0.0"},
	}
	compositeEntry := newCompositeCatalogEntry("composite-entry",
		types.CatalogComponentServer{CatalogEntryID: "healthy-entry", ToolOverrides: []types.ToolOverride{{Name: "a", Enabled: true}}, SourceDigest: mcp.RuntimeIdentityDigest(healthyManifest)},
		types.CatalogComponentServer{CatalogEntryID: "stale-entry", ToolOverrides: []types.ToolOverride{{Name: "b", Enabled: true}}, SourceDigest: "captured-against-an-older-package"},
		types.CatalogComponentServer{CatalogEntryID: "missing-entry"},
	)

	client := newFakeClient(compositeEntry,
		newMCPServerCatalogEntry("healthy-entry", healthyManifest),
		newMCPServerCatalogEntry("stale-entry", staleManifest),
	)
	require.NoError(t, detectCompositeDrift(t, client, compositeEntry))

	updated := getEntry(t, client, compositeEntry)
	assert.True(t, updated.Status.NeedsUpdate)
	require.Len(t, updated.Status.Components, 3)
	assert.False(t, updated.Status.Components[0].ToolOverridesStale)
	assert.False(t, updated.Status.Components[0].Missing)
	assert.True(t, updated.Status.Components[1].ToolOverridesStale)
	assert.False(t, updated.Status.Components[1].Missing)
	assert.False(t, updated.Status.Components[2].ToolOverridesStale, "a component with no overrides is never stale")
	assert.True(t, updated.Status.Components[2].Missing)
}

func TestDetectCompositeDriftClearsNonCompositeEntry(t *testing.T) {
	entry := newMCPServerCatalogEntry("npx-entry", types.MCPServerCatalogEntryManifest{
		Name:           "NPX",
		Runtime:        types.RuntimeNPX,
		ServerUserType: types.ServerUserTypeSingleUser,
		NPXConfig:      &types.NPXRuntimeConfig{Package: "@example/npx@1.0.0"},
	})
	entry.Status.NeedsUpdate = true
	entry.Status.Components = []v1.CatalogComponentServerStatus{{CatalogEntryID: "left-over", Missing: true}}

	client := newFakeClient(entry)
	require.NoError(t, detectCompositeDrift(t, client, entry))

	updated := getEntry(t, client, entry)
	assert.False(t, updated.Status.NeedsUpdate)
	assert.Nil(t, updated.Status.Components)
}

func TestUpdateManifestHashAndLastUpdatedIgnoresComponentSourceDigest(t *testing.T) {
	compositeEntry := newCompositeCatalogEntry("composite-entry", types.CatalogComponentServer{CatalogEntryID: "component-entry", ToolPrefix: "gh", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}, SourceDigest: "digest-one"})

	client := newFakeClient(compositeEntry)
	require.NoError(t, updateManifestHash(t, client, compositeEntry))

	stamped := getEntry(t, client, compositeEntry)
	require.NotEmpty(t, stamped.Status.ManifestHash)
	require.NotNil(t, stamped.Status.LastUpdated)

	// Regenerating identical tool overrides only restamps the digest, which is not configuration.
	stamped.Spec.Manifest.CompositeConfig.ComponentServers[0].SourceDigest = "digest-two"
	require.NoError(t, client.Update(t.Context(), &stamped))
	require.NoError(t, updateManifestHash(t, client, &stamped))

	afterDigest := getEntry(t, client, compositeEntry)
	assert.Equal(t, stamped.Status.ManifestHash, afterDigest.Status.ManifestHash)
	assert.Equal(t, stamped.Status.LastUpdated, afterDigest.Status.LastUpdated)

	// A change to what the composite owns still moves both.
	afterDigest.Spec.Manifest.CompositeConfig.ComponentServers[0].ToolPrefix = "github"
	require.NoError(t, client.Update(t.Context(), &afterDigest))
	require.NoError(t, updateManifestHash(t, client, &afterDigest))

	afterPrefix := getEntry(t, client, compositeEntry)
	assert.NotEqual(t, stamped.Status.ManifestHash, afterPrefix.Status.ManifestHash)
}

func detectCompositeDrift(t *testing.T, client kclient.WithWatch, entry *v1.MCPServerCatalogEntry) error {
	t.Helper()
	return (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, &router.ResponseWrapper{})
}

func updateManifestHash(t *testing.T, client kclient.WithWatch, entry *v1.MCPServerCatalogEntry) error {
	t.Helper()
	return (&Handler{}).UpdateManifestHashAndLastUpdated(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, &router.ResponseWrapper{})
}

func getEntry(t *testing.T, client kclient.WithWatch, entry *v1.MCPServerCatalogEntry) v1.MCPServerCatalogEntry {
	t.Helper()
	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &updated))
	return updated
}

func newCompositeCatalogEntry(name string, components ...types.CatalogComponentServer) *v1.MCPServerCatalogEntry {
	return newMCPServerCatalogEntry(name, types.MCPServerCatalogEntryManifest{
		Name:            "Composite Entry",
		Runtime:         types.RuntimeComposite,
		ServerUserType:  types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: components},
	})
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
		WithObjects(objects...).
		Build()
}

func newMCPServerCatalogEntry(name string, manifest types.MCPServerCatalogEntryManifest) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		APIVersion: v1.SchemeGroupVersion.String(),
		Kind:       "MCPServerCatalogEntry",
		Name:       name,
		Namespace:  "default",
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
		APIVersion: v1.SchemeGroupVersion.String(),
		Kind:       "MCPServer",
		Name:       name,
		Namespace:  "default",
		Spec: v1.MCPServerSpec{
			Manifest: manifest,
		},
	}
}
