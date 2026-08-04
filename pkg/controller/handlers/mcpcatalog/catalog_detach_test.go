package mcpcatalog

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDetachRemovedEntries(t *testing.T) {
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

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(entry), &updated))
	assert.True(t, updated.Spec.Editable)
	assert.Equal(t, entry.Spec.SourceURL, updated.Spec.SourceURL)
	assert.Equal(t, entry.Spec.Manifest.EntryKey, updated.Spec.Manifest.EntryKey)
	assert.True(t, updated.IsDetached())
	assert.False(t, updated.IsGitManaged())
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

func TestUnreferencedRemovedEntryIsDetached(t *testing.T) {
	catalog := testCatalog()
	entry := managedCatalogEntry(t, catalog, "default-context7-12345678")
	c := newCatalogFakeClient(entry)

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(entry), &updated))
	assert.True(t, updated.Spec.Editable)
	assert.True(t, updated.IsDetached())
	assert.Empty(t, updated.Labels[apply.LabelHash])
}

func TestEntriesFromRemovedSourceAreDeleted(t *testing.T) {
	catalog := testCatalog()
	managed := managedCatalogEntry(t, catalog, "default-context7-12345678")
	detached := managedCatalogEntry(t, catalog, "default-other-12345678")
	detached.Spec.Editable = true
	detached.Annotations[v1.MCPServerCatalogEntryDetachedAnnotation] = "true"
	delete(detached.Labels, apply.LabelHash)
	catalog.Spec.SourceURLs = nil
	c := newCatalogFakeClient(managed, detached)

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))

	for _, entry := range []*v1.MCPServerCatalogEntry{managed, detached} {
		var deleted v1.MCPServerCatalogEntry
		err := c.Get(t.Context(), client.ObjectKeyFromObject(entry), &deleted)
		require.True(t, apierrors.IsNotFound(err), "entry %q was not deleted", entry.Name)
	}
}

func TestEntrySuppliedByRemainingSourceIsNotDeleted(t *testing.T) {
	catalog := testCatalog()
	entry := managedCatalogEntry(t, catalog, "default-context7-12345678")
	entry.Spec.SourceURL = "https://github.com/example/removed"
	desired := entry.DeepCopy()
	desired.Spec.SourceURL = catalog.Spec.SourceURLs[0]
	c := newCatalogFakeClient(entry)

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, []client.Object{desired}))

	var existing v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(entry), &existing))
}

func TestFilterDetachedCatalogEntriesReportsConflict(t *testing.T) {
	existing := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "default-context7-12345678",
			Namespace:   "default",
			Annotations: map[string]string{v1.MCPServerCatalogEntryDetachedAnnotation: "true"},
		},
		Spec: v1.MCPServerCatalogEntrySpec{Editable: true},
	}
	desired := existing.DeepCopy()
	desired.Spec.Editable = false
	desired.Spec.SourceURL = "github.com/example/catalog"
	desired.Spec.Manifest.Name = "Context7"
	c := &namespaceRecordingClient{Client: newCatalogFakeClient(existing)}

	filtered, errs, err := filterDetachedCatalogEntries(t.Context(), c, "default", []client.Object{desired})
	require.NoError(t, err)
	assert.Equal(t, "default", c.catalogEntryNamespace)
	assert.Empty(t, filtered)
	assert.Contains(t, errs[desired.Spec.SourceURL], "conflicts with a detached entry")
}

func TestFilterDetachedCatalogEntriesChecksExactEntryAfterStaleList(t *testing.T) {
	existing := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "default-context7-12345678",
			Namespace:   "default",
			Annotations: map[string]string{v1.MCPServerCatalogEntryDetachedAnnotation: "true"},
		},
		Spec: v1.MCPServerCatalogEntrySpec{Editable: true},
	}
	desired := existing.DeepCopy()
	desired.Spec.Editable = false
	desired.Spec.SourceURL = "github.com/example/catalog"
	desired.Spec.Manifest.Name = "Context7"
	c := &staleCatalogEntryListClient{Client: newCatalogFakeClient(existing)}

	filtered, errs, err := filterDetachedCatalogEntries(t.Context(), c, "default", []client.Object{desired})
	require.NoError(t, err)
	assert.Empty(t, filtered)
	assert.Contains(t, errs[desired.Spec.SourceURL], "conflicts with a detached entry")
}

func TestFilterDetachedCatalogEntriesPreservesDuplicateOrder(t *testing.T) {
	first := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "default"}, Spec: v1.MCPServerCatalogEntrySpec{SourceURL: "first"}}
	second := first.DeepCopy()
	second.Spec.SourceURL = "second"
	c := newCatalogFakeClient()

	filtered, errs, err := filterDetachedCatalogEntries(t.Context(), c, "default", []client.Object{first, second})
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
		Spec: v1.MCPCatalogSpec{SourceURLs: []string{"github.com/obot/catalog"}},
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

type namespaceRecordingClient struct {
	client.Client
	catalogEntryNamespace string
}

type staleCatalogEntryListClient struct {
	client.Client
}

func (c *staleCatalogEntryListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if entries, ok := list.(*v1.MCPServerCatalogEntryList); ok {
		entries.Items = nil
		return nil
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *namespaceRecordingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*v1.MCPServerCatalogEntryList); ok {
		listOpts := (&client.ListOptions{}).ApplyOptions(opts)
		c.catalogEntryNamespace = listOpts.Namespace
	}
	return c.Client.List(ctx, list, opts...)
}
