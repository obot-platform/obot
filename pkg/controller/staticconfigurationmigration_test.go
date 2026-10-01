package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// failingVMCPUpdates fails vMCP updates while fail is set, stopping a migration after it has
// migrated entries and servers.
type failingVMCPUpdates struct {
	kclient.Client
	fail bool
}

// failingCredentialUpserts fails writes to one credential context while fail is set.
type failingCredentialUpserts struct {
	mcp.StaticConfigurationStore
	context string
	fail    bool
}

func (c *failingVMCPUpdates) Update(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
	if _, ok := obj.(*v1.VMCP); ok && c.fail {
		return errors.New("injected vMCP update failure")
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (s *failingCredentialUpserts) UpsertCredential(ctx context.Context, credential gatewaytypes.Credential) error {
	if s.fail && credential.Context == s.context {
		return errors.New("injected credential write failure")
	}
	return s.StaticConfigurationStore.UpsertCredential(ctx, credential)
}

func TestMigrateCatalogEntryStaticConfiguration(t *testing.T) {
	staticManifest := func(value string) types.MCPServerCatalogEntryManifest {
		return types.MCPServerCatalogEntryManifest{
			Name:      "Search",
			Runtime:   types.RuntimeNPX,
			NPXConfig: &types.NPXRuntimeConfig{Package: "search"},
			Config: []types.MCPConfig{
				{Key: "API_TOKEN", Value: value, Required: true, Usage: types.Env},
				{Key: "USER", Usage: types.Env},
				{Key: "BOUND", SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "bound"}, Usage: types.Env},
			},
		}
	}

	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry1search",
		Namespace: system.DefaultNamespace,
		Spec:      v1.MCPServerCatalogEntrySpec{Manifest: staticManifest("secret")},
	}
	serverManifest, err := types.MapCatalogEntryToServer(entry.Spec.Manifest, "", false)
	require.NoError(t, err)
	server := &v1.MCPServer{
		Name:      "ms1search",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: entry.Name,
			Manifest:                  serverManifest,
		},
	}

	currentSnapshot := types.MCPServerCatalogEntrySnapshot{Manifest: staticManifest("secret")}
	staleSnapshot := types.MCPServerCatalogEntrySnapshot{Manifest: staticManifest("old")}
	current := types.VMCPComponent{
		ID:                      "current",
		MCPServerCatalogEntryID: entry.Name,
		CatalogEntry:            currentSnapshot,
		SourceDigest:            vmcpconfig.SourceDigest(currentSnapshot),
	}
	stale := types.VMCPComponent{
		ID:                      "stale",
		MCPServerCatalogEntryID: entry.Name,
		CatalogEntry:            staleSnapshot,
		SourceDigest:            vmcpconfig.SourceDigest(staleSnapshot),
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1search",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest:                           types.VMCPManifest{Components: []types.VMCPComponent{current, stale}},
			ComponentStaticConfigurationHashes: map[string]string{"current": "hash"},
		},
	}
	instance := &v1.VMCPInstance{
		Name:      "vi1search",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
			LegacyComponents: []types.VMCPComponent{{
				ID:                      "current",
				MCPServerCatalogEntryID: entry.Name,
				CatalogEntry:            currentSnapshot,
				SourceDigest:            utils.Digest([]any{current, "hash"}),
			}},
		},
	}
	require.Equal(t, instance.Spec.LegacyComponents[0].SourceDigest, utils.Digest([]any{vmcp.Spec.Manifest.Components[0], "hash"}))

	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, server, vmcp, instance).Build()
	gatewayClient := newTestGatewayClient(t)

	require.NoError(t, migrateCatalogEntryStaticConfiguration(t.Context(), client, gatewayClient))
	// A retried migration changes nothing.
	require.NoError(t, migrateCatalogEntryStaticConfiguration(t.Context(), client, gatewayClient))

	get := func(obj kclient.Object) {
		t.Helper()
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj))
	}
	get(entry)
	get(server)
	get(vmcp)
	get(instance)

	assertMigrated := func(config []types.MCPConfig) {
		t.Helper()
		assert.True(t, config[0].Static)
		assert.Empty(t, config[0].Value)
		assert.False(t, config[1].Static)
		assert.False(t, config[2].Static)
		assert.NotNil(t, config[2].SecretBinding)
	}

	revision := entry.Spec.Manifest.StaticConfigurationRevision
	require.NotEmpty(t, revision)
	assertMigrated(entry.Spec.Manifest.Config)
	values, err := mcp.RevealStaticConfiguration(t.Context(), gatewayClient, entry.Name, revision)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"API_TOKEN": "secret"}, values)

	// Copies holding the entry's values share its revision.
	assertMigrated(server.Spec.Manifest.Config)
	assert.Equal(t, revision, server.Spec.Manifest.StaticConfigurationRevision)

	entrySnapshot := types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}
	migratedCurrent, migratedStale := vmcp.Spec.Manifest.Components[0], vmcp.Spec.Manifest.Components[1]
	assertMigrated(migratedCurrent.CatalogEntry.Manifest.Config)
	assert.Equal(t, revision, migratedCurrent.CatalogEntry.Manifest.StaticConfigurationRevision)
	assert.False(t, vmcpconfig.NeedsUpdate(migratedCurrent, entrySnapshot))

	// A snapshot that was out of date keeps its own values and stays out of date.
	assertMigrated(migratedStale.CatalogEntry.Manifest.Config)
	staleRevision := migratedStale.CatalogEntry.Manifest.StaticConfigurationRevision
	assert.NotEqual(t, revision, staleRevision)
	assert.True(t, vmcpconfig.NeedsUpdate(migratedStale, entrySnapshot))
	values, err = mcp.RevealStaticConfiguration(t.Context(), gatewayClient, entry.Name, staleRevision)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"API_TOKEN": "old"}, values)

	// Legacy components stay bound to the migrated component.
	legacy := instance.Spec.LegacyComponents[0]
	assertMigrated(legacy.CatalogEntry.Manifest.Config)
	assert.Equal(t, utils.Digest([]any{migratedCurrent, "hash"}), legacy.SourceDigest)
	assert.Equal(t, revision, vmcpconfig.ComponentsForInstance(*vmcp, *instance)[0].CatalogEntry.Manifest.StaticConfigurationRevision)
}

