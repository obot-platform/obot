package controller

import (
	"fmt"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/controller/handlers/catalogmigration"
	"github.com/obot-platform/obot/pkg/controller/handlers/compositemigration"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newFakeClient(t *testing.T, objects ...kclient.Object) kclient.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		Build()
}

func TestMigrateDefaultModelAccessPolicyModels(t *testing.T) {
	ctx := t.Context()

	t.Run("missing default policy is a no-op", func(t *testing.T) {
		client := newFakeClient(t)
		require.NoError(t, migrateDefaultModelAccessPolicyModels(ctx, client))
	})

	t.Run("cleans only the default policy and preserves custom policies", func(t *testing.T) {
		defaultPolicyName := system.ModelAccessPolicyPrefix + "-default"
		subjects := []types.Subject{{
			Type: types.SubjectTypeGroup,
			ID:   "custom-group",
		}}
		client := newFakeClient(t,
			&v1.ModelAccessPolicy{
				Name:      defaultPolicyName,
				Namespace: system.DefaultNamespace,
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						DisplayName: "Customized Default Policy",
						Subjects:    subjects,
						Models: []types.ModelResource{
							{ID: "obot://llm"},
							{ID: "obot://text-embedding"},
							{ID: "m1-custom"},
							{ID: "obot://image-generation"},
							{ID: "obot://llm-mini"},
							{ID: "obot://vision"},
							{ID: "custom-*"},
						},
					},
				},
			},
			&v1.ModelAccessPolicy{
				Name:      "custom-policy",
				Namespace: system.DefaultNamespace,
				Spec: v1.ModelAccessPolicySpec{
					Manifest: types.ModelAccessPolicyManifest{
						Subjects: subjects,
						Models: []types.ModelResource{
							{ID: "obot://text-embedding"},
							{ID: "obot://llm"},
							{ID: "m1-missing"},
							{ID: "not-a-model"},
							{ID: "obot://unknown"},
						},
					},
				},
			},
			&v1.Model{
				Name:      "m1-custom",
				Namespace: system.DefaultNamespace,
				Spec: v1.ModelSpec{
					Manifest: types.ModelManifest{
						Usage: types.ModelUsageLLM,
					},
				},
			},
		)

		require.NoError(t, migrateDefaultModelAccessPolicyModels(ctx, client))
		require.NoError(t, migrateDefaultModelAccessPolicyModels(ctx, client))

		var defaultPolicy v1.ModelAccessPolicy
		require.NoError(t, client.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      defaultPolicyName,
		}, &defaultPolicy))
		assert.Equal(t, "Customized Default Policy", defaultPolicy.Spec.Manifest.DisplayName)
		assert.Equal(t, subjects, defaultPolicy.Spec.Manifest.Subjects)
		assert.Equal(t, []types.ModelResource{
			{ID: "obot://llm"},
			{ID: "m1-custom"},
			{ID: "obot://llm-mini"},
			{ID: "custom-*"},
		}, defaultPolicy.Spec.Manifest.Models)

		var customPolicy v1.ModelAccessPolicy
		require.NoError(t, client.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      "custom-policy",
		}, &customPolicy))
		assert.Equal(t, []types.ModelResource{
			{ID: "obot://text-embedding"},
			{ID: "obot://llm"},
			{ID: "m1-missing"},
			{ID: "not-a-model"},
			{ID: "obot://unknown"},
		}, customPolicy.Spec.Manifest.Models)
	})
}

