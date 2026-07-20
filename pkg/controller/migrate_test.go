package controller

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
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

func TestMigratePublishedArtifactVisibility(t *testing.T) {
	ctx := context.Background()

	t.Run("migrates public and private artifacts", func(t *testing.T) {
		client := newFakeClient(t,
			&v1.PublishedArtifact{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "PublishedArtifact",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public-artifact",
					Namespace: system.DefaultNamespace,
				},
				Spec: v1.PublishedArtifactSpec{
					LegacyVisibility: "public",
				},
				Status: v1.PublishedArtifactStatus{
					Versions: []types.PublishedArtifactVersionEntry{
						{Version: 1},
						{Version: 2},
					},
				},
			},
			&v1.PublishedArtifact{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "PublishedArtifact",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "private-artifact",
					Namespace: system.DefaultNamespace,
				},
				Spec: v1.PublishedArtifactSpec{
					LegacyVisibility: "private",
				},
				Status: v1.PublishedArtifactStatus{
					Versions: []types.PublishedArtifactVersionEntry{
						{Version: 1, Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}},
					},
				},
			},
		)

		require.NoError(t, migratePublishedArtifactVisibility(ctx, client))

		var publicArtifact v1.PublishedArtifact
		require.NoError(t, client.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      "public-artifact",
		}, &publicArtifact))
		require.Len(t, publicArtifact.Status.Versions, 2)
		for _, version := range publicArtifact.Status.Versions {
			assert.Equal(t, []types.Subject{{
				Type: types.SubjectTypeSelector,
				ID:   "*",
			}}, version.Subjects)
		}
		assert.Empty(t, publicArtifact.Spec.LegacyVisibility)

		var privateArtifact v1.PublishedArtifact
		require.NoError(t, client.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      "private-artifact",
		}, &privateArtifact))
		require.Len(t, privateArtifact.Status.Versions, 1)
		assert.Nil(t, privateArtifact.Status.Versions[0].Subjects)
		assert.Empty(t, privateArtifact.Spec.LegacyVisibility)
	})

	t.Run("sets no subjects for invalid legacy visibility", func(t *testing.T) {
		client := newFakeClient(t,
			&v1.PublishedArtifact{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "PublishedArtifact",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bad-artifact",
					Namespace: system.DefaultNamespace,
				},
				Spec: v1.PublishedArtifactSpec{
					LegacyVisibility: "friends-only",
				},
				Status: v1.PublishedArtifactStatus{
					Versions: []types.PublishedArtifactVersionEntry{
						{Version: 1},
					},
				},
			},
		)

		err := migratePublishedArtifactVisibility(ctx, client)
		require.NoError(t, err)

		var publicArtifact v1.PublishedArtifact
		require.NoError(t, client.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      "bad-artifact",
		}, &publicArtifact))
		assert.Empty(t, publicArtifact.Status.Versions[0].Subjects)
	})
}