// Earlier migrations moved some literal values out of copied manifests and into credentials. Those
// copies matched their entry before this migration, and must still match it afterwards.
func TestMigrateCatalogEntryStaticConfigurationHeldInCredentials(t *testing.T) {
	for _, failure := range []string{"", "vMCP update", "server credential scrub"} {
		t.Run(fmt.Sprint("failure=", failure), func(t *testing.T) {
			testMigrateCatalogEntryStaticConfigurationHeldInCredentials(t, failure)
		})
	}
}

// testMigrateCatalogEntryStaticConfigurationHeldInCredentials migrates once, or retries after the
// named step failed; a retry must finish in the same state as a migration that never failed.
func testMigrateCatalogEntryStaticConfigurationHeldInCredentials(t *testing.T, failure string) {
	t.Helper()

	entryManifest := types.MCPServerCatalogEntryManifest{
		Name:      "Search",
		Runtime:   types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{Package: "search"},
		Config:    []types.MCPConfig{{Key: "API_TOKEN", Value: "secret", Required: true, Usage: types.Env}},
	}
	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry1held",
		Namespace: system.DefaultNamespace,
		Spec:      v1.MCPServerCatalogEntrySpec{Manifest: entryManifest},
	}

	// A snapshot without the literal, whose value moved to vMCP configuration.
	blank := types.MCPServerCatalogEntrySnapshot{Manifest: *entryManifest.DeepCopy()}
	blank.Manifest.Config[0].Value = ""
	component := func(id string) types.VMCPComponent {
		return types.VMCPComponent{
			ID:                      id,
			MCPServerCatalogEntryID: entry.Name,
			CatalogEntry:            *blank.DeepCopy(),
			// The digest of the source the snapshot was taken from, before its values were moved.
			SourceDigest:  vmcpconfig.SourceDigest(entrySnapshot(*entry)),
			Configuration: []types.VMCPConfigurationPolicy{{Key: "API_TOKEN", Policy: types.VMCPConfigurationPolicyFixed}},
		}
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1held",
		Namespace: system.DefaultNamespace,
		Spec:      v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{component("matching"), component("override")}}},
	}
	legacy := component("matching")
	legacy.Configuration[0].Policy = types.VMCPConfigurationPolicyUserAllowed
	instance := &v1.VMCPInstance{
		Name:      "vi1held",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest:         types.VMCPInstanceManifest{VMCPID: vmcp.Name},
			LegacyComponents: []types.VMCPComponent{legacy},
		},
	}

	serverManifest, err := types.MapCatalogEntryToServer(blank.Manifest, "", false)
	require.NoError(t, err)
	server := &v1.MCPServer{
		Name:      "ms1held",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: entry.Name,
			MCPCatalogID:              "default",
			Manifest:                  serverManifest,
		},
	}

	gatewayClient := newTestGatewayClient(t)
	for _, credential := range []gatewaytypes.Credential{
		{
			Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
			Name:    vmcpconfig.StaticConfigurationCredentialName(vmcp),
			Secrets: map[string]string{
				vmcpconfig.ConfigurationKey("matching", "API_TOKEN"): "secret",
				vmcpconfig.ConfigurationKey("override", "API_TOKEN"): "override",
			},
		},
		{
			Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
			Name:    vmcpconfig.ConfigurationCredentialName(),
			Secrets: map[string]string{vmcpconfig.ConfigurationKey("matching", "API_TOKEN"): "secret"},
		},
		{
			Context: server.CredentialContext(server.Spec.UserID),
			Name:    server.Name,
			Secrets: map[string]string{"API_TOKEN": "secret"},
		},
	} {
		require.NoError(t, gatewayClient.UpsertCredential(t.Context(), credential))
	}

	client := &failingVMCPUpdates{
		Client: fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, server, vmcp, instance).Build(),
		fail:   failure == "vMCP update",
	}
	store := &failingCredentialUpserts{
		StaticConfigurationStore: gatewayClient,
		context:                  server.CredentialContext(server.Spec.UserID),
		fail:                     failure == "server credential scrub",
	}
	if failure != "" {
		// The retry starts from objects the failed attempt already migrated.
		require.Error(t, migrateCatalogEntryStaticConfiguration(t.Context(), client, store))
		client.fail, store.fail = false, false
	}
	require.NoError(t, migrateCatalogEntryStaticConfiguration(t.Context(), client, store))
	for _, obj := range []kclient.Object{entry, server, vmcp, instance} {
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj))
	}

	revision := entry.Spec.Manifest.StaticConfigurationRevision
	require.NotEmpty(t, revision)
	current := types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}

	// The server's credential value moved to the entry's revision, so the server matches the entry.
	assert.True(t, server.Spec.Manifest.Config[0].Static)
	assert.Equal(t, revision, server.Spec.Manifest.StaticConfigurationRevision)

	matching, override := vmcp.Spec.Manifest.Components[0], vmcp.Spec.Manifest.Components[1]
	assert.True(t, matching.CatalogEntry.Manifest.Config[0].Static)
	assert.Equal(t, revision, matching.CatalogEntry.Manifest.StaticConfigurationRevision)
	assert.Equal(t, current, matching.CatalogEntry)
	assert.False(t, vmcpconfig.NeedsUpdate(matching, current))

	// An override stays with its vMCP policy; the component is still reported as up to date.
	assert.False(t, override.CatalogEntry.Manifest.Config[0].Static)
	assert.Empty(t, override.CatalogEntry.Manifest.StaticConfigurationRevision)
	assert.False(t, vmcpconfig.NeedsUpdate(override, current))

	migratedLegacy := instance.Spec.LegacyComponents[0]
	assert.True(t, migratedLegacy.CatalogEntry.Manifest.Config[0].Static)
	assert.Equal(t, revision, migratedLegacy.CatalogEntry.Manifest.StaticConfigurationRevision)

	// Moved values leave the credentials and policies they were copied into, so they are never
	// reused as configuration if the entry later stops supplying them.
	assert.Empty(t, matching.Configuration)
	assert.Equal(t, component("override").Configuration, override.Configuration)
	assert.Empty(t, migratedLegacy.Configuration)
	reveal := func(credentialContext, name string) map[string]string {
		t.Helper()
		credential, err := gatewayClient.RevealCredential(t.Context(), []string{credentialContext}, name)
		require.NoError(t, err)
		return credential.Secrets
	}
	assert.Empty(t, reveal(server.CredentialContext(server.Spec.UserID), server.Name))
	assert.Equal(t, map[string]string{vmcpconfig.ConfigurationKey("override", "API_TOKEN"): "override"},
		reveal(vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name), vmcpconfig.StaticConfigurationCredentialName(vmcp)))
	assert.Empty(t, reveal(vmcpconfig.InstanceConfigurationCredentialContext(instance.Name), vmcpconfig.ConfigurationCredentialName()))
}
