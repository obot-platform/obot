package mcpcatalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestVMCPNameSupportsLocalCatalogPaths(t *testing.T) {
	got := vmcpName("default", "/Users/thedadams/code/vmcp-catalog", "Email")
	assert.Empty(t, kvalidation.IsDNS1123Subdomain(got))
	assert.NotEqual(t, got, vmcpName("default", "/Users/thedadams/code/other-vmcp-catalog", "Email"))
}

func TestVMCPNameDistinguishesCollidingSanitizedNames(t *testing.T) {
	const source = "https://example.com/catalog"
	first := vmcpName("default", source, "Engineering Tools")
	second := vmcpName("default", source, "Engineering-Tools")
	assert.NotEqual(t, first, second)
	assert.Equal(t, first, vmcpName("default", source, "Engineering Tools"))
	assert.Empty(t, kvalidation.IsDNS1123Subdomain(first))
	assert.Empty(t, kvalidation.IsDNS1123Subdomain(second))
}

func TestVMCPNameSanitizesDisplayNames(t *testing.T) {
	for _, displayName := range []string{
		"OpenSearch (staging)",
		"Tools/API_v2.example!",
		"日本語 🔎",
		"()...___",
		strings.Repeat("OpenSearch (staging) ", 30),
	} {
		t.Run(displayName, func(t *testing.T) {
			got := vmcpName("default", "https://example.com/vmcps", displayName)
			assert.Empty(t, kvalidation.IsDNS1123Subdomain(got))
			assert.Equal(t, got, vmcpName("default", "https://example.com/vmcps", displayName))
		})
	}
	assert.NotEqual(t,
		vmcpName("default", "https://example.com/vmcps", "OpenSearch (staging)"),
		vmcpName("default", "https://example.com/vmcps", "OpenSearch staging"),
	)
}

func TestSyncVMCPDisplayNameAndCreationErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		createErr error
	}{
		{
			name: "punctuation in display name",
		},
		{
			name:      "creation error appears in sync status",
			createErr: errors.New("vMCP storage rejected creation"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "vmcp.yaml"), []byte(`- displayName: OpenSearch (staging)
  components:
    - name: opensearch
      entryKey: https://example.com/catalog::opensearch
`), 0o600))
			entry := managedVMCPEntry("https://example.com/catalog", "opensearch")
			catalog := &v1.VMCPCatalog{
				Name:      "catalog",
				Namespace: "default",
				Spec:      v1.VMCPCatalogSpec{SourceURLs: []string{dir}},
			}
			client := interceptor.NewClient(newVMCPFakeClient(catalog, &entry), interceptor.Funcs{
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					if _, ok := obj.(*v1.VMCP); ok {
						require.Empty(t, kvalidation.IsDNS1123Subdomain(obj.GetName()))
						if tc.createErr != nil {
							return tc.createErr
						}
					}
					return c.Create(ctx, obj, opts...)
				},
			})
			h := &Handler{gatewayClient: newVMCPTestGateway(t)}
			var current v1.VMCPCatalog
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
			require.NoError(t, h.SyncVMCP(router.Request{Ctx: t.Context(), Client: client, Object: &current}, &router.ResponseWrapper{}))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
			assert.False(t, current.Status.IsSyncing)
			var definitions v1.VMCPList
			require.NoError(t, client.List(t.Context(), &definitions, kclient.InNamespace(catalog.Namespace)))
			if tc.createErr != nil {
				assert.Contains(t, current.Status.SyncErrors[dir], tc.createErr.Error())
				assert.Contains(t, current.Status.SyncErrors[dir], "OpenSearch (staging)")
				assert.Empty(t, definitions.Items)
				return
			}
			require.Empty(t, current.Status.SyncErrors)
			require.Len(t, definitions.Items, 1)
			assert.Equal(t, "OpenSearch (staging)", definitions.Items[0].Spec.Manifest.DisplayName)
			assert.Contains(t, definitions.Items[0].Name, "opensearch-staging")
		})
	}
}