func TestMigrateAuditLogExportSourceTypes(t *testing.T) {
	ctx := t.Context()
	client := newFakeClient(t,
		&v1.ScheduledAuditLogExport{
			Name: "legacy-with-filters", Namespace: system.DefaultNamespace,
			Spec: v1.ScheduledAuditLogExportSpec{
				Filters: &types.AuditLogExportFilters{MCPIDs: []string{"mcp-1"}},
			},
		},
		&v1.ScheduledAuditLogExport{
			Name: "legacy-without-filters", Namespace: system.DefaultNamespace,
		},
		&v1.ScheduledAuditLogExport{
			Name: "already-explicit", Namespace: system.DefaultNamespace,
			Spec: v1.ScheduledAuditLogExportSpec{
				Filters: &types.AuditLogExportFilters{SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP}},
			},
		},
		&v1.ScheduledAuditLogExport{
			Name: "local-agent", Namespace: system.DefaultNamespace,
			Spec: v1.ScheduledAuditLogExportSpec{
				Type:    types.AuditLogTypeMCP,
				Filters: &types.AuditLogExportFilters{SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeLocalAgentToolCall}},
			},
		},
		&v1.ScheduledAuditLogExport{
			Name: "llm", Namespace: system.DefaultNamespace,
			Spec: v1.ScheduledAuditLogExportSpec{Type: types.AuditLogTypeLLM},
		},
	)

	require.NoError(t, migrateAuditLogExportSourceTypes(ctx, client))

	var migrated v1.ScheduledAuditLogExport
	require.NoError(t, client.Get(ctx, kclient.ObjectKey{Name: "legacy-with-filters", Namespace: system.DefaultNamespace}, &migrated))
	require.NotNil(t, migrated.Spec.Filters)
	assert.Equal(t, []types.AuditLogSourceType{types.AuditLogSourceTypeMCP}, migrated.Spec.Filters.SourceTypes)
	assert.Equal(t, []string{"mcp-1"}, migrated.Spec.Filters.MCPIDs)

	require.NoError(t, client.Get(ctx, kclient.ObjectKey{Name: "legacy-without-filters", Namespace: system.DefaultNamespace}, &migrated))
	require.NotNil(t, migrated.Spec.Filters)
	assert.Equal(t, []types.AuditLogSourceType{types.AuditLogSourceTypeMCP}, migrated.Spec.Filters.SourceTypes)

	for _, name := range []string{"already-explicit", "local-agent", "llm"} {
		var unchanged v1.ScheduledAuditLogExport
		require.NoError(t, client.Get(ctx, kclient.ObjectKey{Name: name, Namespace: system.DefaultNamespace}, &unchanged))
		switch name {
		case "already-explicit":
			assert.Equal(t, []types.AuditLogSourceType{types.AuditLogSourceTypeMCP}, unchanged.Spec.Filters.SourceTypes)
		case "local-agent":
			assert.Equal(t, []types.AuditLogSourceType{types.AuditLogSourceTypeLocalAgentToolCall}, unchanged.Spec.Filters.SourceTypes)
		default:
			assert.Nil(t, unchanged.Spec.Filters)
		}
	}
}

func TestMigrateVMCPDefaultAdminGroups(t *testing.T) {
	for _, tc := range []struct {
		name        string
		profileName string
		allowAll    bool
		subjectType types.SubjectType
		subjectID   string
		wantType    types.SubjectType
	}{
		{
			name:        "legacy default",
			profileName: "default",
			allowAll:    true,
			subjectType: types.SubjectTypeGroup,
			subjectID:   types.GroupAdmin,
			wantType:    types.SubjectTypeObotGroup,
		},
		{
			name:        "custom profile",
			profileName: "custom",
			allowAll:    true,
			subjectType: types.SubjectTypeGroup,
			subjectID:   types.GroupAdmin,
			wantType:    types.SubjectTypeGroup,
		},
		{
			name:        "restricted default",
			profileName: "default",
			subjectType: types.SubjectTypeGroup,
			subjectID:   types.GroupAdmin,
			wantType:    types.SubjectTypeGroup,
		},
		{
			name:        "other group",
			profileName: "default",
			allowAll:    true,
			subjectType: types.SubjectTypeGroup,
			subjectID:   "other",
			wantType:    types.SubjectTypeGroup,
		},
		{
			name:        "admin user",
			profileName: "default",
			allowAll:    true,
			subjectType: types.SubjectTypeUser,
			subjectID:   types.GroupAdmin,
			wantType:    types.SubjectTypeUser,
		},
		{
			name:        "already migrated",
			profileName: "default",
			allowAll:    true,
			subjectType: types.SubjectTypeObotGroup,
			subjectID:   types.GroupAdmin,
			wantType:    types.SubjectTypeObotGroup,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := &v1.VMCP{
				Name:      "vmcp",
				Namespace: system.DefaultNamespace,
				Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
					Profiles: []types.VMCPProfile{{
						Name: tc.profileName,
						Subjects: []types.Subject{
							{Type: types.SubjectTypeUser, ID: "other"},
							{Type: tc.subjectType, ID: tc.subjectID},
						},
						Permissions: types.VMCPProfilePermissions{AllowAllComponents: tc.allowAll},
					}},
				}},
			}
			client := newFakeClient(t, vmcp)
			want := vmcp.DeepCopy()
			want.Spec.Manifest.Profiles[0].Subjects[1].Type = tc.wantType
			for range 2 {
				require.NoError(t, migrateVMCPDefaultAdminGroups(t.Context(), client))
				var got v1.VMCP
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &got))
				assert.Equal(t, want.Spec, got.Spec)
			}
		})
	}
}

