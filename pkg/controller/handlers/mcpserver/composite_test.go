package mcpserver

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestCompositeConfigHasDrifted(t *testing.T) {
	tests := []struct {
		name         string
		serverConfig *types.CompositeRuntimeConfig
		entryConfig  *types.CompositeCatalogConfig
		want         bool
	}{
		{
			name:         "both empty",
			serverConfig: nil,
			entryConfig:  nil,
		},
		{
			name:         "only the server has a config",
			serverConfig: &types.CompositeRuntimeConfig{},
			entryConfig:  nil,
			want:         true,
		},
		{
			name: "identical membership",
			serverConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{
				{CatalogEntryID: "gh", ToolPrefix: "gh", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}},
				{MCPServerID: "slack", ToolPrefix: "slack"},
			}},
			entryConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
				{CatalogEntryID: "gh", ToolPrefix: "gh", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}},
				{MCPServerID: "slack", ToolPrefix: "slack"},
			}},
		},
		{
			name: "component added to the entry",
			serverConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{
				{CatalogEntryID: "gh"},
			}},
			entryConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
				{CatalogEntryID: "gh"},
				{CatalogEntryID: "jira"},
			}},
			want: true,
		},
		{
			name: "component replaced in the entry",
			serverConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{
				{CatalogEntryID: "gh"},
			}},
			entryConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
				{CatalogEntryID: "jira"},
			}},
			want: true,
		},
		{
			name: "tool prefix changed",
			serverConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{
				{CatalogEntryID: "gh", ToolPrefix: "gh"},
			}},
			entryConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
				{CatalogEntryID: "gh", ToolPrefix: "github"},
			}},
			want: true,
		},
		{
			name: "tool overrides changed",
			serverConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{
				{CatalogEntryID: "gh", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}},
			}},
			entryConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
				{CatalogEntryID: "gh", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: false}}},
			}},
			want: true,
		},
		{
			name: "only disabled differs",
			serverConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{
				{CatalogEntryID: "gh", ToolPrefix: "gh", Disabled: true},
			}},
			entryConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
				{CatalogEntryID: "gh", ToolPrefix: "gh"},
			}},
		},
		{
			name: "only the entry's source digest differs",
			serverConfig: &types.CompositeRuntimeConfig{ComponentServers: []types.ComponentServer{
				{CatalogEntryID: "gh", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}},
			}},
			entryConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
				{CatalogEntryID: "gh", ToolOverrides: []types.ToolOverride{{Name: "create_issue", Enabled: true}}, SourceDigest: "a-digest"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, compositeConfigHasDrifted(tt.serverConfig, tt.entryConfig))
		})
	}
}