func TestSetUpDefaultVMCPCatalog(t *testing.T) {
	t.Run("creates trimmed comma-delimited sources", func(t *testing.T) {
		client := newVMCPFakeClient()
		require.NoError(t, (&Handler{}).SetUpDefaultVMCPCatalog(t.Context(), client, " first, , second ,"))

		var catalog v1.VMCPCatalog
		require.NoError(t, client.Get(t.Context(), router.Key(system.DefaultNamespace, system.DefaultCatalog), &catalog))
		assert.Equal(t, []string{"first", "second"}, catalog.Spec.SourceURLs)
	})

	t.Run("empty configuration creates an empty catalog", func(t *testing.T) {
		client := newVMCPFakeClient()
		require.NoError(t, (&Handler{}).SetUpDefaultVMCPCatalog(t.Context(), client, " , "))

		var catalog v1.VMCPCatalog
		require.NoError(t, client.Get(t.Context(), router.Key(system.DefaultNamespace, system.DefaultCatalog), &catalog))
		assert.Empty(t, catalog.Spec.SourceURLs)
	})

	t.Run("existing catalog is untouched", func(t *testing.T) {
		existing := &v1.VMCPCatalog{Name: system.DefaultCatalog, Namespace: system.DefaultNamespace, Spec: v1.VMCPCatalogSpec{SourceURLs: []string{"kept"}}}
		client := newVMCPFakeClient(existing)
		require.NoError(t, (&Handler{}).SetUpDefaultVMCPCatalog(t.Context(), client, "new"))

		var catalog v1.VMCPCatalog
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(existing), &catalog))
		assert.Equal(t, []string{"kept"}, catalog.Spec.SourceURLs)
	})
}

func TestSetUpDefaultMCPCatalogOnlyMigratesLegacyURL(t *testing.T) {
	t.Run("preserves configured source", func(t *testing.T) {
		existing := &v1.MCPCatalog{Name: system.DefaultCatalog, Namespace: system.DefaultNamespace, Spec: v1.MCPCatalogSpec{SourceURLs: []string{"kept"}}}
		client := newVMCPFakeClient(existing)
		h := &Handler{defaultCatalogPath: "new"}
		require.NoError(t, h.SetUpDefaultMCPCatalog(t.Context(), client))

		var catalog v1.MCPCatalog
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(existing), &catalog))
		assert.Equal(t, []string{"kept"}, catalog.Spec.SourceURLs)
	})

	t.Run("migrates the legacy default", func(t *testing.T) {
		existing := &v1.MCPCatalog{Name: system.DefaultCatalog, Namespace: system.DefaultNamespace, Spec: v1.MCPCatalogSpec{SourceURLs: []string{"https://github.com/obot-platform/mcp-catalog"}}}
		client := newVMCPFakeClient(existing)
		h := &Handler{defaultCatalogPath: "new"}
		require.NoError(t, h.SetUpDefaultMCPCatalog(t.Context(), client))

		var catalog v1.MCPCatalog
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(existing), &catalog))
		assert.Equal(t, []string{"new"}, catalog.Spec.SourceURLs)
	})
}

func TestResolveVMCPDefinition(t *testing.T) {
	sourceURL := "https://example.test/catalog"
	entryKey := sourceURL + "::weather"
	entry := managedVMCPEntry(sourceURL, "weather")
	entry.Spec.Manifest.Runtime = types.RuntimeRemote
	entry.Spec.Manifest.RemoteConfig = &types.RemoteCatalogConfig{StaticOAuthRequired: true}
	entry.Spec.UnsupportedTools = []string{"unsupported"}
	source := types.VMCPSourceManifest{
		DisplayName: "Weather tools",
		Components: []types.VMCPSourceComponent{{
			Name:            "weather",
			EntryKey:        entryKey,
			ForceSingleUser: true,
			Configuration:   []types.VMCPConfigurationPolicy{{Key: "missing", Policy: types.VMCPConfigurationPolicyFixed, Value: "secret"}},
			ToolOverrides:   []types.ToolOverride{{Name: "gone", Enabled: true}},
		}},
		Profiles: []types.VMCPProfile{{
			Name:     "everyone",
			Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			Permissions: types.VMCPProfilePermissions{
				AllowedComponents: map[string]types.VMCPComponentSet{entryKey: {AllowedTools: []string{"missing-tool"}}},
			},
		}},
	}

	manifest, err := resolveVMCPDefinition(source, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)
	require.Len(t, manifest.Components, 1)
	component := manifest.Components[0]
	assert.Equal(t, utils.Digest(entryKey), component.ID)
	assert.Equal(t, entry.Name, component.MCPServerCatalogEntryID)
	assert.Equal(t, entry.Spec.MCPCatalogName, component.MCPCatalogID)
	assert.Equal(t, entry.Spec.UnsupportedTools, component.CatalogEntry.UnsupportedTools)
	assert.Equal(t, system.MCPOAuthCredentialName(entry.Name), component.OAuthCredentialID)
	assert.Empty(t, component.CatalogEntry.Manifest.ToolPreview)
	assert.Equal(t, map[string]types.VMCPComponentSet{component.ID: {AllowedTools: []string{"missing-tool"}}}, manifest.Profiles[0].Permissions.AllowedComponents)
	assert.Contains(t, source.Profiles[0].Permissions.AllowedComponents, entryKey)
	assert.Equal(t, types.VMCPConfigurationPolicyFixed, component.Configuration[0].Policy)
	assert.True(t, component.ForceSingleUser)

	source.Components[0].ToolOverrides = nil
	source.Components[0].Name = ""
	manifest, err = resolveVMCPDefinition(source, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)
	assert.Equal(t, entry.Name, manifest.Components[0].Name)
	assert.Equal(t, map[string]types.VMCPComponentSet{component.ID: {AllowedTools: []string{"missing-tool"}}}, manifest.Profiles[0].Permissions.AllowedComponents)
}

