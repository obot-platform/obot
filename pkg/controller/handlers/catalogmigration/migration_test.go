package catalogmigration

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpwebhookvalidation"
	vmcphandler "github.com/obot-platform/obot/pkg/controller/handlers/vmcp"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func testHandler(t *testing.T, source, destination map[string]map[string]string) *Handler {
	t.Helper()
	return &Handler{
		reveal: func(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
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

func testEntry(id string) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		Name:      id,
		Namespace: "default",
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: "default",
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:         id,
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
				Config: []types.MCPConfig{
					{Key: "TOKEN", Usage: types.Header, Required: true},
					{Key: "DEFAULT", Value: "catalog-default", Usage: types.Header},
				},
			},
		},
	}
}

func testRule(id string, resources ...types.Resource) *v1.AccessControlRule {
	return &v1.AccessControlRule{
		Name:      id,
		Namespace: "default",
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: "default",
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeGroup, ID: id}},
				Resources: resources,
			},
		},
	}
}

func TestMigrateCatalogAndSingleUser(t *testing.T) {
	entry := testEntry("catalog")
	server := &v1.MCPServer{
		Name:      "single",
		Namespace: entry.Namespace,
		Spec: v1.MCPServerSpec{
			UserID:                    "1",
			MCPServerCatalogEntryName: entry.Name,
			Manifest: types.MCPServerManifest{
				Name:         "Original snapshot",
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/personal"},
				Config:       []types.MCPConfig{{Key: "TOKEN", Value: "inline", Usage: types.Header}},
			},
		},
	}
	rule := testRule("readers", types.Resource{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name})
	wrongCatalog := testRule("wrong", types.Resource{Type: types.ResourceTypeSelector, ID: "*"})
	wrongCatalog.Spec.MCPCatalogID = "other"
	filter := &v1.MCPWebhookValidation{Name: "direct-filter", Namespace: entry.Namespace}
	filter.Spec.Manifest.Resources = []types.Resource{{Type: types.ResourceTypeMCPServer, ID: server.Name}}
	client := migrationClientBuilder().WithObjects(entry, server, rule, wrongCatalog, testEntry("unshared"), filter).Build()
	destination := make(map[string]map[string]string)
	handler := testHandler(t, map[string]map[string]string{"1-single/single": {"TOKEN": "secret"}}, destination)
	var oauthTarget string
	handler.copyOAuth = func(_ context.Context, user, source, target string) error {
		require.Equal(t, "1", user)
		require.Equal(t, server.Name, source)
		oauthTarget = target
		return nil
	}
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	var targets v1.VMCPList
	require.NoError(t, client.List(t.Context(), &targets))
	require.Len(t, targets.Items, 1)
	target := targets.Items[0]
	require.Equal(t, entry.Name, target.Spec.LegacySlug)
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), filter))
	require.Equal(t, []types.Resource{{Type: types.ResourceTypeMCPServer, ID: target.Name}}, filter.Spec.Manifest.Resources)
	require.Len(t, target.Spec.Manifest.Profiles, 1)
	require.Equal(t, rule.Spec.Manifest.Subjects, target.Spec.Manifest.Profiles[0].Subjects)
	require.True(t, target.Spec.Manifest.Profiles[0].Permissions.AllowAllComponents)
	require.Equal(t, map[string]string{vmcp.ConfigurationKey("catalog", "DEFAULT"): "catalog-default"}, destination[vmcp.StaticConfigurationCredentialContext(target.Name)])
	var instances v1.VMCPInstanceList
	require.NoError(t, client.List(t.Context(), &instances))
	require.Len(t, instances.Items, 1)
	instance := instances.Items[0]
	require.Equal(t, target.Name, instance.Spec.Manifest.VMCPID)
	require.Equal(t, server.Name, instance.Spec.LegacySlug)
	require.Equal(t, map[string]string{vmcp.ConfigurationKey("catalog", "TOKEN"): "secret"}, destination[vmcp.InstanceConfigurationCredentialContext(instance.Name)])
	components := vmcp.ComponentsForInstance(target, instance)
	require.Equal(t, "https://example.com/personal", components[0].CatalogEntry.Manifest.RemoteConfig.FixedURL)
	require.Empty(t, components[0].CatalogEntry.Manifest.Config[0].Value)
	require.Equal(t, name.SafeConcatName(system.MCPServerPrefix+instance.Name, "catalog"), oauthTarget)
	// Sources survive until the caller records migration completion.
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &v1.MCPServer{}))
	handler.upsert = func(context.Context, gatewaytypes.Credential) error {
		return errors.New("must not overwrite credentials on retry")
	}
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	require.NoError(t, Cleanup(t.Context(), client))
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &v1.MCPServer{})))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &v1.MCPServerCatalogEntry{}))
}

