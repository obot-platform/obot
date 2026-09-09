package compositemigration

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMigrationNamePreservesUUID(t *testing.T) {
	require.Equal(t, "vmcp150241b93-4d53-5183-abcc-f9645245cdd7", migrationName("vmcp1", "default", "catalog"))
}

func TestBuildVMCPSkipsMCPServerCatalogEntryID(t *testing.T) {
	entry := migrationEntry(t)
	var legacy legacyManifest
	require.NoError(t, json.Unmarshal(entry.Spec.LegacyCompositeManifest, &legacy)) //nolint:staticcheck // Exercise the legacy migration input.
	legacy.CompositeConfig.ComponentServers[0].CatalogEntryID = system.MCPServerPrefix + "invalid"

	target, _, err := (&Handler{}).buildVMCP(router.Request{
		Ctx:    t.Context(),
		Client: migrationClient(),
	}, entry, legacy)
	require.NoError(t, err)
	require.Len(t, target.Spec.Manifest.Components, 1)
	require.Equal(t, "remote", target.Spec.Manifest.Components[0].ID)
	require.Equal(t, "remote", target.Spec.Manifest.Components[0].MCPServerCatalogEntryID)
}

func TestMigrateAll(t *testing.T) {
	entry := migrationEntry(t)
	parent := migrationParent(t, "connection")
	entry.Finalizers = []string{"test-cleanup"}
	parent.Finalizers = []string{"test-cleanup"}
	client := migrationClient(entry, parent)
	handler := credentialHandler(t, nil, map[string]map[string]string{})

	require.NoError(t, handler.MigrateAll(t.Context(), client))
	var target v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{
		Namespace: entry.Namespace,
		Name:      migrationName(system.VMCPPrefix, entry.Namespace, entry.Name),
	}, &target))
	var instances v1.VMCPInstanceList
	require.NoError(t, client.List(t.Context(), &instances))
	require.Len(t, instances.Items, 1)
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(parent), parent))
	require.False(t, parent.DeletionTimestamp.IsZero())
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), entry))
	require.False(t, entry.DeletionTimestamp.IsZero())
	// Do not wait for finalizers: their controllers have not started yet.
	require.NoError(t, handler.MigrateAll(t.Context(), client))
}

func TestMigrateAllFailureAndRetry(t *testing.T) {
	entry := migrationEntry(t)
	parent := migrationParent(t, "connection")
	client := migrationClient(entry, parent)
	handler := credentialHandler(t, nil, map[string]map[string]string{})
	upsert := handler.upsert
	handler.upsert = func(context.Context, gatewaytypes.Credential) error {
		return errors.New("credential store unavailable")
	}
	require.ErrorContains(t, handler.MigrateAll(t.Context(), client), "credential store unavailable")
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), entry))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(parent), parent))
	require.True(t, entry.DeletionTimestamp.IsZero())
	require.True(t, parent.DeletionTimestamp.IsZero())

	handler.upsert = upsert
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	require.NoError(t, handler.MigrateAll(t.Context(), client))
}

func TestMigrateAllRejectsOrphanComposite(t *testing.T) {
	client := migrationClient(migrationParent(t, "orphan"))
	require.ErrorContains(t, (&Handler{}).MigrateAll(t.Context(), client), "default/orphan was not migrated")
}

func migrationEntry(t *testing.T) *v1.MCPServerCatalogEntry {
	t.Helper()
	var entry v1.MCPServerCatalogEntry
	require.NoError(t, json.Unmarshal([]byte(`{
		"metadata":{"name":"composite","namespace":"default"},
		"spec":{"mcpCatalogName":"default","manifest":{
			"name":"Composite","runtime":"composite","compositeConfig":{"componentServers":[
				{"catalogEntryID":"local","toolPrefix":"local_","manifest":{"name":"Local","runtime":"npx","npxConfig":{"package":"everything"},"env":[{"key":"TOKEN","sensitive":true},{"key":"FILE","file":true,"dynamicFile":true}]}},
				{"catalogEntryID":"remote","manifest":{"name":"Remote","runtime":"remote","remoteConfig":{"fixedURL":"https://example.com/mcp","headers":[{"key":"Authorization","prefix":"Bearer ","required":true}]}}}
			]}
		}}
	}`), &entry))
	return &entry
}