func TestResolveVMCPDefinitionRejectsBadReferences(t *testing.T) {
	entry := managedVMCPEntry("source", "entry")
	for _, tc := range []struct {
		name     string
		source   types.VMCPSourceManifest
		entries  []v1.MCPServerCatalogEntry
		contains string
	}{
		{
			name: "unqualified component key",
			source: types.VMCPSourceManifest{
				DisplayName: "test",
				Components:  []types.VMCPSourceComponent{{Name: "one", EntryKey: "entry"}},
			},
			entries:  []v1.MCPServerCatalogEntry{entry},
			contains: "must have the form",
		},
		{
			name: "unresolved component key",
			source: types.VMCPSourceManifest{
				DisplayName: "test",
				Components:  []types.VMCPSourceComponent{{Name: "one", EntryKey: "source::missing"}},
			},
			entries:  []v1.MCPServerCatalogEntry{entry},
			contains: "cannot be resolved",
		},
		{
			name: "ambiguous component key",
			source: types.VMCPSourceManifest{
				DisplayName: "test",
				Components:  []types.VMCPSourceComponent{{Name: "one", EntryKey: "source::entry"}},
			},
			entries:  []v1.MCPServerCatalogEntry{entry, entry},
			contains: "ambiguous",
		},
		{
			name: "profile unknown source key",
			source: types.VMCPSourceManifest{
				DisplayName: "test",
				Components:  []types.VMCPSourceComponent{{Name: "one", EntryKey: "source::entry"}},
				Profiles: []types.VMCPProfile{{
					Name:     "all",
					Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
					Permissions: types.VMCPProfilePermissions{
						AllowedComponents: map[string]types.VMCPComponentSet{"source::other": {AllowedTools: []string{"tool"}}},
					},
				}},
			},
			entries:  []v1.MCPServerCatalogEntry{entry},
			contains: "unknown component entryKey",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveVMCPDefinition(tc.source, tc.entries)
			require.ErrorContains(t, err, tc.contains)
		})
	}
}

func TestReadVMCPManifestsStrictlyRejectsCatalogEntriesAndVMCPEntryKey(t *testing.T) {
	for _, content := range []string{
		"- entryKey: catalog-entry\n  name: Not a vMCP\n",
		"- displayName: vMCP\n  entryKey: not-allowed\n",
	} {
		file := filepath.Join(t.TempDir(), "catalog.yaml")
		require.NoError(t, os.WriteFile(file, []byte(content), 0o600))
		definitions, err := readCatalogManifests[types.VMCPSourceManifest](t.Context(), nil, file, "")
		require.Error(t, err)
		assert.Empty(t, definitions)
	}
	file := filepath.Join(t.TempDir(), "catalog.yaml")
	require.NoError(t, os.WriteFile(file, []byte("- displayName: vMCP\n  components: []\n"), 0o600))
	entries, err := readCatalogManifests[types.MCPServerCatalogEntryManifest](t.Context(), nil, file, "")
	require.Error(t, err)
	assert.Empty(t, entries)
}

func TestRemoveVMCPCatalogCleansUpSourceTokens(t *testing.T) {
	catalog := &v1.VMCPCatalog{Name: "catalog", Namespace: "default"}
	client := newVMCPFakeClient(catalog)
	gateway := newVMCPTestGateway(t)
	credentialContext := VMCPCatalogCredentialContext(catalog.Name)
	secrets := map[string]string{"https://example.com/catalog": "token"}
	for _, context := range []string{credentialContext, catalog.Name} {
		require.NoError(t, gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
			Context: context,
			Name:    CatalogCredentialToolName,
			Secrets: secrets,
		}))
	}
	h := &Handler{gatewayClient: gateway}
	reconcileErr := errors.New("failed to list definitions")
	failingClient := interceptor.NewClient(client, interceptor.Funcs{
		List: func(context.Context, kclient.WithWatch, kclient.ObjectList, ...kclient.ListOption) error {
			return reconcileErr
		},
	})
	require.ErrorIs(t, h.RemoveVMCPCatalog(router.Request{Ctx: t.Context(), Client: failingClient, Object: catalog}, nil), reconcileErr)
	credential, err := gateway.RevealCredential(t.Context(), []string{credentialContext}, CatalogCredentialToolName)
	require.NoError(t, err)
	assert.Equal(t, secrets, credential.Secrets)

	for range 2 {
		require.NoError(t, h.RemoveVMCPCatalog(router.Request{Ctx: t.Context(), Client: client, Object: catalog}, nil))
		_, err := gateway.RevealCredential(t.Context(), []string{credentialContext}, CatalogCredentialToolName)
		var notFound gatewayclient.CredentialNotFoundError
		require.ErrorAs(t, err, &notFound)
	}
	credential, err = gateway.RevealCredential(t.Context(), []string{catalog.Name}, CatalogCredentialToolName)
	require.NoError(t, err)
	assert.Equal(t, secrets, credential.Secrets)
}