func TestMigrateSharedConfigurationAndFilters(t *testing.T) {
	server := &v1.MCPServer{
		Name:      "shared",
		Namespace: "default",
		Spec: v1.MCPServerSpec{
			MCPCatalogID:              "default",
			MCPServerCatalogEntryName: "catalog",
			UserID:                    "admin",
			Alias:                     "Shared server alias",
			Manifest: types.MCPServerManifest{
				Name:         "Shared server",
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"},
				Config: []types.MCPConfig{
					{Key: "STATIC", Value: "inline", Usage: types.Header},
					{Key: "USER", UserAllowed: true, Usage: types.Header},
				},
			},
		},
	}
	instance := &v1.MCPServerInstance{
		Name:      "connection",
		Namespace: "default",
		Spec: v1.MCPServerInstanceSpec{
			UserID:        "1",
			MCPServerName: server.Name,
			Config:        []types.MCPConfig{{Key: "USER", Value: "inline-user", Usage: types.Header}},
		},
	}
	rule := testRule("shared-users", types.Resource{Type: types.ResourceTypeMCPServer, ID: server.Name})
	filters := []*v1.MCPWebhookValidation{
		{Name: "server", Namespace: "default"},
		{Name: "entry", Namespace: "default"},
		{Name: "catalog", Namespace: "default"},
		{Name: "all", Namespace: "default"},
		{Name: "unrelated", Namespace: "default"},
	}
	resources := []types.Resource{
		{Type: types.ResourceTypeMCPServer, ID: server.Name},
		{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "catalog"},
		{Type: types.ResourceTypeMcpCatalog, ID: "default"},
		{Type: types.ResourceTypeSelector, ID: "*"},
		{Type: types.ResourceTypeMCPServer, ID: "other"},
	}
	objects := []kclient.Object{server, instance, rule}
	for i, filter := range filters {
		filter.Spec.Manifest.Resources = []types.Resource{resources[i]}
		objects = append(objects, filter)
	}
	client := migrationClientBuilder().WithObjects(objects...).Build()
	destination := make(map[string]map[string]string)
	handler := testHandler(t, map[string]map[string]string{
		"default-shared/shared":   {"STATIC": "static-secret"},
		"1-connection/connection": {"USER": "user-secret"},
	}, destination)
	var oauthTarget string
	handler.copyOAuth = func(_ context.Context, user, source, target string) error {
		require.Equal(t, "1", user)
		require.Equal(t, server.Name, source)
		oauthTarget = target
		return nil
	}
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	var target v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{Namespace: "default", Name: migrationName(system.VMCPPrefix, "default", server.Name)}, &target))
	require.Equal(t, "admin", target.Spec.CreatorUserID)
	require.Equal(t, server.Spec.Alias, target.Spec.Manifest.DisplayName)
	require.Equal(t, "Shared server", target.Spec.Manifest.Components[0].Name)
	require.True(t, vmcp.IsMultiUser(target.Spec.Manifest.Components[0]))
	require.Equal(t, map[string]string{vmcp.ConfigurationKey("shared", "STATIC"): "static-secret"}, destination[vmcp.StaticConfigurationCredentialContext(target.Name)])
	require.NotEmpty(t, target.Spec.StaticConfigurationHash)
	var migrated v1.VMCPInstance
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{Namespace: "default", Name: migrationName(system.VMCPInstancePrefix, "default", instance.Name)}, &migrated))
	require.Equal(t, map[string]string{vmcp.ConfigurationKey("shared", "USER"): "user-secret"}, destination[vmcp.InstanceConfigurationCredentialContext(migrated.Name)])
	require.Equal(t, name.SafeConcatName(system.MCPServerPrefix+target.Name, "shared"), oauthTarget)
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	for i, filter := range filters {
		var actual v1.MCPWebhookValidation
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), &actual))
		if i < 2 {
			require.Equal(t, []types.Resource{{Type: types.ResourceTypeMCPServer, ID: target.Name}}, actual.Spec.Manifest.Resources)
		} else if i == 2 {
			require.Equal(t, []types.Resource{resources[i], {Type: types.ResourceTypeMCPServer, ID: target.Name}}, actual.Spec.Manifest.Resources)
		} else {
			require.Equal(t, []types.Resource{resources[i]}, actual.Spec.Manifest.Resources)
		}
	}
	require.NoError(t, Cleanup(t.Context(), client))
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &v1.MCPServer{})))
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &v1.MCPServerInstance{})))
}