func migrationParent(t *testing.T, name string) *v1.MCPServer {
	t.Helper()
	var server v1.MCPServer
	require.NoError(t, json.Unmarshal([]byte(`{
		"metadata":{"namespace":"default"},"spec":{"userID":"user1","mcpServerCatalogEntryName":"composite","manifest":{
			"runtime":"composite","compositeConfig":{"componentServers":[
				{"catalogEntryID":"local","toolPrefix":"mine_","toolOverrides":[{"name":"echo","enabled":true}],"manifest":{"name":"Local","runtime":"npx","npxConfig":{"package":"connection-version"},"env":[{"key":"TOKEN","sensitive":true}]}},
				{"catalogEntryID":"remote","disabled":true,"manifest":{"name":"Remote","runtime":"remote","remoteConfig":{"url":"https://example.com/mcp","headers":[{"key":"Authorization","prefix":"Bearer "}]}}}
			]}
		}}
	}`), &server))
	server.Name = name
	return &server
}

func migrationClient(objects ...kclient.Object) kclient.WithWatch {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.CompositeName}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.compositeName", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServerInstance).Spec.CompositeName}
		}).Build()
}

func credentialHandler(t *testing.T, source map[string]map[string]string, destination map[string]map[string]string) *Handler {
	t.Helper()
	return &Handler{
		reveal: func(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
			require.Len(t, contexts, 1)
			values, ok := source[contexts[0]+"/"+name]
			if !ok {
				return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{}
			}
			return gatewaytypes.Credential{Secrets: maps.Clone(values)}, nil
		},
		upsert: func(_ context.Context, credential gatewaytypes.Credential) error {
			require.Equal(t, vmcp.ConfigurationCredentialName(), credential.Name)
			destination[credential.Context] = maps.Clone(credential.Secrets)
			return nil
		},
	}
}

func TestMigrateCompositeConnections(t *testing.T) {
	entry := migrationEntry(t)
	first, second := migrationParent(t, "first"), migrationParent(t, "second")
	objects := []kclient.Object{entry, first, second}
	source := map[string]map[string]string{}
	for _, parent := range []*v1.MCPServer{first, second} {
		child := &v1.MCPServer{
			Name: parent.Name + "-local", Namespace: entry.Namespace,
			Spec: v1.MCPServerSpec{
				UserID:                    "user1",
				CompositeName:             parent.Name,
				MCPServerCatalogEntryName: "local",
			},
		}
		objects = append(objects, child)
		source["user1-"+child.Name+"/"+child.Name] = map[string]string{"TOKEN": parent.Name + "-secret"}
	}
	rule := &v1.AccessControlRule{
		Name: "owners", Namespace: entry.Namespace,
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: "default",
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeGroup, ID: "owners"}},
				Resources: []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}},
			},
		},
	}
	objects = append(objects, rule)
	client := migrationClient(objects...)
	destination := map[string]map[string]string{}
	handler := credentialHandler(t, source, destination)
	req := router.Request{Ctx: t.Context(), Client: client, Object: entry}
	require.NoError(t, handler.Migrate(req, nil))
	var target v1.VMCP
	targetName := migrationName(system.VMCPPrefix, entry.Namespace, entry.Name)
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{Namespace: entry.Namespace, Name: targetName}, &target))
	require.Equal(t, entry.Name, target.Spec.LegacySlug)
	require.Contains(t, target.Finalizers, v1.VMCPFinalizer)
	require.True(t, target.Spec.Manifest.ForceSingleUser)
	require.Equal(t, []types.VMCPProfile{{Name: "owners", Subjects: rule.Spec.Manifest.Subjects, AllowAllTools: true}}, target.Spec.Manifest.Profiles)
	require.Equal(t, "dynamicFile", string(target.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config[1].Usage))
	require.Equal(t, "Bearer ", target.Spec.Manifest.Components[1].CatalogEntry.Manifest.Config[0].Prefix)
	var instances v1.VMCPInstanceList
	require.NoError(t, client.List(t.Context(), &instances))
	require.Len(t, instances.Items, 2)
	require.NotEqual(t, instances.Items[0].Name, instances.Items[1].Name)
	for _, instance := range instances.Items {
		require.Contains(t, instance.Finalizers, v1.VMCPInstanceFinalizer)
		require.Equal(t, "user1", instance.Spec.UserID)
		require.Equal(t, target.Name, instance.Spec.Manifest.VMCPID)
		configuration := destination[vmcp.InstanceConfigurationCredentialContext(instance.Name)]
		require.Equal(t, instance.Spec.LegacySlug+"-secret", configuration[vmcp.ConfigurationKey("local", "TOKEN")])
		require.Equal(t, utils.Digest(configuration), instance.Status.UserConfigurationHash)
		require.Equal(t, "connection-version", instance.Spec.LegacyComponents[0].CatalogEntry.Manifest.NPXConfig.Package)
		require.Equal(t, "mine_", instance.Spec.LegacyComponents[0].ToolPrefix)
		require.Equal(t, []string{"remote"}, instance.Spec.LegacyDisabledComponents)
		require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKey{Namespace: entry.Namespace, Name: instance.Spec.LegacySlug}, &v1.MCPServer{})))
	}
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &v1.MCPServerCatalogEntry{})))

	// A retry after destination creation must not overwrite credentials edited there.
	for _, parent := range []*v1.MCPServer{first, second} {
		parent.ResourceVersion = ""
		require.NoError(t, client.Create(t.Context(), parent))
	}
	handler.upsert = func(context.Context, gatewaytypes.Credential) error {
		t.Fatal("retry overwrote a destination credential")
		return nil
	}
	require.NoError(t, handler.Migrate(req, nil))
}

