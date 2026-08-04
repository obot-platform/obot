package catalogversion

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveDefaultAndExactCatalogVersions(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "entry", Namespace: system.DefaultNamespace},
		Spec:       v1.MCPServerCatalogEntrySpec{DefaultVersion: 2},
	}
	v1Manifest := types.MCPServerCatalogEntryManifest{Name: "v1", Runtime: types.RuntimeNPX}
	v2Manifest := types.MCPServerCatalogEntryManifest{Name: "v2", Runtime: types.RuntimeRemote}
	objects := []v1.MCPServerCatalogEntryVersion{
		{ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(entry.Name, 1), Namespace: entry.Namespace}, Spec: v1.MCPServerCatalogEntryVersionSpec{MCPServerCatalogEntryName: entry.Name, Version: 1, Manifest: v1Manifest, Active: false}},
		{ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(entry.Name, 2), Namespace: entry.Namespace}, Spec: v1.MCPServerCatalogEntryVersionSpec{MCPServerCatalogEntryName: entry.Name, Version: 2, Manifest: v2Manifest, Active: true}},
	}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry, &objects[0], &objects[1]).Build()

	resolved, err := ResolveDefault(t.Context(), client, entry.Namespace, entry.Name)
	require.NoError(t, err)
	assert.Equal(t, 2, resolved.Version.Spec.Version)
	assert.Equal(t, v2Manifest, resolved.Version.Spec.Manifest)

	_, err = ResolveExact(t.Context(), client, entry.Namespace, entry.Name, 1, true)
	assert.ErrorContains(t, err, "not active")
	resolved, err = ResolveExact(t.Context(), client, entry.Namespace, entry.Name, 1, false)
	require.NoError(t, err)
	assert.Equal(t, v1Manifest, resolved.Version.Spec.Manifest)
}

func TestResolveDefaultFallsBackToCompatibilityProjection(t *testing.T) {
	manifest := types.MCPServerCatalogEntryManifest{Name: "legacy", Runtime: types.RuntimeNPX}
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy", Namespace: system.DefaultNamespace},
		Spec:       v1.MCPServerCatalogEntrySpec{Manifest: manifest, UnsupportedTools: []string{"tool"}},
	}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(entry).Build()

	resolved, err := ResolveDefault(t.Context(), client, entry.Namespace, entry.Name)
	require.NoError(t, err)
	assert.Equal(t, 0, resolved.Version.Spec.Version)
	assert.Equal(t, manifest, resolved.Version.Spec.Manifest)
	assert.Equal(t, []string{"tool"}, resolved.Version.Spec.UnsupportedTools)
}