func TestReconcileRemovedVMCPs(t *testing.T) {
	catalog := &v1.VMCPCatalog{Name: "catalog", Namespace: "default"}
	managed := managedVMCP(catalog, "managed")
	referenced := managedVMCP(catalog, "referenced")
	detached := managedVMCP(catalog, "detached")
	detached.Spec.Detached = true
	ordinary := &v1.VMCP{Name: "ordinary", Namespace: catalog.Namespace}
	instance := &v1.VMCPInstance{Name: "instance", Namespace: catalog.Namespace, Spec: v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: referenced.Name}}}
	client := newVMCPFakeClient(managed, referenced, detached, ordinary, instance)

	require.NoError(t, reconcileRemovedVMCPs(t.Context(), client, catalog, nil))

	var deleted v1.VMCP
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(managed), &deleted)))
	var converted v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(referenced), &converted))
	assert.True(t, converted.Spec.Detached)
	assert.False(t, converted.IsGitManaged())
	for _, preserved := range []*v1.VMCP{detached, ordinary} {
		var got v1.VMCP
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(preserved), &got))
		assert.Equal(t, preserved.Spec, got.Spec)
	}

	require.NoError(t, reconcileRemovedVMCPs(t.Context(), client, catalog, nil))
	var stillDetached v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(referenced), &stillDetached))
}

func TestSyncVMCPDefinitionPreservesSnapshotsUntilSourceChanges(t *testing.T) {
	catalog := &v1.VMCPCatalog{Name: "catalog", Namespace: "default"}
	entry := managedVMCPEntry("source", "entry")
	entry.Spec.Manifest.Name = "original snapshot"
	client := newVMCPFakeClient()
	gateway := newVMCPTestGateway(t)
	h := &Handler{gatewayClient: gateway}
	definition := types.VMCPSourceManifest{
		DisplayName: "vMCP",
		Components:  []types.VMCPSourceComponent{{Name: "component", EntryKey: "source::entry"}},
	}

	_, err := h.syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)
	entry.Spec.Manifest.Name = "drifted catalog snapshot"
	_, err = h.syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)

	var unchanged v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{Namespace: catalog.Namespace, Name: "vmcp"}, &unchanged))
	assert.Equal(t, "original snapshot", unchanged.Spec.Manifest.Components[0].CatalogEntry.Manifest.Name)

	definition.Description = "changed source definition"
	_, err = h.syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)
	var refreshed v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&unchanged), &refreshed))
	assert.Equal(t, "drifted catalog snapshot", refreshed.Spec.Manifest.Components[0].CatalogEntry.Manifest.Name)
}

func TestSyncVMCPDefinitionExtractsFixedConfiguration(t *testing.T) {
	catalog := &v1.VMCPCatalog{Name: "catalog", Namespace: "default"}
	entry := managedVMCPEntry("source", "entry")
	client := newVMCPFakeClient()
	gateway := newVMCPTestGateway(t)
	definition := types.VMCPSourceManifest{
		DisplayName: "vMCP",
		Components: []types.VMCPSourceComponent{{
			Name: "component", EntryKey: "source::entry",
			Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed, Value: "secret"}},
		}},
	}

	_, err := (&Handler{gatewayClient: gateway}).syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)
	var stored v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{Namespace: catalog.Namespace, Name: "vmcp"}, &stored))
	component := stored.Spec.Manifest.Components[0]
	assert.Empty(t, component.Configuration[0].Value)
	assert.Equal(t, utils.Digest(map[string]string{vmcpconfig.ConfigurationKey(component.ID, "TOKEN"): "secret"}), stored.Spec.StaticConfigurationHash)
	credential, err := gateway.RevealCredential(t.Context(), []string{vmcpconfig.StaticConfigurationCredentialContext(stored.Name)}, vmcpconfig.ConfigurationCredentialName())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{vmcpconfig.ConfigurationKey(component.ID, "TOKEN"): "secret"}, credential.Secrets)
}