func TestEnsureCompositeComponentsCreatesServersAndInstances(t *testing.T) {
	entry := newMCPServerCatalogEntry("gh-entry", npxCatalogManifest("GitHub", "@example/github@1.0.0"))
	sharedServer := newMCPServer("shared-server")
	sharedServer.Spec.MCPCatalogID = "default"
	sharedServer.Spec.Manifest = types.MCPServerManifest{
		Name:            "Slack",
		Runtime:         types.RuntimeContainerized,
		MultiUserConfig: &types.MultiUserConfig{UserDefinedHeaders: []types.MCPHeader{{Key: "API_KEY"}}},
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/slack:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	composite := newCompositeServer("composite",
		types.ComponentServer{CatalogEntryID: "gh-entry", ToolPrefix: "gh"},
		types.ComponentServer{MCPServerID: "shared-server", ToolPrefix: "slack"},
	)

	client := newFakeClient(t, entry, sharedServer, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	componentServers := listComponentServers(t, client, "composite")
	require.Len(t, componentServers, 1)
	component := componentServers[0]
	assert.Equal(t, "gh-entry", component.Spec.MCPServerCatalogEntryName)
	assert.Equal(t, "composite", component.Spec.CompositeName)
	assert.Equal(t, "user-1", component.Spec.UserID)
	assert.Contains(t, component.Finalizers, v1.MCPServerFinalizer)
	assert.False(t, component.Spec.NeedsURL)
	require.NotNil(t, component.Spec.Manifest.NPXConfig)
	assert.Equal(t, "@example/github@1.0.0", component.Spec.Manifest.NPXConfig.Package)

	instances := listComponentInstances(t, client, "composite")
	require.Len(t, instances, 1)
	assert.Equal(t, "shared-server", instances[0].Spec.MCPServerName)
	assert.Equal(t, "composite", instances[0].Spec.CompositeName)
	assert.Equal(t, "user-1", instances[0].Spec.UserID)
	assert.Equal(t, "default", instances[0].Spec.MCPCatalogName)
	assert.Equal(t, sharedServer.Spec.Manifest.MultiUserConfig, instances[0].Spec.MultiUserConfig)

	updated := getServer(t, client, "composite")
	assert.Equal(t, updated.Generation, updated.Status.ObservedCompositeGeneration)
	assert.Empty(t, updated.Status.ComponentErrors)
}

func TestEnsureCompositeComponentsNeedsURLForHostnameConstrainedComponent(t *testing.T) {
	entry := newMCPServerCatalogEntry("jira-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Jira",
		Runtime:        types.RuntimeRemote,
		ServerUserType: types.ServerUserTypeSingleUser,
		RemoteConfig:   &types.RemoteCatalogConfig{Hostname: "*.atlassian.net"},
	})
	composite := newCompositeServer("composite", types.ComponentServer{CatalogEntryID: "jira-entry", ToolPrefix: "jira"})

	client := newFakeClient(t, entry, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	componentServers := listComponentServers(t, client, "composite")
	require.Len(t, componentServers, 1)
	assert.True(t, componentServers[0].Spec.NeedsURL, "the user supplies the URL when they configure the composite")
	assert.Empty(t, getServer(t, client, "composite").Status.ComponentErrors)
}

func TestEnsureCompositeComponentsLeavesExistingComponentAloneWithoutUpdateRequest(t *testing.T) {
	entry := newMCPServerCatalogEntry("gh-entry", npxCatalogManifest("GitHub", "@example/github@2.0.0"))
	existing := newComponentServer("component-server", "composite", "gh-entry", types.MCPServerManifest{
		Name:      "GitHub",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "@example/github@1.0.0"},
	})
	composite := newCompositeServer("composite", types.ComponentServer{CatalogEntryID: "gh-entry"})

	client := newFakeClient(t, entry, existing, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	componentServers := listComponentServers(t, client, "composite")
	require.Len(t, componentServers, 1)
	require.NotNil(t, componentServers[0].Spec.Manifest.NPXConfig)
	assert.Equal(t, "@example/github@1.0.0", componentServers[0].Spec.Manifest.NPXConfig.Package)
}

func TestEnsureCompositeComponentsObservesSyncRequest(t *testing.T) {
	entryManifest := npxCatalogManifest("GitHub", "@example/github@1.0.0")
	entry := newMCPServerCatalogEntry("gh-entry", entryManifest)
	currentManifest, err := types.MapCatalogEntryToServer(entryManifest, "", false)
	require.NoError(t, err)

	existing := newComponentServer("component-server", "composite", "gh-entry", currentManifest)
	composite := newCompositeServer("composite", types.ComponentServer{CatalogEntryID: "gh-entry"})
	composite.Annotations = map[string]string{v1.MCPServerCompositeSyncRequestedAtAnnotation: "request-1"}

	client := newFakeClient(t, entry, existing, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	updated := getServer(t, client, "composite")
	assert.Equal(t, updated.Generation, updated.Status.ObservedCompositeGeneration)
	assert.Equal(t, "request-1", updated.Status.ObservedCompositeSyncRequest)
	assert.Equal(t, "request-1", updated.Annotations[v1.MCPServerCompositeSyncRequestedAtAnnotation])

	updated.Annotations[v1.MCPServerCompositeSyncRequestedAtAnnotation] = "request-2"
	require.NoError(t, client.Update(t.Context(), &updated))
	require.NoError(t, ensureCompositeComponents(t, client, &updated))

	assert.Equal(t, "request-2", getServer(t, client, "composite").Status.ObservedCompositeSyncRequest)
}

func TestEnsureCompositeComponentsSkipsUnresolvableReference(t *testing.T) {
	entry := newMCPServerCatalogEntry("gh-entry", npxCatalogManifest("GitHub", "@example/github@1.0.0"))
	// Deleting the component server would run the MCPServerFinalizer and destroy its credentials,
	// tokens, and volume, so an unresolvable reference leaves it running.
	orphaned := newComponentServer("orphaned-server", "composite", "deleted-entry", types.MCPServerManifest{
		Name:      "Gone",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "@example/gone@1.0.0"},
	})
	composite := newCompositeServer("composite",
		types.ComponentServer{CatalogEntryID: "deleted-entry"},
		types.ComponentServer{CatalogEntryID: "gh-entry"},
	)
	composite.Annotations = map[string]string{v1.MCPServerCompositeSyncRequestedAtAnnotation: "request-1"}

	client := newFakeClient(t, entry, orphaned, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	names := make([]string, 0, 2)
	for _, componentServer := range listComponentServers(t, client, "composite") {
		names = append(names, componentServer.Spec.MCPServerCatalogEntryName)
	}
	assert.ElementsMatch(t, []string{"deleted-entry", "gh-entry"}, names, "the sibling is created and the orphan keeps serving")

	updated := getServer(t, client, "composite")
	assert.Empty(t, updated.Status.ComponentErrors)
	assert.Equal(t, updated.Generation, updated.Status.ObservedCompositeGeneration)
	assert.Equal(t, "request-1", updated.Status.ObservedCompositeSyncRequest)
}

func TestEnsureCompositeComponentsRecordsValidationFailureAgainstOneComponent(t *testing.T) {
	healthy := newMCPServerCatalogEntry("gh-entry", npxCatalogManifest("GitHub", "@example/github@1.0.0"))
	broken := newMCPServerCatalogEntry("jira-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Jira",
		Runtime:        types.RuntimeRemote,
		ServerUserType: types.ServerUserTypeSingleUser,
		RemoteConfig:   &types.RemoteCatalogConfig{Hostname: "api.atlassian.net", TunnelName: "corp-vpn"},
	})
	composite := newCompositeServer("composite",
		types.ComponentServer{CatalogEntryID: "gh-entry"},
		types.ComponentServer{CatalogEntryID: "jira-entry"},
	)

	client := newFakeClient(t, healthy, broken, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	componentServers := listComponentServers(t, client, "composite")
	require.Len(t, componentServers, 1)
	assert.Equal(t, "gh-entry", componentServers[0].Spec.MCPServerCatalogEntryName, "the sibling is unaffected")

	updated := getServer(t, client, "composite")
	require.Len(t, updated.Status.ComponentErrors, 1)
	assert.Contains(t, updated.Status.ComponentErrors["jira-entry"], "validation failed")
	assert.Equal(t, updated.Generation, updated.Status.ObservedCompositeGeneration)
}

func TestEnsureCompositeComponentsRefusesComponentThatChangedKind(t *testing.T) {
	healthy := newMCPServerCatalogEntry("gh-entry", npxCatalogManifest("GitHub", "@example/github@1.0.0"))
	// The write paths reject both of these, so only drift after authoring reaches the controller.
	nested := newMCPServerCatalogEntry("nested-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Nested",
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
	})
	multiUser := newMCPServerCatalogEntry("shared-entry", npxCatalogManifest("Shared", "@example/shared@1.0.0"))
	multiUser.Spec.Manifest.ServerUserType = types.ServerUserTypeMultiUser
	composite := newCompositeServer("composite",
		types.ComponentServer{CatalogEntryID: "gh-entry"},
		types.ComponentServer{CatalogEntryID: "nested-entry"},
		types.ComponentServer{CatalogEntryID: "shared-entry"},
	)

	client := newFakeClient(t, healthy, nested, multiUser, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	componentServers := listComponentServers(t, client, "composite")
	require.Len(t, componentServers, 1)
	assert.Equal(t, "gh-entry", componentServers[0].Spec.MCPServerCatalogEntryName, "the sibling is unaffected")

	updated := getServer(t, client, "composite")
	require.Len(t, updated.Status.ComponentErrors, 2)
	assert.Contains(t, updated.Status.ComponentErrors["nested-entry"], "composite servers cannot be nested")
	assert.Contains(t, updated.Status.ComponentErrors["shared-entry"], "reference the multi-user server instead")
}

func TestEnsureCompositeComponentsDeduplicatesComponentServersSharingAReference(t *testing.T) {
	entry := newMCPServerCatalogEntry("gh-entry", npxCatalogManifest("GitHub", "@example/github@1.0.0"))
	manifest := types.MCPServerManifest{
		Name:      "GitHub",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "@example/github@1.0.0"},
	}
	oldest := newComponentServer("oldest", "composite", "gh-entry", manifest)
	oldest.CreationTimestamp = metav1.NewTime(metav1.Now().Add(-time.Hour))
	newer := newComponentServer("newer", "composite", "gh-entry", manifest)
	newer.CreationTimestamp = metav1.Now()
	composite := newCompositeServer("composite", types.ComponentServer{CatalogEntryID: "gh-entry"})

	client := newFakeClient(t, entry, oldest, newer, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	// Leaving the duplicate behind would register the component twice in the nanobot config, since
	// CompositeServerToServerConfig iterates every server matching spec.compositeName.
	componentServers := listComponentServers(t, client, "composite")
	require.Len(t, componentServers, 1)
	assert.Equal(t, "oldest", componentServers[0].Name)
	assert.Equal(t, getServer(t, client, "composite").Generation, getServer(t, client, "composite").Status.ObservedCompositeGeneration)
}

func TestEnsureCompositeComponentsDeletesMembersRemovedFromTheManifest(t *testing.T) {
	entry := newMCPServerCatalogEntry("gh-entry", npxCatalogManifest("GitHub", "@example/github@1.0.0"))
	removedServer := newComponentServer("removed-server", "composite", "dropped-entry", types.MCPServerManifest{
		Name:      "Dropped",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "@example/dropped@1.0.0"},
	})
	removedInstance := &v1.MCPServerInstance{
		Name: "removed-instance", Namespace: "default",
		Spec: v1.MCPServerInstanceSpec{
			MCPServerName: "dropped-server",
			CompositeName: "composite",
			UserID:        "user-1",
		},
	}
	composite := newCompositeServer("composite", types.ComponentServer{CatalogEntryID: "gh-entry"})

	client := newFakeClient(t, entry, removedServer, removedInstance, composite)
	require.NoError(t, ensureCompositeComponents(t, client, composite))

	componentServers := listComponentServers(t, client, "composite")
	require.Len(t, componentServers, 1)
	assert.Equal(t, "gh-entry", componentServers[0].Spec.MCPServerCatalogEntryName)
	assert.Empty(t, listComponentInstances(t, client, "composite"))
}

func TestEnsureCompositeComponentsIgnoresNonComposite(t *testing.T) {
	server := newMCPServer("npx-server")
	server.Spec.Manifest = types.MCPServerManifest{
		Name:      "NPX",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "@example/npx@1.0.0"},
	}

	client := newFakeClient(t, server)
	require.NoError(t, ensureCompositeComponents(t, client, server))

	updated := getServer(t, client, "npx-server")
	assert.Zero(t, updated.Status.ObservedCompositeGeneration)
	assert.Empty(t, updated.Status.ComponentErrors)
}

func TestDetectDriftMarksComponentServerNeedingUpdate(t *testing.T) {
	entry := newMCPServerCatalogEntry("gh-entry", npxCatalogManifest("GitHub", "@example/github@2.0.0"))
	// A component server carries its own catalog entry, so it detects its own drift.
	component := newComponentServer("component-server", "composite", "gh-entry", types.MCPServerManifest{
		Name:      "GitHub",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "@example/github@1.0.0"},
	})

	client := newFakeClient(t, entry, component)
	require.NoError(t, detectDrift(t, client, component))

	assert.True(t, getServer(t, client, "component-server").Status.NeedsUpdate)
}

