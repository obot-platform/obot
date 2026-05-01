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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