func TestMigrateVMCPEverythingAdminGroups(t *testing.T) {
	for _, tc := range []struct {
		name        string
		profileName string
		displayName string
		ruleCatalog string
		workspace   string
		missingRule bool
		ambiguous   bool
		multiple    bool
		unchanged   bool
	}{
		{
			name:        "retained display name",
			profileName: "Administrators",
			displayName: "Administrators",
		},
		{
			name:        "historical name without retained rule provenance",
			profileName: "Everything",
			displayName: "Administrators",
			unchanged:   true,
		},
		{
			name:        "empty display name",
			profileName: "acr1-everything",
		},
		{
			name:        "deleted rule",
			profileName: "Everything",
			missingRule: true,
			unchanged:   true,
		},
		{
			name:        "unrelated prefix",
			profileName: "acr1-everything-custom",
			unchanged:   true,
		},
		{
			name:        "unallocated suffix",
			profileName: "acr1-everything-2",
			unchanged:   true,
		},
		{
			name:        "ambiguous retained rule",
			profileName: "Everything",
			displayName: "Everything",
			ambiguous:   true,
			unchanged:   true,
		},
		{
			name:        "multiple possible Everything profiles",
			profileName: "Everything",
			displayName: "Everything",
			multiple:    true,
			unchanged:   true,
		},
		{
			name:        "Everything rule in another catalog",
			profileName: "Everything",
			ruleCatalog: "other",
			unchanged:   true,
		},
		{
			name:        "Everything rule in a workspace",
			profileName: "Everything",
			workspace:   "puw1workspace",
			unchanged:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subjects := []types.Subject{{Type: types.SubjectTypeGroup, ID: types.GroupAdmin}}
			if !tc.missingRule {
				subjects = append(subjects,
					types.Subject{Type: types.SubjectTypeGroup, ID: "engineering"},
					types.Subject{Type: types.SubjectTypeUser, ID: types.GroupAdmin},
				)
			}
			rule := &v1.AccessControlRule{
				Name:      "acr1-everything",
				Namespace: system.DefaultNamespace,
				Spec: v1.AccessControlRuleSpec{
					MCPCatalogID: system.DefaultCatalog,
					Manifest: types.AccessControlRuleManifest{
						DisplayName: tc.displayName,
						Subjects:    subjects,
						Resources:   []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
					},
				},
			}
			if tc.ruleCatalog != "" {
				rule.Spec.MCPCatalogID = tc.ruleCatalog
			}
			rule.Spec.PowerUserWorkspaceID = tc.workspace
			vmcp := &v1.VMCP{
				Name:      "vmcp1migrated",
				Namespace: system.DefaultNamespace,
				Spec: v1.VMCPSpec{
					LegacySlug: "legacy-entry",
					Manifest: types.VMCPManifest{
						Components: []types.VMCPComponent{{ID: "component", MCPCatalogID: system.DefaultCatalog}},
						Profiles: []types.VMCPProfile{{
							Name:        tc.profileName,
							Subjects:    subjects,
							Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
						}},
					},
				},
			}
			if tc.multiple {
				another := *vmcp.Spec.Manifest.Profiles[0].DeepCopy()
				another.Name = "acr1-everything"
				vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, another)
			}
			restricted := *vmcp.Spec.Manifest.Profiles[0].DeepCopy()
			restricted.Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"component": {AllowedTools: []string{"echo"}}}}
			vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, restricted)

			created := vmcp.DeepCopy()
			created.Name = "vmcp1created"
			created.Spec.LegacySlug = ""
			personal := vmcp.DeepCopy()
			personal.Name = "vmcp1personal"
			personal.Spec.UserID = "1"
			otherNamespace := vmcp.DeepCopy()
			otherNamespace.Namespace = "other"
			otherCatalog := vmcp.DeepCopy()
			otherCatalog.Name = "vmcp1othercatalog"
			otherCatalog.Spec.LegacySlug = "other-catalog-entry"
			otherCatalog.Spec.Manifest.Components[0].MCPCatalogID = "other"
			unknownSource := vmcp.DeepCopy()
			unknownSource.Name = "vmcp1unknownsource"
			unknownSource.Spec.LegacySlug = "unknown-entry"
			unknownSource.Spec.Manifest.Components = nil
			otherSubjects := vmcp.DeepCopy()
			otherSubjects.Name = "vmcp1othersubjects"
			for i := range otherSubjects.Spec.Manifest.Profiles {
				profile := &otherSubjects.Spec.Manifest.Profiles[i]
				profile.Subjects = append(profile.Subjects, types.Subject{Type: types.SubjectTypeUser, ID: "99"})
			}
			foreignSource := vmcp.DeepCopy()
			foreignSource.Name = "vmcp1foreignsource"
			foreignSource.Spec.LegacySlug = "foreign-entry"
			workspaceSource := vmcp.DeepCopy()
			workspaceSource.Name = "vmcp1workspacesource"
			workspaceSource.Spec.LegacySlug = "workspace-entry"
			objects := []kclient.Object{vmcp, created, personal, otherNamespace, otherCatalog, unknownSource, otherSubjects, foreignSource, workspaceSource,
				&v1.MCPServerCatalogEntry{
					Name:      vmcp.Spec.LegacySlug,
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerCatalogEntrySpec{
						MCPCatalogName: system.DefaultCatalog,
					},
				},
				&v1.MCPServerCatalogEntry{
					Name:      foreignSource.Spec.LegacySlug,
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerCatalogEntrySpec{
						MCPCatalogName: "other",
					},
				},
				&v1.MCPServerCatalogEntry{
					Name:      workspaceSource.Spec.LegacySlug,
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerCatalogEntrySpec{
						MCPCatalogName:       system.DefaultCatalog,
						PowerUserWorkspaceID: "puw1workspace",
					},
				},
			}
			if !tc.missingRule {
				objects = append(objects, rule)
			}
			if tc.ambiguous {
				other := rule.DeepCopy()
				other.Name = "acr1-unrelated"
				other.Spec.Manifest.DisplayName = tc.profileName
				objects = append(objects, other)
			}
			client := newFakeClient(t, objects...)
			want := vmcp.DeepCopy()
			if !tc.unchanged {
				want.Spec.Manifest.Profiles[0].Subjects[0].Type = types.SubjectTypeObotGroup
			}

			require.NoError(t, migrateVMCPEverythingAdminGroups(t.Context(), client))
			var got v1.VMCP
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &got))
			assert.Equal(t, want.Spec, got.Spec)

			version := got.ResourceVersion
			require.NoError(t, migrateVMCPEverythingAdminGroups(t.Context(), client))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &got))
			assert.Equal(t, version, got.ResourceVersion, "rerunning must not write an unchanged vMCP")
			for _, unchanged := range []*v1.VMCP{created, personal, otherNamespace, otherCatalog, unknownSource, otherSubjects, foreignSource, workspaceSource} {
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(unchanged), &got))
				assert.Equal(t, unchanged.Spec, got.Spec)
			}
		})
	}
}