func TestSyncVMCPDefinitionFiltersCatalogOwnedConfiguration(t *testing.T) {
	for _, policy := range []types.VMCPConfigurationPolicyType{
		types.VMCPConfigurationPolicyUserAllowed,
		types.VMCPConfigurationPolicyFixed,
	} {
		t.Run(string(policy), func(t *testing.T) {
			catalog := &v1.VMCPCatalog{Name: "catalog", Namespace: "default"}
			entry := managedVMCPEntry("source", "entry")
			entry.Spec.Manifest.Config = []types.MCPConfig{
				{Key: "LITERAL", Value: "catalog-value"},
				{Key: "BOUND", SecretBinding: &types.MCPSecretBinding{Name: "config", Key: "token"}},
				{Key: "USER_INPUT"},
				{Key: "FIXED_INPUT"},
			}
			var value string
			if policy == types.VMCPConfigurationPolicyFixed {
				value = "source-override"
			}

			definition := types.VMCPSourceManifest{
				DisplayName: "vMCP",
				Components: []types.VMCPSourceComponent{{
					Name:     "component",
					EntryKey: "source::entry",
					Configuration: []types.VMCPConfigurationPolicy{
						{Key: "LITERAL", Policy: policy, Value: value},
						{Key: "BOUND", Policy: policy, Value: value},
						{Key: "USER_INPUT", Policy: types.VMCPConfigurationPolicyUserAllowed},
						{Key: "FIXED_INPUT", Policy: types.VMCPConfigurationPolicyFixed, Value: "fixed-secret"},
					},
				}},
			}
			original := definition.DeepCopy()
			client := newVMCPFakeClient()
			gateway := newVMCPTestGateway(t)
			h := &Handler{gatewayClient: gateway}
			_, err := h.syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
			require.NoError(t, err)
			assert.Equal(t, *original, definition)

			var stored v1.VMCP
			require.NoError(t, client.Get(t.Context(), router.Key(catalog.Namespace, "vmcp"), &stored))
			require.Len(t, stored.Spec.Manifest.Components, 1)
			component := stored.Spec.Manifest.Components[0]
			assert.Equal(t, []types.VMCPConfigurationPolicy{
				{Key: "USER_INPUT", Policy: types.VMCPConfigurationPolicyUserAllowed},
				{Key: "FIXED_INPUT", Policy: types.VMCPConfigurationPolicyFixed},
			}, component.Configuration)
			assert.Equal(t, entry.Spec.Manifest.Config, component.CatalogEntry.Manifest.Config)
			assert.Equal(t, utils.Digest(definition), stored.Spec.SourceDigest)

			credential, err := gateway.RevealCredential(t.Context(), []string{vmcpconfig.StaticConfigurationCredentialContext(stored.Name)}, vmcpconfig.ConfigurationCredentialName())
			require.NoError(t, err)
			assert.Equal(t, map[string]string{vmcpconfig.ConfigurationKey(component.ID, "FIXED_INPUT"): "fixed-secret"}, credential.Secrets)
		})
	}
}

func TestSyncVMCPDefinitionErrorDoesNotOverwriteCurrentDefinition(t *testing.T) {
	catalog := &v1.VMCPCatalog{Name: "catalog", Namespace: "default"}
	entry := managedVMCPEntry("source", "entry")
	client := newVMCPFakeClient()
	h := &Handler{gatewayClient: newVMCPTestGateway(t)}
	definition := types.VMCPSourceManifest{DisplayName: "vMCP", Components: []types.VMCPSourceComponent{{Name: "component", EntryKey: "source::entry"}}}
	_, err := h.syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)
	var before v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{Namespace: catalog.Namespace, Name: "vmcp"}, &before))

	definition.Components[0].EntryKey = "source::missing"
	_, err = h.syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
	require.ErrorContains(t, err, "cannot be resolved")
	var after v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&before), &after))
	assert.Equal(t, before.Spec, after.Spec)
}