func TestDetectDriftRollsUpComponentServerDriftToComposite(t *testing.T) {
	entry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Engineering",
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "gh-entry", ToolPrefix: "gh"},
		}},
	})
	composite := newCompositeServer("composite", types.ComponentServer{CatalogEntryID: "gh-entry", ToolPrefix: "gh"})
	composite.Spec.MCPServerCatalogEntryName = "composite-entry"
	component := newComponentServer("component-server", "composite", "gh-entry", types.MCPServerManifest{
		Name:      "GitHub",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "@example/github@1.0.0"},
	})
	component.Status.NeedsUpdate = true

	client := newFakeClient(t, entry, composite, component)
	require.NoError(t, detectDrift(t, client, composite))

	// List responses drive the update badge but carry no per-component detail, so this is stored.
	assert.True(t, getServer(t, client, "composite").Status.NeedsUpdate)
}

func TestDetectDriftLeavesCompositeAloneWhenNoComponentHasDrifted(t *testing.T) {
	entry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Engineering",
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "gh-entry", ToolPrefix: "gh"},
		}},
	})
	composite := newCompositeServer("composite", types.ComponentServer{CatalogEntryID: "gh-entry", ToolPrefix: "gh"})
	composite.Spec.MCPServerCatalogEntryName = "composite-entry"
	composite.Status.NeedsUpdate = true
	component := newComponentServer("component-server", "composite", "gh-entry", types.MCPServerManifest{
		Name:      "GitHub",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "@example/github@1.0.0"},
	})

	client := newFakeClient(t, entry, composite, component)
	require.NoError(t, detectDrift(t, client, composite))

	assert.False(t, getServer(t, client, "composite").Status.NeedsUpdate)
}