func TestSharedEntryMigratesOnlyItsDeployments(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			entry := testEntry("catalog")
			rule := testRule("everyone", types.Resource{Type: types.ResourceTypeSelector, ID: "*"})
			filter := &v1.MCPWebhookValidation{Name: "entry-filter", Namespace: entry.Namespace}
			filter.Spec.Manifest.Resources = []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}}
			objects := []kclient.Object{entry, rule, filter}
			var want []types.Resource
			for i := range count {
				server := &v1.MCPServer{
					Name:      fmt.Sprintf("shared-%d", i),
					Namespace: entry.Namespace,
					Spec: v1.MCPServerSpec{
						MCPCatalogID:              "default",
						MCPServerCatalogEntryName: entry.Name,
						Manifest:                  types.MCPServerManifest{Name: "Shared", Runtime: types.RuntimeRemote},
					},
				}
				objects = append(objects, server)
				want = append(want, types.Resource{Type: types.ResourceTypeMCPServer, ID: migrationName(system.VMCPPrefix, entry.Namespace, server.Name)})
			}
			client := migrationClientBuilder().WithObjects(objects...).Build()
			handler := testHandler(t, nil, make(map[string]map[string]string))
			for range 2 {
				require.NoError(t, handler.MigrateAll(t.Context(), client))
				var targets v1.VMCPList
				require.NoError(t, client.List(t.Context(), &targets))
				require.Len(t, targets.Items, count)
				for _, target := range targets.Items {
					require.NotEqual(t, entry.Name, target.Spec.LegacySlug)
					require.Equal(t, "Shared", target.Spec.Manifest.DisplayName)
					require.True(t, vmcp.IsMultiUser(target.Spec.Manifest.Components[0]))
					require.Equal(t, rule.Spec.Manifest.Subjects, target.Spec.Manifest.Profiles[0].Subjects)
				}
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), filter))
				require.ElementsMatch(t, want, filter.Spec.Manifest.Resources)
			}
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), entry))
		})
	}
}

func TestEntryFilterPreservesAllPersonalFallbacks(t *testing.T) {
	entry := testEntry("private")
	filter := &v1.MCPWebhookValidation{Name: "entry-filter", Namespace: entry.Namespace}
	filter.Spec.Manifest.Resources = []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}}
	objects := []kclient.Object{entry, filter}
	var want []types.Resource
	for _, id := range []string{"first", "second"} {
		objects = append(objects, &v1.MCPServer{
			Name:      id,
			Namespace: entry.Namespace,
			Spec: v1.MCPServerSpec{
				UserID:                    id,
				MCPServerCatalogEntryName: entry.Name,
				Manifest:                  types.MCPServerManifest{Name: id, Runtime: types.RuntimeRemote},
			},
		})
		want = append(want, types.Resource{Type: types.ResourceTypeMCPServer, ID: migrationName(system.VMCPPrefix, entry.Namespace, id)})
	}
	client := migrationClientBuilder().WithObjects(objects...).Build()
	handler := testHandler(t, nil, make(map[string]map[string]string))
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), filter))
	require.ElementsMatch(t, want, filter.Spec.Manifest.Resources)
}

func TestSharedEntryClassificationIsNamespaced(t *testing.T) {
	entry := testEntry("catalog")
	otherEntry := entry.DeepCopy()
	otherEntry.Namespace = "other"
	shared := &v1.MCPServer{
		Name:      "shared",
		Namespace: otherEntry.Namespace,
		Spec: v1.MCPServerSpec{
			MCPCatalogID:              "default",
			MCPServerCatalogEntryName: otherEntry.Name,
			Manifest: types.MCPServerManifest{
				Name:    "Shared",
				Runtime: types.RuntimeRemote,
			},
		},
	}
	rule := testRule("everyone", types.Resource{Type: types.ResourceTypeSelector, ID: "*"})
	client := migrationClientBuilder().WithObjects(entry, otherEntry, shared, rule).Build()
	handler := testHandler(t, nil, make(map[string]map[string]string))

	require.NoError(t, handler.MigrateAll(t.Context(), client))

	var targets v1.VMCPList
	require.NoError(t, client.List(t.Context(), &targets))
	require.Len(t, targets.Items, 2)
	var target v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{
		Namespace: entry.Namespace,
		Name:      migrationName(system.VMCPPrefix, entry.Namespace, entry.Name),
	}, &target))
	require.Equal(t, rule.Spec.Manifest.Subjects, target.Spec.Manifest.Profiles[0].Subjects)
}