func TestMigrateAuditLogExportSourceTypes(t *testing.T) {
	ctx := t.Context()
	client := newFakeClient(t,
		&v1.ScheduledAuditLogExport{
			ObjectMeta: metav1.ObjectMeta{Name: "legacy-with-filters", Namespace: system.DefaultNamespace},
			Spec: v1.ScheduledAuditLogExportSpec{
				Filters: &types.AuditLogExportFilters{MCPIDs: []string{"mcp-1"}},
			},
		},
		&v1.ScheduledAuditLogExport{
			ObjectMeta: metav1.ObjectMeta{Name: "legacy-without-filters", Namespace: system.DefaultNamespace},
		},
		&v1.ScheduledAuditLogExport{
			ObjectMeta: metav1.ObjectMeta{Name: "already-explicit", Namespace: system.DefaultNamespace},
			Spec: v1.ScheduledAuditLogExportSpec{
				Filters: &types.AuditLogExportFilters{SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeMCP}},
			},
		},
		&v1.ScheduledAuditLogExport{
			ObjectMeta: metav1.ObjectMeta{Name: "local-agent", Namespace: system.DefaultNamespace},
			Spec: v1.ScheduledAuditLogExportSpec{
				Type:    types.AuditLogTypeMCP,
				Filters: &types.AuditLogExportFilters{SourceTypes: []types.AuditLogSourceType{types.AuditLogSourceTypeLocalAgentToolCall}},
			},
		},
		&v1.ScheduledAuditLogExport{
			ObjectMeta: metav1.ObjectMeta{Name: "llm", Namespace: system.DefaultNamespace},
			Spec:       v1.ScheduledAuditLogExportSpec{Type: types.AuditLogTypeLLM},
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

func TestDeleteToolReferenceOwnedModels(t *testing.T) {
	ctx := context.Background()
	client := newFakeClient(t,
		&v1.Model{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       "Model",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tool-reference-owned",
				Namespace: system.DefaultNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: v1.SchemeGroupVersion.String(),
						Kind:       "ToolReference",
						Name:       "tool",
						UID:        ktypes.UID("tool-uid"),
					},
				},
			},
		},
		&v1.Model{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       "Model",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "model-provider-owned",
				Namespace: system.DefaultNamespace,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: v1.SchemeGroupVersion.String(),
						Kind:       "ModelProvider",
						Name:       "provider",
						UID:        ktypes.UID("provider-uid"),
					},
				},
			},
		},
		&v1.Model{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       "Model",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unowned",
				Namespace: system.DefaultNamespace,
			},
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

func TestExtractAndClearMCPServerConfigValues(t *testing.T) {
	manifest := types.MCPServerManifest{
		Env: []types.MCPEnv{
			{
				MCPHeader: types.MCPHeader{
					Key:   "TOKEN",
					Value: "secret-token",
				},
			},
			{
				MCPHeader: types.MCPHeader{
					Key: "EMPTY",
				},
			},
			{
				MCPHeader: types.MCPHeader{
					Value: "missing-key",
				},
			},
		},
		RemoteConfig: &types.RemoteRuntimeConfig{
			Headers: []types.MCPHeader{
				{
					Key:   "Authorization",
					Value: "Bearer secret",
				},
				{
					Key: "X-Empty",
				},
			},
		},
	}

	values, changed := extractAndClearMCPServerConfigValues(&manifest)

	assert.True(t, changed)
	assert.Equal(t, map[string]string{
		"TOKEN":         "secret-token",
		"Authorization": "Bearer secret",
	}, values)
	assert.Empty(t, manifest.Env[0].Value)
	assert.Empty(t, manifest.Env[1].Value)
	assert.Empty(t, manifest.Env[2].Value)
	assert.Empty(t, manifest.RemoteConfig.Headers[0].Value)
	assert.Empty(t, manifest.RemoteConfig.Headers[1].Value)
}

func TestExtractAndClearMCPServerConfigValuesNoValues(t *testing.T) {
	manifest := types.MCPServerManifest{
		Env: []types.MCPEnv{
			{
				MCPHeader: types.MCPHeader{
					Key: "TOKEN",
				},
			},
		},
	}

	values, changed := extractAndClearMCPServerConfigValues(&manifest)

	assert.False(t, changed)
	assert.Empty(t, values)
}

func TestMCPServerCredentialContext(t *testing.T) {
	assert.Equal(t, "default-server-1", mcpServerCredentialContext(v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server-1"},
		Spec: v1.MCPServerSpec{
			MCPCatalogID: "default",
		},
	}))

	assert.Equal(t, "workspace-1-server-2", mcpServerCredentialContext(v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server-2"},
		Spec: v1.MCPServerSpec{
			PowerUserWorkspaceID: "workspace-1",
		},
	}))

	assert.Empty(t, mcpServerCredentialContext(v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server-3"},
	}))
}