func detectDrift(t *testing.T, client kclient.WithWatch, server *v1.MCPServer) error {
	t.Helper()
	return (&Handler{}).DetectDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}, &router.ResponseWrapper{})
}

func ensureCompositeComponents(t *testing.T, client kclient.WithWatch, server *v1.MCPServer) error {
	t.Helper()
	return (&Handler{mcpSessionManager: &mcp.SessionManager{}}).EnsureCompositeComponents(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    server,
		Namespace: server.Namespace,
		Name:      server.Name,
	}, &router.ResponseWrapper{})
}

func newCompositeServer(name string, components ...types.ComponentServer) *v1.MCPServer {
	server := newMCPServer(name)
	server.Spec.UserID = "user-1"
	server.Spec.Manifest = types.MCPServerManifest{
		Name:            "Engineering",
		Runtime:         types.RuntimeComposite,
		CompositeConfig: &types.CompositeRuntimeConfig{ComponentServers: components},
	}
	return server
}

func newComponentServer(name, compositeName, catalogEntryName string, manifest types.MCPServerManifest) *v1.MCPServer {
	server := newMCPServer(name)
	server.Spec.UserID = "user-1"
	server.Spec.CompositeName = compositeName
	server.Spec.MCPServerCatalogEntryName = catalogEntryName
	server.Spec.Manifest = manifest
	return server
}

