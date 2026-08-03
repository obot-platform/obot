package mcpcatalog

import (
	"testing"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDetachReferencedRemovedEntries(t *testing.T) {
	catalog := testCatalog()
	entry := managedCatalogEntry(t, catalog, "default-context7-12345678")
	entry.Labels["example.com/label"] = "keep"
	entry.Annotations["example.com/annotation"] = "keep"
	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "ms1context7", Namespace: catalog.Namespace},
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: entry.Name,
		},
	}
	c := newCatalogFakeClient(entry, server)

	require.NoError(t, detachReferencedRemovedEntries(t.Context(), c, catalog, nil))

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(entry), &updated))
	assert.True(t, updated.Spec.Editable)
	assert.Empty(t, updated.Spec.SourceURL)
	assert.Empty(t, updated.Spec.Manifest.EntryKey)
	assert.Equal(t, "true", updated.Annotations[detachedEntryAnnotation])
	assert.Equal(t, "keep", updated.Labels["example.com/label"])
	assert.Equal(t, "keep", updated.Annotations["example.com/annotation"])
	for key := range updated.Labels {
		assert.NotContains(t, key, apply.LabelPrefix)
	}
	for key := range updated.Annotations {
		assert.NotContains(t, key, apply.LabelPrefix)
	}
	assert.Empty(t, updated.OwnerReferences)

	var existingServer v1.MCPServer
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(server), &existingServer))
}

func TestUnreferencedRemovedEntryRemainsManagedForPrune(t *testing.T) {
	catalog := testCatalog()
	entry := managedCatalogEntry(t, catalog, "default-context7-12345678")
	c := newCatalogFakeClient(entry)

	require.NoError(t, detachReferencedRemovedEntries(t.Context(), c, catalog, nil))

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(entry), &updated))
	assert.False(t, updated.Spec.Editable)
	assert.NotEmpty(t, updated.Labels[apply.LabelHash])
}

func TestFilterDetachedCatalogEntriesReportsConflict(t *testing.T) {
	existing := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "default-context7-12345678",
			Namespace:   "default",
			Annotations: map[string]string{detachedEntryAnnotation: "true"},
		},
		Spec: v1.MCPServerCatalogEntrySpec{Editable: true},
	}
	desired := existing.DeepCopy()
	desired.Spec.Editable = false
	desired.Spec.SourceURL = "github.com/example/catalog"
	desired.Spec.Manifest.Name = "Context7"
	c := newCatalogFakeClient(existing)

	filtered, errs, err := filterDetachedCatalogEntries(t.Context(), c, []client.Object{desired})
	require.NoError(t, err)
	assert.Empty(t, filtered)
	assert.Contains(t, errs[desired.Spec.SourceURL], "conflicts with a detached entry")
}

func TestFilterDetachedCatalogEntriesPreservesDuplicateOrder(t *testing.T) {
	first := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "default"}, Spec: v1.MCPServerCatalogEntrySpec{SourceURL: "first"}}
	second := first.DeepCopy()
	second.Spec.SourceURL = "second"
	c := newCatalogFakeClient()

	filtered, errs, err := filterDetachedCatalogEntries(t.Context(), c, []client.Object{first, second})
	require.NoError(t, err)
	assert.Empty(t, errs)
	require.Len(t, filtered, 2)
	assert.Equal(t, "first", filtered[0].(*v1.MCPServerCatalogEntry).Spec.SourceURL)
	assert.Equal(t, "second", filtered[1].(*v1.MCPServerCatalogEntry).Spec.SourceURL)
}

func testCatalog() *v1.MCPCatalog {
	return &v1.MCPCatalog{
		TypeMeta: metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "MCPCatalog"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
			UID:       ktypes.UID("catalog-uid"),
		},
	}
}

func managedCatalogEntry(t *testing.T, catalog *v1.MCPCatalog, name string) *v1.MCPServerCatalogEntry {
	t.Helper()
	labels, annotations, err := apply.GetLabelsAndAnnotations(scheme.Scheme, "catalog-"+catalog.Name, catalog)
	require.NoError(t, err)
	return &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   catalog.Namespace,
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1.SchemeGroupVersion.String(),
				Kind:       "MCPCatalog",
				Name:       catalog.Name,
				UID:        catalog.UID,
			}},
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: catalog.Name,
			SourceURL:      "github.com/obot/catalog",
			Manifest: types.MCPServerCatalogEntryManifest{
				Name:     "Context7",
				EntryKey: "obot-context7",
			},
		},
	}
}

func newCatalogFakeClient(objects ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj client.Object) []string {
			server := obj.(*v1.MCPServer)
			if server.Spec.MCPServerCatalogEntryName == "" {
				return nil
			}
			return []string{server.Spec.MCPServerCatalogEntryName}
		}).
		WithObjects(objects...).
		Build()
}