func TestSyncVMCPDefinitionRollsBackCredentialOnWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing bool
		cancel   bool
		conflict bool
	}{
		{
			name: "failed create removes credential",
		},
		{
			name:     "failed update restores credential",
			existing: true,
		},
		{
			name:     "exhausted conflicts restore credential",
			existing: true,
			conflict: true,
		},
		{
			name:   "canceled create removes credential",
			cancel: true,
		},
		{
			name:     "canceled update restores credential",
			existing: true,
			cancel:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := &v1.VMCPCatalog{Name: "catalog", Namespace: "default"}
			entry := managedVMCPEntry("source", "entry")
			client := newVMCPFakeClient()
			gateway := newVMCPTestGateway(t)
			h := &Handler{gatewayClient: gateway}
			definition := types.VMCPSourceManifest{
				DisplayName: "vMCP",
				Components: []types.VMCPSourceComponent{{
					Name:          "component",
					EntryKey:      "source::entry",
					Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed, Value: "old-secret"}},
				}},
			}
			var before v1.VMCP
			if tc.existing {
				_, err := h.syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
				require.NoError(t, err)
				require.NoError(t, client.Get(t.Context(), router.Key(catalog.Namespace, "vmcp"), &before))
			}

			definition.Components[0].Configuration[0].Value = "new-secret"
			credentialContext := vmcpconfig.StaticConfigurationCredentialContext("vmcp")
			credentialName := vmcpconfig.ConfigurationCredentialName()
			secretKey := vmcpconfig.ConfigurationKey(utils.Digest("source::entry"), "TOKEN")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			writeErr := errors.New("resource write failed")
			if tc.conflict {
				writeErr = apierrors.NewConflict(schema.GroupResource{Group: "obot.obot.ai", Resource: "vmcps"}, "vmcp", writeErr)
			}
			if tc.cancel {
				writeErr = context.Canceled
			}
			failWrite := func() error {
				credential, err := gateway.RevealCredential(t.Context(), []string{credentialContext}, credentialName)
				require.NoError(t, err)
				assert.Equal(t, "new-secret", credential.Secrets[secretKey])
				if tc.cancel {
					cancel()
				}
				return writeErr
			}
			failingClient := interceptor.NewClient(client, interceptor.Funcs{
				Create: func(context.Context, kclient.WithWatch, kclient.Object, ...kclient.CreateOption) error {
					return failWrite()
				},
				Update: func(context.Context, kclient.WithWatch, kclient.Object, ...kclient.UpdateOption) error {
					return failWrite()
				},
			})
			_, err := h.syncVMCPDefinition(ctx, failingClient, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
			require.ErrorIs(t, err, writeErr)

			var after v1.VMCP
			resourceErr := client.Get(t.Context(), router.Key(catalog.Namespace, "vmcp"), &after)
			credential, credentialErr := gateway.RevealCredential(t.Context(), []string{credentialContext}, credentialName)
			if tc.existing {
				require.NoError(t, resourceErr)
				assert.Equal(t, before.Spec, after.Spec)
				require.NoError(t, credentialErr)
				assert.Equal(t, map[string]string{secretKey: "old-secret"}, credential.Secrets)
			} else {
				assert.True(t, apierrors.IsNotFound(resourceErr))
				var notFound gatewayclient.CredentialNotFoundError
				require.ErrorAs(t, credentialErr, &notFound)
			}
		})
	}
}

func TestSyncVMCPDefinitionRetriesConflictingUpdate(t *testing.T) {
	catalog := &v1.VMCPCatalog{Name: "catalog", Namespace: "default"}
	entry := managedVMCPEntry("source", "entry")
	client := newVMCPFakeClient()
	gateway := newVMCPTestGateway(t)
	h := &Handler{gatewayClient: gateway}
	definition := types.VMCPSourceManifest{
		DisplayName: "vMCP",
		Components: []types.VMCPSourceComponent{{
			Name:          "component",
			EntryKey:      "source::entry",
			Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed, Value: "old-secret"}},
		}},
	}
	_, err := h.syncVMCPDefinition(t.Context(), client, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)
	definition.Components[0].Configuration[0].Value = "new-secret"
	attempts := 0
	conflictingClient := interceptor.NewClient(client, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			attempts++
			if attempts == 1 {
				var concurrent v1.VMCP
				require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(obj), &concurrent))
				concurrent.Annotations = map[string]string{"concurrent": "preserved"}
				require.NoError(t, c.Update(ctx, &concurrent))
				return apierrors.NewConflict(schema.GroupResource{Group: "obot.obot.ai", Resource: "vmcps"}, obj.GetName(), errors.New("concurrent update"))
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	_, err = h.syncVMCPDefinition(t.Context(), conflictingClient, catalog, "source", "vmcp", definition, []v1.MCPServerCatalogEntry{entry})
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
	var stored v1.VMCP
	require.NoError(t, client.Get(t.Context(), router.Key(catalog.Namespace, "vmcp"), &stored))
	assert.Equal(t, "preserved", stored.Annotations["concurrent"])
	credential, err := gateway.RevealCredential(t.Context(), []string{vmcpconfig.StaticConfigurationCredentialContext("vmcp")}, vmcpconfig.ConfigurationCredentialName())
	require.NoError(t, err)
	want := map[string]string{vmcpconfig.ConfigurationKey(utils.Digest("source::entry"), "TOKEN"): "new-secret"}
	assert.Equal(t, want, credential.Secrets)
	assert.Equal(t, utils.Digest(want), stored.Spec.StaticConfigurationHash)
	assert.Equal(t, utils.Digest(definition), stored.Spec.SourceDigest)
}