func TestEntryFilterSurvivesPartialMigrationAndRetry(t *testing.T) {
	for _, shared := range []bool{true, false} {
		t.Run(fmt.Sprintf("shared=%t", shared), func(t *testing.T) {
			entry := testEntry("catalog")
			filter := &v1.MCPWebhookValidation{Name: "entry-filter", Namespace: entry.Namespace}
			source := types.Resource{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}
			filter.Spec.Manifest.Resources = []types.Resource{source}
			objects := []kclient.Object{entry, filter}
			var servers []*v1.MCPServer
			var want []types.Resource
			for _, id := range []string{"first", "second"} {
				server := &v1.MCPServer{
					Name:      id,
					Namespace: entry.Namespace,
					Spec: v1.MCPServerSpec{
						UserID:                    id,
						MCPServerCatalogEntryName: entry.Name,
						Manifest:                  types.MCPServerManifest{Name: id, Runtime: types.RuntimeRemote},
					},
				}
				if shared {
					server.Spec.MCPCatalogID = "default"
				}

				servers = append(servers, server)
				objects = append(objects, server)
				want = append(want, types.Resource{Type: types.ResourceTypeMCPServer, ID: migrationName(system.VMCPPrefix, entry.Namespace, id)})
			}

			client := migrationClientBuilder().WithObjects(objects...).Build()
			handler := testHandler(t, nil, make(map[string]map[string]string))
			request := func(object kclient.Object) router.Request {
				return router.Request{Ctx: t.Context(), Client: client, Object: object, Namespace: entry.Namespace}
			}
			cleanup := func() {
				t.Helper()
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), filter))
				require.NoError(t, (&mcpwebhookvalidation.Handler{}).CleanupResources(request(filter), nil))
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), filter))
			}

			failure := errors.New("second destination unavailable")
			failing := interceptor.NewClient(client, interceptor.Funcs{
				Create: func(ctx context.Context, c kclient.WithWatch, object kclient.Object, opts ...kclient.CreateOption) error {
					if _, ok := object.(*v1.VMCP); ok && object.GetName() == want[1].ID {
						return failure
					}
					return c.Create(ctx, object, opts...)
				},
			})
			require.ErrorIs(t, handler.MigrateAll(t.Context(), failing), failure)
			cleanup()
			require.Equal(t, []types.Resource{source}, filter.Spec.Manifest.Resources)
			for _, server := range servers {
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &v1.MCPServer{}))
			}

			require.NoError(t, handler.MigrateAll(t.Context(), client))
			cleanup()
			require.ElementsMatch(t, want, filter.Spec.Manifest.Resources)
			require.NoError(t, Cleanup(t.Context(), client))
			cleanup()
			require.ElementsMatch(t, want, filter.Spec.Manifest.Resources)
			require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(servers[0]), &v1.MCPServer{})))
		})
	}
}

func TestPersonalFallbackAndRetryAfterFailure(t *testing.T) {
	server := &v1.MCPServer{
		Name:      "private",
		Namespace: "default",
		Spec: v1.MCPServerSpec{
			UserID: "1",
			Manifest: types.MCPServerManifest{
				Name:         "Private",
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"},
				Config:       []types.MCPConfig{{Key: "TOKEN", Value: "inline", Usage: types.Header}},
			},
		},
	}
	client := migrationClientBuilder().WithObjects(server).Build()
	destination := make(map[string]map[string]string)
	handler := testHandler(t, nil, destination)
	handler.copyOAuth = func(context.Context, string, string, string) error { return errors.New("OAuth copy failed") }
	require.ErrorContains(t, handler.MigrateAll(t.Context(), client), "OAuth copy failed")
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &v1.MCPServer{}))
	var instances v1.VMCPInstanceList
	require.NoError(t, client.List(t.Context(), &instances))
	require.Empty(t, instances.Items)
	handler.copyOAuth = nil
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	var targets v1.VMCPList
	require.NoError(t, client.List(t.Context(), &targets))
	require.Len(t, targets.Items, 1)
	target := targets.Items[0]
	require.Equal(t, "1", target.Spec.UserID)
	require.Empty(t, target.Spec.Manifest.Profiles)
	require.Empty(t, destination[vmcp.StaticConfigurationCredentialContext(target.Name)])
	require.Equal(t, types.VMCPConfigurationPolicyUserAllowed, target.Spec.Manifest.Components[0].Configuration[0].Policy)
	require.NoError(t, client.List(t.Context(), &instances))
	require.Len(t, instances.Items, 1)
	require.Equal(t, map[string]string{vmcp.ConfigurationKey("private", "TOKEN"): "inline"}, destination[vmcp.InstanceConfigurationCredentialContext(instances.Items[0].Name)])
}