func TestMigrateVMCPEverythingAdminGroupsWorkspace(t *testing.T) {
	for _, defaultCatalogComponents := range []bool{false, true} {
		name := "workspace shared server"
		if defaultCatalogComponents {
			name = "workspace owner with default catalog components"
		}
		t.Run(name, func(t *testing.T) {
			workspace := &v1.PowerUserWorkspace{
				Name:      "puw1workspace",
				Namespace: system.DefaultNamespace,
				Spec: v1.PowerUserWorkspaceSpec{
					UserID: "1",
				},
			}
			server := &v1.MCPServer{
				Name:      "ms1workspace",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					PowerUserWorkspaceID: workspace.Name,
					UserID:               "1",
					Manifest: types.MCPServerManifest{
						Name:         "Workspace server",
						Runtime:      types.RuntimeRemote,
						RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"},
					},
				},
			}
			rule := &v1.AccessControlRule{
				Name:      "acr1workspace",
				Namespace: system.DefaultNamespace,
				Spec: v1.AccessControlRuleSpec{
					PowerUserWorkspaceID: workspace.Name,
					Manifest: types.AccessControlRuleManifest{
						DisplayName: "Everything",
						Subjects:    []types.Subject{{Type: types.SubjectTypeGroup, ID: types.GroupAdmin}},
						Resources:   []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
					},
				},
			}
			defaultRule := rule.DeepCopy()
			defaultRule.Name = "acr1-everything"
			defaultRule.Spec.PowerUserWorkspaceID = ""
			defaultRule.Spec.MCPCatalogID = system.DefaultCatalog
			client := newFakeClient(t, workspace, server, rule, defaultRule)
			gatewayClient := newTestGatewayClient(t)
			t.Cleanup(func() { require.NoError(t, gatewayClient.Close()) })
			require.NoError(t, catalogmigration.New(gatewayClient, nil).MigrateAll(t.Context(), client))
			require.NoError(t, catalogmigration.Cleanup(t.Context(), client))
			var targets v1.VMCPList
			require.NoError(t, client.List(t.Context(), &targets))
			require.Len(t, targets.Items, 1)
			target := &targets.Items[0]
			require.NotEmpty(t, target.Spec.LegacySlug)
			require.Empty(t, target.Spec.UserID)
			require.Equal(t, "1", target.Spec.CreatorUserID)
			if defaultCatalogComponents {
				target.Spec.Manifest.Components[0].MCPCatalogID = system.DefaultCatalog
				require.NoError(t, client.Update(t.Context(), target))
			}
			before := target.DeepCopy()
			providerMember := &kuser.DefaultInfo{UID: "2", Extra: map[string][]string{"auth_provider_groups": {types.GroupAdmin}}}
			obotAdmin := &kuser.DefaultInfo{UID: "3", Extra: map[string][]string{"obot_groups": {types.GroupAdmin}}}
			require.True(t, authz.UserCanConnectVMCP(providerMember, target))
			require.False(t, authz.UserCanConnectVMCP(obotAdmin, target))

			require.NoError(t, migrateVMCPEverythingAdminGroups(t.Context(), client))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(target), target))
			assert.Equal(t, before.Spec, target.Spec)
			assert.Equal(t, before.ResourceVersion, target.ResourceVersion)
			assert.True(t, authz.UserCanConnectVMCP(providerMember, target), "workspace IdP grant must be preserved")
			assert.False(t, authz.UserCanConnectVMCP(obotAdmin, target), "unrelated Obot admins must not gain access")
		})
	}
}