func TestSyncVMCPReportsSourceErrorAndPreservesDefinitions(t *testing.T) {
	dir := t.TempDir()
	entry := managedVMCPEntry(dir, "entry")
	catalog := &v1.VMCPCatalog{
		Name:        "catalog",
		Namespace:   "default",
		Annotations: map[string]string{v1.VMCPCatalogSyncAnnotation: "true"},
		Spec:        v1.VMCPCatalogSpec{SourceURLs: []string{dir}},
	}
	client := newVMCPFakeClient(catalog, &entry)
	h := &Handler{gatewayClient: newVMCPTestGateway(t)}
	file := filepath.Join(dir, "vmcp.yaml")
	require.NoError(t, os.WriteFile(file, []byte("- displayName: vMCP\n  components:\n    - name: component\n      entryKey: "+dir+"::entry\n"), 0o600))
	var initial v1.VMCPCatalog
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &initial))
	require.NoError(t, h.SyncVMCP(router.Request{Ctx: t.Context(), Client: client, Object: &initial}, &router.ResponseWrapper{}))
	var definitions v1.VMCPList
	require.NoError(t, client.List(t.Context(), &definitions, kclient.InNamespace(catalog.Namespace)))
	require.Len(t, definitions.Items, 1)

	require.NoError(t, os.WriteFile(file, []byte("- displayName: invalid\n  unexpected: true\n"), 0o600))
	var current v1.VMCPCatalog
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
	current.Annotations[v1.VMCPCatalogSyncAnnotation] = "true"
	require.NoError(t, client.Update(t.Context(), &current))
	require.NoError(t, h.SyncVMCP(router.Request{Ctx: t.Context(), Client: client, Object: &current}, &router.ResponseWrapper{}))

	var updated v1.VMCPCatalog
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &updated))
	assert.Contains(t, updated.Status.SyncErrors, dir)
	definitions = v1.VMCPList{}
	require.NoError(t, client.List(t.Context(), &definitions, kclient.InNamespace(catalog.Namespace)))
	assert.Len(t, definitions.Items, 1)
}

func TestSyncVMCPRemovalAndRestoration(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "vmcp.yaml")
	content := []byte(`- displayName: referenced
  components:
    - name: component
      entryKey: https://example.com/catalog::entry
- displayName: unused
  components: []
`)
	entry := managedVMCPEntry("https://example.com/catalog", "entry")
	catalog := &v1.VMCPCatalog{
		Name:      "catalog",
		Namespace: "default",
		Spec:      v1.VMCPCatalogSpec{SourceURLs: []string{dir}},
	}
	client := newVMCPFakeClient(catalog, &entry)
	h := &Handler{gatewayClient: newVMCPTestGateway(t)}
	sync := func(content []byte) {
		t.Helper()
		require.NoError(t, os.WriteFile(file, content, 0o600))
		var current v1.VMCPCatalog
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
		current.Annotations = map[string]string{v1.VMCPCatalogSyncAnnotation: "true"}
		require.NoError(t, client.Update(t.Context(), &current))
		require.NoError(t, h.SyncVMCP(router.Request{Ctx: t.Context(), Client: client, Object: &current}, &router.ResponseWrapper{}))
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
		require.Empty(t, current.Status.SyncErrors)
	}

	sync(content)
	referencedKey := router.Key(catalog.Namespace, vmcpName(catalog.Name, dir, "referenced"))
	unusedKey := router.Key(catalog.Namespace, vmcpName(catalog.Name, dir, "unused"))
	var original v1.VMCP
	require.NoError(t, client.Get(t.Context(), referencedKey, &original))
	require.True(t, original.IsGitManaged())
	instance := &v1.VMCPInstance{
		Name:      "instance",
		Namespace: catalog.Namespace,
		Spec:      v1.VMCPInstanceSpec{Manifest: types.VMCPInstanceManifest{VMCPID: original.Name}},
	}
	require.NoError(t, client.Create(t.Context(), instance))

	sync([]byte("[]\n"))
	var detached v1.VMCP
	require.NoError(t, client.Get(t.Context(), referencedKey, &detached))
	assert.True(t, detached.Spec.Detached)
	assert.False(t, detached.IsGitManaged())
	assert.Equal(t, original.Spec.Manifest, detached.Spec.Manifest)
	var unused v1.VMCP
	require.NoError(t, client.Get(t.Context(), unusedKey, &unused))
	require.False(t, unused.DeletionTimestamp.IsZero())
	// The fake client has no cleanup controller to finish finalization.
	unused.Finalizers = nil
	require.NoError(t, client.Update(t.Context(), &unused))
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), unusedKey, &unused)))

	detached.Spec.Manifest.Description = "edited while detached"
	require.NoError(t, client.Update(t.Context(), &detached))
	sync([]byte("[]\n"))
	var preserved v1.VMCP
	require.NoError(t, client.Get(t.Context(), referencedKey, &preserved))
	assert.Equal(t, detached.Spec, preserved.Spec)

	// Restore the identical source, including its original digest.
	sync(content)
	var restored v1.VMCP
	require.NoError(t, client.Get(t.Context(), referencedKey, &restored))
	assert.False(t, restored.Spec.Detached)
	assert.True(t, restored.IsGitManaged())
	assert.Equal(t, original.Spec, restored.Spec)
	var connection v1.VMCPInstance
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &connection))
	assert.Equal(t, instance.Spec, connection.Spec)
	require.NoError(t, client.Get(t.Context(), unusedKey, &unused))
	assert.True(t, unused.DeletionTimestamp.IsZero())
	assert.True(t, unused.IsGitManaged())
}