func npxCatalogManifest(name, pkg string) types.MCPServerCatalogEntryManifest {
	return types.MCPServerCatalogEntryManifest{
		Name:           name,
		Runtime:        types.RuntimeNPX,
		ServerUserType: types.ServerUserTypeSingleUser,
		NPXConfig:      &types.NPXRuntimeConfig{Package: pkg},
	}
}

func listComponentServers(t *testing.T, client kclient.WithWatch, compositeName string) []v1.MCPServer {
	t.Helper()
	var servers v1.MCPServerList
	require.NoError(t, client.List(t.Context(), &servers, kclient.MatchingFields{"spec.compositeName": compositeName}))

	alive := make([]v1.MCPServer, 0, len(servers.Items))
	for _, server := range servers.Items {
		if server.DeletionTimestamp.IsZero() {
			alive = append(alive, server)
		}
	}
	return alive
}

func listComponentInstances(t *testing.T, client kclient.WithWatch, compositeName string) []v1.MCPServerInstance {
	t.Helper()
	var instances v1.MCPServerInstanceList
	require.NoError(t, client.List(t.Context(), &instances, kclient.MatchingFields{"spec.compositeName": compositeName}))

	alive := make([]v1.MCPServerInstance, 0, len(instances.Items))
	for _, instance := range instances.Items {
		if instance.DeletionTimestamp.IsZero() {
			alive = append(alive, instance)
		}
	}
	return alive
}

func getServer(t *testing.T, client kclient.WithWatch, name string) v1.MCPServer {
	t.Helper()
	var server v1.MCPServer
	require.NoError(t, client.Get(t.Context(), router.Key("default", name), &server))
	return server
}