func TestMigrateVMCPEverythingAdminGroupsCompositeScope(t *testing.T) {
	for _, tc := range []struct {
		name             string
		catalog          string
		workspace        string
		componentCatalog string
		oldMigration     bool
		retainParent     bool
		wantRepair       bool
	}{
		{
			name:             "new default catalog composite",
			catalog:          system.DefaultCatalog,
			componentCatalog: system.DefaultCatalog,
			wantRepair:       true,
		},
		{
			name:             "default parent with other catalog components",
			catalog:          system.DefaultCatalog,
			componentCatalog: "other",
			wantRepair:       true,
		},
		{
			name:             "other parent with default catalog components",
			catalog:          "other",
			componentCatalog: system.DefaultCatalog,
		},
		{
			name:             "old other parent with default catalog components",
			catalog:          "other",
			componentCatalog: system.DefaultCatalog,
			oldMigration:     true,
		},
		{
			name:             "workspace parent with default catalog components",
			catalog:          system.DefaultCatalog,
			workspace:        "puw1workspace",
			componentCatalog: system.DefaultCatalog,
		},
		{
			name:             "old composite without parent scope",
			catalog:          system.DefaultCatalog,
			componentCatalog: system.DefaultCatalog,
			oldMigration:     true,
		},
		{
			name:             "old composite with retained default parent",
			catalog:          system.DefaultCatalog,
			componentCatalog: "other",
			oldMigration:     true,
			retainParent:     true,
			wantRepair:       true,
		},
		{
			name:             "old composite with retained other parent",
			catalog:          "other",
			componentCatalog: system.DefaultCatalog,
			oldMigration:     true,
			retainParent:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := &v1.AccessControlRule{
				Name:      "acr1-everything",
				Namespace: system.DefaultNamespace,
				Spec: v1.AccessControlRuleSpec{
					MCPCatalogID: system.DefaultCatalog,
					Manifest: types.AccessControlRuleManifest{
						DisplayName: "Everything",
						Subjects:    []types.Subject{{Type: types.SubjectTypeGroup, ID: types.GroupAdmin}},
						Resources:   []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
					},
				},
			}
			entry := &v1.MCPServerCatalogEntry{
				Name:      "mcp1composite",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerCatalogEntrySpec{
					MCPCatalogName:       tc.catalog,
					PowerUserWorkspaceID: tc.workspace,
					Manifest: types.MCPServerCatalogEntryManifest{
						Name:    "Composite",
						Runtime: types.RuntimeComposite,
					},
				},
			}
			entry.Spec.LegacyCompositeManifest = []byte(`{"name":"Composite","runtime":"composite","compositeConfig":{"componentServers":[{"mcpServerID":"ms1shared"}]}}`) //nolint:staticcheck // Exercise the removed composite schema at the migration boundary.
			if tc.retainParent {
				entry.Finalizers = []string{v1.MCPServerCatalogEntryFinalizer}
			}
			shared := &v1.MCPServer{
				Name:      "ms1shared",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					MCPCatalogID: tc.componentCatalog,
					Manifest: types.MCPServerManifest{
						Name:         "Shared server",
						Runtime:      types.RuntimeRemote,
						RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"},
					},
				},
			}
			objects := []kclient.Object{rule, entry, shared}
			if tc.catalog != system.DefaultCatalog || tc.workspace != "" {
				other := rule.DeepCopy()
				other.Name = "acr1-other"
				other.Spec.MCPCatalogID = tc.catalog
				other.Spec.PowerUserWorkspaceID = tc.workspace
				objects = append(objects, other)
			}
			if tc.workspace != "" {
				objects = append(objects, &v1.PowerUserWorkspace{
					Name:      tc.workspace,
					Namespace: system.DefaultNamespace,
					Spec: v1.PowerUserWorkspaceSpec{
						UserID: "1",
					},
				})
			}
			client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).
				WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
					return []string{obj.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
				}).Build()
			gatewayClient := newTestGatewayClient(t)
			t.Cleanup(func() { require.NoError(t, gatewayClient.Close()) })
			require.NoError(t, compositemigration.New(gatewayClient).MigrateAll(t.Context(), client))
			var targets v1.VMCPList
			require.NoError(t, client.List(t.Context(), &targets))
			require.Len(t, targets.Items, 1)
			target := &targets.Items[0]
			require.Equal(t, tc.componentCatalog, target.Spec.Manifest.Components[0].MCPCatalogID)
			wantScope := "false"
			if tc.catalog == system.DefaultCatalog && tc.workspace == "" {
				wantScope = "true"
			}
			require.Equal(t, wantScope, target.Annotations[compositemigration.DefaultCatalogSourceAnnotation])
			if tc.oldMigration {
				target.Annotations = nil
				require.NoError(t, client.Update(t.Context(), target))
			}
			var parent v1.MCPServerCatalogEntry
			err := client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), &parent)
			if tc.retainParent {
				require.NoError(t, err)
				require.False(t, parent.DeletionTimestamp.IsZero())
			} else {
				require.True(t, apierrors.IsNotFound(err))
			}
			before := target.DeepCopy()
			providerMember := &kuser.DefaultInfo{UID: "2", Extra: map[string][]string{"auth_provider_groups": {types.GroupAdmin}}}
			obotAdmin := &kuser.DefaultInfo{UID: "3", Extra: map[string][]string{"obot_groups": {types.GroupAdmin}}}
			require.True(t, authz.UserCanConnectVMCP(providerMember, target))
			require.False(t, authz.UserCanConnectVMCP(obotAdmin, target))

			require.NoError(t, migrateEverythingAccessControlRuleAdminGroup(t.Context(), client))
			require.NoError(t, migrateVMCPEverythingAdminGroups(t.Context(), client))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(target), target))
			assert.Equal(t, tc.wantRepair, authz.UserCanConnectVMCP(obotAdmin, target))
			assert.Equal(t, !tc.wantRepair, authz.UserCanConnectVMCP(providerMember, target))
			if !tc.wantRepair {
				assert.Equal(t, before.Spec, target.Spec)
				assert.Equal(t, before.ResourceVersion, target.ResourceVersion)
			}
		})
	}
}