func TestSyncVMCPYAMLPermissions(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vmcp.yaml"), []byte(`- displayName: Engineering
  components:
    - name: github
      entryKey: https://github.com/example/mcp-catalog::github
  profiles:
    - name: all
      subjects:
        - type: selector
          id: "*"
      vmcpPermissions:
        allowedComponents:
          "https://github.com/example/mcp-catalog::github":
            allowedTools: null
    - name: none
      subjects:
        - type: selector
          id: "*"
      vmcpPermissions:
        allowedComponents:
          "https://github.com/example/mcp-catalog::github":
            allowedTools: []
    - name: selected
      subjects:
        - type: selector
          id: "*"
      vmcpPermissions:
        allowedComponents:
          "https://github.com/example/mcp-catalog::github":
            allowedTools: [list_issues, list_pulls]
`), 0o600))
	entry := managedVMCPEntry("https://github.com/example/mcp-catalog", "github")
	catalog := &v1.VMCPCatalog{
		Name:        "catalog",
		Namespace:   "default",
		Annotations: map[string]string{v1.VMCPCatalogSyncAnnotation: "true"},
		Spec:        v1.VMCPCatalogSpec{SourceURLs: []string{dir}},
	}
	client := newVMCPFakeClient(catalog, &entry)
	h := &Handler{gatewayClient: newVMCPTestGateway(t)}
	var current v1.VMCPCatalog
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
	require.NoError(t, h.SyncVMCP(router.Request{Ctx: t.Context(), Client: client, Object: &current}, &router.ResponseWrapper{}))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), &current))
	require.Empty(t, current.Status.SyncErrors)

	var stored v1.VMCP
	require.NoError(t, client.Get(t.Context(), router.Key(catalog.Namespace, vmcpName(catalog.Name, dir, "Engineering")), &stored))
	require.Len(t, stored.Spec.Manifest.Components, 1)
	component := stored.Spec.Manifest.Components[0]
	assert.Equal(t, entry.Name, component.MCPServerCatalogEntryID)
	assert.Equal(t, utils.Digest("https://github.com/example/mcp-catalog::github"), component.ID)
	require.Len(t, stored.Spec.Manifest.Profiles, 3)
	for i, tools := range [][]string{nil, {}, {"list_issues", "list_pulls"}} {
		assert.Equal(t, map[string]types.VMCPComponentSet{component.ID: {AllowedTools: tools}}, stored.Spec.Manifest.Profiles[i].Permissions.AllowedComponents)
	}
}

func managedVMCPEntry(source, entryKey string) v1.MCPServerCatalogEntry {
	return v1.MCPServerCatalogEntry{
		Name:      "entry",
		Namespace: "default",
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: "mcp-catalog",
			SourceURL:      source,
			Manifest:       types.MCPServerCatalogEntryManifest{EntryKey: entryKey},
		},
	}
}

func managedVMCP(catalog *v1.VMCPCatalog, name string) *v1.VMCP {
	return &v1.VMCP{Name: name, Namespace: catalog.Namespace, Spec: v1.VMCPSpec{VMCPCatalogName: catalog.Name, SourceURL: "source"}}
}

func newVMCPFakeClient(objects ...kclient.Object) kclient.WithWatch {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithStatusSubresource(&v1.VMCPCatalog{}).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string {
			id := obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID
			if id == "" {
				return nil
			}
			return []string{id}
		}).
		WithObjects(objects...).Build()
}

func newVMCPTestGateway(t *testing.T) *gatewayclient.Client {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway
}