func TestMigrateRetainsSourceOnCredentialFailure(t *testing.T) {
	for _, failStatic := range []bool{true, false} {
		t.Run(map[bool]string{true: "static", false: "instance"}[failStatic], func(t *testing.T) {
			entry, parent := migrationEntry(t), migrationParent(t, "parent")
			client := migrationClient(entry, parent)
			handler := credentialHandler(t, nil, map[string]map[string]string{})
			writes := 0
			handler.upsert = func(context.Context, gatewaytypes.Credential) error {
				writes++
				if failStatic || writes == 2 {
					return errors.New("credential unavailable")
				}
				return nil
			}
			err := handler.Migrate(router.Request{Ctx: t.Context(), Client: client, Object: entry}, nil)
			require.ErrorContains(t, err, "credential unavailable")
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &v1.MCPServerCatalogEntry{}))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(parent), &v1.MCPServer{}))
			var instances v1.VMCPInstanceList
			require.NoError(t, client.List(t.Context(), &instances))
			require.Empty(t, instances.Items)
		})
	}
}

func TestMigrateNoConnectionsDoesNotGrantWildcard(t *testing.T) {
	entry := migrationEntry(t)
	client := migrationClient(entry)
	handler := credentialHandler(t, nil, map[string]map[string]string{})
	require.NoError(t, handler.Migrate(router.Request{Ctx: t.Context(), Client: client, Object: entry}, nil))
	var targets v1.VMCPList
	require.NoError(t, client.List(t.Context(), &targets))
	require.Len(t, targets.Items, 1)
	require.Empty(t, targets.Items[0].Spec.Manifest.Profiles)
	require.Len(t, targets.Items[0].Spec.Manifest.Components, 2)
}