func TestMigrationPreservesFinalizers(t *testing.T) {
	legacy := &v1.MCPServer{
		Name:       "legacy",
		Namespace:  "default",
		Finalizers: []string{"test"},
		Spec: v1.MCPServerSpec{
			UserID:   "1",
			Manifest: types.MCPServerManifest{Name: "Legacy", Runtime: types.RuntimeRemote},
		},
	}
	client := migrationClientBuilder().WithObjects(legacy).Build()
	handler := testHandler(t, nil, make(map[string]map[string]string))
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	require.NoError(t, Cleanup(t.Context(), client))
	require.NoError(t, Cleanup(t.Context(), client))
	var deleting v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(legacy), &deleting))
	require.False(t, deleting.DeletionTimestamp.IsZero())
	require.Equal(t, legacy.Finalizers, deleting.Finalizers)
	// Finalizing sources do not repeat any migration work.
	require.NoError(t, (&Handler{}).MigrateAll(t.Context(), client))
}

func TestWildcardProfilesRespectScope(t *testing.T) {
	entry := testEntry("catalog")
	rule := testRule("everyone", types.Resource{Type: types.ResourceTypeSelector, ID: "*"})
	workspaceRule := testRule("workspace", types.Resource{Type: types.ResourceTypeSelector, ID: "*"})
	workspaceRule.Spec.PowerUserWorkspaceID = "workspace"
	otherNamespace := testRule("other-namespace", types.Resource{Type: types.ResourceTypeSelector, ID: "*"})
	otherNamespace.Namespace = "other"
	client := migrationClientBuilder().WithObjects(entry, rule, workspaceRule, otherNamespace).Build()
	handler := testHandler(t, nil, make(map[string]map[string]string))
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	var targets v1.VMCPList
	require.NoError(t, client.List(t.Context(), &targets))
	require.Len(t, targets.Items, 1)
	require.Len(t, targets.Items[0].Spec.Manifest.Profiles, 1)
	require.Equal(t, rule.Name, targets.Items[0].Spec.Manifest.Profiles[0].Name)
}

func TestCredentialFailureRetainsSources(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failRead bool
	}{
		{
			name:     "read",
			failRead: true,
		},
		{
			name: "write",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &v1.MCPServer{
				Name:      "shared",
				Namespace: "default",
				Spec: v1.MCPServerSpec{
					MCPCatalogID: "default",
					Manifest:     types.MCPServerManifest{Name: "Shared", Runtime: types.RuntimeRemote},
				},
			}
			client := migrationClientBuilder().WithObjects(server).Build()
			handler := testHandler(t, nil, make(map[string]map[string]string))
			failure := errors.New("credential store unavailable")
			if tc.failRead {
				handler.reveal = func(context.Context, []string, string) (gatewaytypes.Credential, error) {
					return gatewaytypes.Credential{}, failure
				}
			} else {
				handler.upsert = func(context.Context, gatewaytypes.Credential) error { return failure }
			}
			require.ErrorIs(t, handler.MigrateAll(t.Context(), client), failure)
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &v1.MCPServer{}))
			var targets v1.VMCPList
			require.NoError(t, client.List(t.Context(), &targets))
			require.Empty(t, targets.Items)
		})
	}
}