func TestMigrateVMCPEverythingAdminGroupsUpgrade(t *testing.T) {
	for _, tc := range []struct {
		alreadyMigrated bool
		collisions      int
		shared          bool
	}{
		{
			alreadyMigrated: false,
			collisions:      0,
		},
		{
			alreadyMigrated: true,
			collisions:      0,
		},
		{
			alreadyMigrated: false,
			collisions:      2,
		},
		{
			alreadyMigrated: true,
			collisions:      2,
		},
		{
			alreadyMigrated: false,
			collisions:      3,
		},
		{
			alreadyMigrated: true,
			collisions:      3,
		},
		{
			alreadyMigrated: false,
			shared:          true,
		},
		{
			alreadyMigrated: true,
			shared:          true,
		},
	} {
		name := "new upgrade"
		if tc.alreadyMigrated {
			name = "previous migrations completed"
		}
		if tc.shared {
			name += " shared server"
		}
		t.Run(fmt.Sprintf("%s/%d collisions", name, tc.collisions), func(t *testing.T) {
			ctx := t.Context()
			rule := &v1.AccessControlRule{
				Name:      "acr1-everything",
				Namespace: system.DefaultNamespace,
				Spec: v1.AccessControlRuleSpec{
					MCPCatalogID: system.DefaultCatalog,
					Manifest: types.AccessControlRuleManifest{
						DisplayName: "Everything",
						Subjects:    []types.Subject{{Type: types.SubjectTypeGroup, ID: types.GroupAdmin}},
						Resources:   []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
					},
				},
			}
			entry := &v1.MCPServerCatalogEntry{
				Name:      "legacy-entry",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerCatalogEntrySpec{
					MCPCatalogName: system.DefaultCatalog,
					Manifest: types.MCPServerCatalogEntryManifest{
						Name:         "Remote server",
						Runtime:      types.RuntimeRemote,
						RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"},
					},
				},
			}
			server := &v1.MCPServer{
				Name:      "ms1legacy",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					UserID:                    "1",
					MCPServerCatalogEntryName: entry.Name,
					Manifest: types.MCPServerManifest{
						Name:         "Remote server",
						Runtime:      types.RuntimeRemote,
						RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://example.com/mcp"},
					},
				},
			}
			objects := []kclient.Object{rule, entry, server}
			if tc.shared {
				server.Spec.MCPCatalogID = system.DefaultCatalog
				objects = append(objects, &v1.MCPServerInstance{
					Name:      "mcpsi1legacy",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName: server.Name,
						UserID:        "1",
					},
				})
			}
			for i, scope := range []struct {
				catalog   string
				workspace string
			}{
				{
					catalog: "other",
				},
				{
					catalog:   system.DefaultCatalog,
					workspace: "puw1workspace",
				},
				{
					workspace: "puw1workspace",
				},
			} {
				other := rule.DeepCopy()
				other.Name = fmt.Sprintf("acr1-unrelated-%d", i)
				other.Spec.MCPCatalogID = scope.catalog
				other.Spec.PowerUserWorkspaceID = scope.workspace
				objects = append(objects, other)
			}
			var occupied []types.VMCPProfile
			for i, displayName := range []string{"Everything", "acr1-everything", "acr1-everything-2"}[:tc.collisions] {
				other := rule.DeepCopy()
				other.Name = fmt.Sprintf("acr1-%d", i)
				other.Spec.Manifest.DisplayName = displayName
				other.Spec.Manifest.Subjects = []types.Subject{{Type: types.SubjectTypeUser, ID: "99"}}
				objects = append(objects, other)
				occupied = catalogmigration.AppendRuleProfile(occupied, *other)
			}
			expectedProfiles := catalogmigration.AppendRuleProfile(occupied, *rule)
			if tc.collisions > 0 {
				require.Equal(t, fmt.Sprintf("acr1-everything-%d", tc.collisions), expectedProfiles[tc.collisions].Name)
			}
			client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).
				WithIndex(&v1.VMCP{}, "spec.legacySlug", func(obj kclient.Object) []string {
					return []string{obj.(*v1.VMCP).Spec.LegacySlug}
				}).
				WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(obj kclient.Object) []string {
					return []string{obj.(*v1.VMCPInstance).Spec.LegacySlug}
				}).Build()
			gatewayClient := newTestGatewayClient(t)
			t.Cleanup(func() { require.NoError(t, gatewayClient.Close()) })
			migrations := []struct {
				name string
				run  func() error
			}{
				{
					name: compositemigration.MigrationName,
					run:  func() error { return compositemigration.New(gatewayClient).MigrateAll(ctx, client) },
				},
				{
					name: catalogmigration.MigrationName,
					run:  func() error { return catalogmigration.New(gatewayClient, nil).MigrateAll(ctx, client) },
				},
				{
					name: "vmcp_default_admin_obot_groups",
					run:  func() error { return migrateVMCPDefaultAdminGroups(ctx, client) },
				},
				{
					name: "everything_acr_admin_obot_group",
					run:  func() error { return migrateEverythingAccessControlRuleAdminGroup(ctx, client) },
				},
			}
			if !tc.alreadyMigrated {
				require.NoError(t, gatewayClient.MigrateKinmIfNotRun(ctx, "everything_acr_admin_obot_group", func() error {
					return migrateEverythingAccessControlRuleAdminGroup(ctx, client)
				}))
				expectedProfiles[tc.collisions].Subjects[0].Type = types.SubjectTypeObotGroup
			}
			for _, migration := range migrations {
				require.NoError(t, gatewayClient.MigrateKinmIfNotRun(ctx, migration.name, migration.run))
			}
			require.NoError(t, catalogmigration.Cleanup(ctx, client))
			if tc.alreadyMigrated {
				for _, migration := range migrations {
					require.NoError(t, gatewayClient.MigrateKinmIfNotRun(ctx, migration.name, func() error {
						t.Fatal("completed migrations must not run on restart")
						return nil
					}))
				}
			}

			var vmcps v1.VMCPList
			var instances v1.VMCPInstanceList
			require.NoError(t, client.List(ctx, &vmcps))
			require.NoError(t, client.List(ctx, &instances))
			require.Len(t, vmcps.Items, 1)
			require.Len(t, instances.Items, 1)
			require.Equal(t, expectedProfiles, vmcps.Items[0].Spec.Manifest.Profiles)
			admin := &kuser.DefaultInfo{
				UID:    "1",
				Groups: []string{types.GroupAdmin},
				Extra:  map[string][]string{"obot_groups": {types.GroupAdmin}},
			}
			require.Equal(t, !tc.alreadyMigrated, authz.UserCanConnectVMCP(admin, &vmcps.Items[0]))
			require.NoError(t, client.Get(ctx, kclient.ObjectKeyFromObject(rule), rule))
			require.Equal(t, types.SubjectTypeObotGroup, rule.Spec.Manifest.Subjects[0].Type)

			require.NoError(t, gatewayClient.MigrateKinmIfNotRun(ctx, vmcpEverythingAdminGroupsMigrationName, func() error {
				return migrateVMCPEverythingAdminGroups(ctx, client)
			}))
			for _, id := range []string{vmcps.Items[0].Name, instances.Items[0].Name, server.Name} {
				allowed, err := authz.UserCanConnectToMCP(ctx, client, nil, admin, id)
				require.NoError(t, err)
				assert.True(t, allowed, "admin must retain access through %s", id)
			}
			var repaired v1.VMCP
			require.NoError(t, client.Get(ctx, kclient.ObjectKeyFromObject(&vmcps.Items[0]), &repaired))
			expectedProfiles[tc.collisions].Subjects[0].Type = types.SubjectTypeObotGroup
			assert.Equal(t, expectedProfiles, repaired.Spec.Manifest.Profiles)
			var instance v1.VMCPInstance
			require.NoError(t, client.Get(ctx, kclient.ObjectKeyFromObject(&instances.Items[0]), &instance))
			assert.Equal(t, instances.Items[0].Spec, instance.Spec)
			require.NoError(t, gatewayClient.MigrateKinmIfNotRun(ctx, vmcpEverythingAdminGroupsMigrationName, func() error {
				t.Fatal("completed repair must not run on restart")
				return nil
			}))
		})
	}
}

func TestDeleteToolReferenceOwnedModels(t *testing.T) {
	ctx := t.Context()
	client := newFakeClient(t,
		&v1.Model{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Model",
			Name:       "tool-reference-owned",
			Namespace:  system.DefaultNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "ToolReference",
					Name:       "tool",
					UID:        ktypes.UID("tool-uid"),
				},
			},
		},
		&v1.Model{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Model",
			Name:       "model-provider-owned",
			Namespace:  system.DefaultNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "ModelProvider",
					Name:       "provider",
					UID:        ktypes.UID("provider-uid"),
				},
			},
		},
		&v1.Model{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Model",
			Name:       "unowned",
			Namespace:  system.DefaultNamespace,
		},
	)

	require.NoError(t, deleteToolReferenceOwnedModels(ctx, client))

	var model v1.Model
	err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      "tool-reference-owned",
	}, &model)
	require.True(t, apierrors.IsNotFound(err))

	require.NoError(t, client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      "model-provider-owned",
	}, &model))
	require.NoError(t, client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      "unowned",
	}, &model))
}