func TestMigrateRetainsSourceOnOAuthCopyFailure(t *testing.T) {
	entry, parent := migrationEntry(t), migrationParent(t, "parent")
	child := &v1.MCPServer{Name: "child", Namespace: entry.Namespace, Spec: v1.MCPServerSpec{CompositeName: parent.Name, MCPServerCatalogEntryName: "local"}}
	client := migrationClient(entry, parent, child)
	handler := credentialHandler(t, nil, map[string]map[string]string{})
	handler.copyOAuth = func(_ context.Context, user, source, target string) error {
		require.Equal(t, parent.Spec.UserID, user)
		require.Equal(t, child.Name, source)
		require.NotEmpty(t, target)
		return errors.New("OAuth copy failed")
	}
	require.ErrorContains(t, handler.Migrate(router.Request{Ctx: t.Context(), Client: client, Object: entry}, nil), "OAuth copy failed")
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &v1.MCPServerCatalogEntry{}))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(parent), &v1.MCPServer{}))
}

func TestMigrateSharedConfigurationAndUserHeaders(t *testing.T) {
	entry := migrationEntry(t)
	entry.Spec.LegacyCompositeManifest = json.RawMessage(`{"name":"Composite","runtime":"composite","compositeConfig":{"componentServers":[{"mcpServerID":"shared"}]}}`) //nolint:staticcheck // Exercise the legacy migration input.
	parent := migrationParent(t, "parent")
	parent.Spec.Manifest.CompositeConfig.ComponentServers = []types.ComponentServer{{MCPServerID: "shared"}}
	shared := &v1.MCPServer{
		Name: "shared", Namespace: entry.Namespace,
		Spec: v1.MCPServerSpec{
			MCPCatalogID: "default",
			Manifest: types.MCPServerManifest{
				Name:    "Shared",
				Runtime: types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{
					URL: "https://example.com/mcp",
				},
				Config: []types.MCPConfig{
					{Key: "ADMIN_TOKEN", Sensitive: true, Usage: types.Header},
					{Key: "USER_TOKEN", Required: true, Usage: types.Header, UserAllowed: true},
				},
			},
		},
	}
	connection := &v1.MCPServerInstance{
		Name: "shared-connection", Namespace: entry.Namespace,
		Spec: v1.MCPServerInstanceSpec{
			UserID:        "user1",
			CompositeName: parent.Name,
			MCPServerName: shared.Name,
		},
	}
	client := migrationClient(entry, parent, shared, connection)
	destination := map[string]map[string]string{}
	handler := credentialHandler(t, map[string]map[string]string{
		"default-shared/shared":                     {"ADMIN_TOKEN": "admin-secret"},
		"user1-shared-connection/shared-connection": {"USER_TOKEN": "user-secret"},
	}, destination)
	require.NoError(t, handler.Migrate(router.Request{Ctx: t.Context(), Client: client, Object: entry}, nil))
	var target v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{Namespace: entry.Namespace, Name: migrationName(system.VMCPPrefix, entry.Namespace, entry.Name)}, &target))
	require.Equal(t, "https://example.com/mcp", target.Spec.Manifest.Components[0].CatalogEntry.Manifest.RemoteConfig.FixedURL)
	require.Equal(t, []types.VMCPConfigurationPolicy{
		{Key: "ADMIN_TOKEN", Policy: types.VMCPConfigurationPolicyFixed},
		{Key: "USER_TOKEN", Policy: types.VMCPConfigurationPolicyUserAllowed},
	}, target.Spec.Manifest.Components[0].Configuration)
	static := destination[vmcp.StaticConfigurationCredentialContext(target.Name)]
	require.Equal(t, map[string]string{vmcp.ConfigurationKey("shared", "ADMIN_TOKEN"): "admin-secret"}, static)
	require.Equal(t, utils.Digest(static), target.Spec.StaticConfigurationHash)
	require.Equal(t, utils.Digest(map[string]string{"ADMIN_TOKEN": "admin-secret"}), target.Spec.ComponentStaticConfigurationHashes["shared"])
	var instances v1.VMCPInstanceList
	require.NoError(t, client.List(t.Context(), &instances))
	require.Len(t, instances.Items, 1)
	require.Equal(t, map[string]string{vmcp.ConfigurationKey("shared", "USER_TOKEN"): "user-secret"}, destination[vmcp.InstanceConfigurationCredentialContext(instances.Items[0].Name)])
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(shared), &v1.MCPServer{}), "shared source remains available to other consumers")
}