func TestExistingTargetOwnershipConflict(t *testing.T) {
	entry := testEntry("catalog")
	rule := testRule("readers", types.Resource{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name})
	conflict := &v1.VMCP{Name: migrationName(system.VMCPPrefix, entry.Namespace, entry.Name), Namespace: entry.Namespace}
	client := migrationClientBuilder().WithObjects(entry, rule, conflict).Build()
	handler := testHandler(t, nil, make(map[string]map[string]string))
	require.ErrorContains(t, handler.MigrateAll(t.Context(), client), "different ownership")
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &v1.MCPServerCatalogEntry{}))
}

func TestPersonalFallbackRetainsCatalogFilters(t *testing.T) {
	entry := testEntry("unshared")
	server := &v1.MCPServer{
		Name:      "single",
		Namespace: "default",
		Spec: v1.MCPServerSpec{
			UserID:                    "1",
			MCPServerCatalogEntryName: entry.Name,
			Manifest:                  types.MCPServerManifest{Name: "Single", Runtime: types.RuntimeRemote},
		},
	}
	filter := &v1.MCPWebhookValidation{Name: "validation", Namespace: "default"}
	filter.Spec.Manifest.Resources = []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}}
	client := migrationClientBuilder().WithObjects(entry, server, filter).Build()
	handler := testHandler(t, nil, make(map[string]map[string]string))
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	var targets v1.VMCPList
	require.NoError(t, client.List(t.Context(), &targets))
	require.Len(t, targets.Items, 1)
	require.Equal(t, "1", targets.Items[0].Spec.UserID)
	require.Equal(t, "default", targets.Items[0].Spec.Manifest.Components[0].MCPCatalogID)
	var actual v1.MCPWebhookValidation
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), &actual))
	require.Contains(t, actual.Spec.Manifest.Resources, types.Resource{Type: types.ResourceTypeMCPServer, ID: targets.Items[0].Name})
}

func TestMigrationPreservesAgentServers(t *testing.T) {
	server := &v1.MCPServer{
		Name:      "agent-server",
		Namespace: "default",
		Spec: v1.MCPServerSpec{
			NanobotAgentID: "agent",
			UserID:         "1",
			Manifest:       types.MCPServerManifest{Name: "Agent server", Runtime: types.RuntimeRemote},
		},
	}
	connection := &v1.MCPServerInstance{
		Name:      "agent-connection",
		Namespace: server.Namespace,
		Spec:      v1.MCPServerInstanceSpec{MCPServerName: server.Name, UserID: "1"},
	}
	client := migrationClientBuilder().WithObjects(server, connection).Build()
	destination := make(map[string]map[string]string)
	handler := testHandler(t, nil, destination)

	require.NoError(t, handler.MigrateAll(t.Context(), client))
	var targets v1.VMCPList
	require.NoError(t, client.List(t.Context(), &targets))
	require.Empty(t, targets.Items)
	var instances v1.VMCPInstanceList
	require.NoError(t, client.List(t.Context(), &instances))
	require.Empty(t, instances.Items)
	require.Empty(t, destination)

	require.NoError(t, Cleanup(t.Context(), client))
	var preserved v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &preserved))
	require.Equal(t, server.Spec, preserved.Spec)
	require.True(t, preserved.DeletionTimestamp.IsZero())
	var preservedConnection v1.MCPServerInstance
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(connection), &preservedConnection))
	require.Equal(t, connection.Spec, preservedConnection.Spec)
	require.True(t, preservedConnection.DeletionTimestamp.IsZero())
}

func TestMigrationSkipsExistingVMCPAndCompositeComponents(t *testing.T) {
	objects := []kclient.Object{
		&v1.MCPServer{
			Name:      "native-shared",
			Namespace: "default",
			Spec:      v1.MCPServerSpec{VMCPID: "native", MCPCatalogID: "default"},
		},
		&v1.MCPServer{
			Name:      "native-single",
			Namespace: "default",
			Spec:      v1.MCPServerSpec{VMCPInstanceID: "native-instance"},
		},
		&v1.MCPServerInstance{
			Name:      "native-connection",
			Namespace: "default",
			Spec:      v1.MCPServerInstanceSpec{VMCPInstanceID: "native-instance"},
		},
		&v1.MCPServer{
			Name:      "composite-child",
			Namespace: "default",
			Spec:      v1.MCPServerSpec{CompositeName: "composite"},
		},
		&v1.MCPServerInstance{
			Name:      "composite-connection",
			Namespace: "default",
			Spec:      v1.MCPServerInstanceSpec{CompositeName: "composite"},
		},
	}
	client := migrationClientBuilder().WithObjects(objects...).Build()
	handler := testHandler(t, nil, make(map[string]map[string]string))
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	var targets v1.VMCPList
	require.NoError(t, client.List(t.Context(), &targets))
	require.Empty(t, targets.Items)
	var instances v1.VMCPInstanceList
	require.NoError(t, client.List(t.Context(), &instances))
	require.Empty(t, instances.Items)

	require.NoError(t, Cleanup(t.Context(), client))
	for _, object := range objects[:3] {
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(object), object))
		require.True(t, object.GetDeletionTimestamp().IsZero())
	}
	for _, object := range objects[3:] {
		require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(object), object)))
	}
}

func migrationClientBuilder() *fake.ClientBuilder {
	return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
		return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
	}).WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerName", func(obj kclient.Object) []string {
		return []string{obj.(*v1.MCPServerInstance).Spec.MCPServerName}
	})
}

func TestStaticConfigurationDoesNotCauseSourceDrift(t *testing.T) {
	for _, usage := range []types.Usage{types.Header, types.Env} {
		t.Run(string(usage), func(t *testing.T) {
			entry := testEntry("static")
			entry.Spec.Manifest.Config = []types.MCPConfig{{Key: "VALUE", Usage: usage, Value: "original"}}
			rule := testRule("readers", types.Resource{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name})
			client := migrationClientBuilder().WithStatusSubresource(&v1.VMCP{}).WithObjects(entry, rule).Build()
			credentials := make(map[string]map[string]string)
			require.NoError(t, testHandler(t, nil, credentials).MigrateAll(t.Context(), client))
			var targets v1.VMCPList
			require.NoError(t, client.List(t.Context(), &targets))
			require.Len(t, targets.Items, 1)
			target := &targets.Items[0]
			require.Empty(t, target.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config[0].Value)
			require.Equal(t, "original", credentials[vmcp.StaticConfigurationCredentialContext(target.Name)][vmcp.ConfigurationKey(entry.Name, "VALUE")])
			reconcile := func(want bool) {
				t.Helper()
				require.NoError(t, vmcphandler.DetectDrift(router.Request{Ctx: t.Context(), Client: client, Object: target}, nil))
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(target), target))
				require.Equal(t, want, target.Status.Components[0].NeedsUpdate)
			}
			reconcile(false)

			entry.Spec.Manifest.Config[0].Value = "changed"
			require.NoError(t, client.Update(t.Context(), entry))
			reconcile(true)

			entry.Spec.Manifest.Config[0].Value = "original"
			require.NoError(t, client.Update(t.Context(), entry))
			reconcile(false)
			entry.Spec.Manifest.Description = "new source version"
			require.NoError(t, client.Update(t.Context(), entry))
			reconcile(true)
		})
	}
}

func TestMigrateWorkspaceSharedCredentials(t *testing.T) {
	workspace := &v1.PowerUserWorkspace{
		Name:      "workspace",
		Namespace: "default",
		Spec:      v1.PowerUserWorkspaceSpec{UserID: "owner"},
	}
	server := &v1.MCPServer{
		Name:      "shared",
		Namespace: workspace.Namespace,
		Spec: v1.MCPServerSpec{
			PowerUserWorkspaceID: workspace.Name,
			UserID:               "creator",
			Manifest: types.MCPServerManifest{
				Name:         "Shared workspace server",
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"},
				Config:       []types.MCPConfig{{Key: "TOKEN", Usage: types.Header}},
			},
		},
	}
	client := migrationClientBuilder().WithObjects(workspace, server).Build()
	destination := make(map[string]map[string]string)
	handler := testHandler(t, map[string]map[string]string{
		"workspace-shared/shared": {"TOKEN": "workspace-secret"},
		"creator-shared/shared":   {"TOKEN": "wrong-context"},
	}, destination)

	require.NoError(t, handler.MigrateAll(t.Context(), client))

	var target v1.VMCP
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKey{
		Namespace: server.Namespace,
		Name:      migrationName(system.VMCPPrefix, server.Namespace, server.Name),
	}, &target))
	require.Equal(t, map[string]string{
		vmcp.ConfigurationKey(server.Name, "TOKEN"): "workspace-secret",
	}, destination[vmcp.StaticConfigurationCredentialContext(target.Name)])
	require.Empty(t, target.Spec.Manifest.Components[0].CatalogEntry.Manifest.Config[0].Value)
}
