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

func TestRemovedEntryWithServerBecomesEditable(t *testing.T) {
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
	secondServer := server.DeepCopy()
	secondServer.Name = "ms2context7"
	c := newCatalogFakeClient(entry, server, secondServer)

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(entry), &updated))
	assert.True(t, updated.Spec.Editable)
	assert.Empty(t, updated.Spec.SourceURL)
	assert.Empty(t, updated.Spec.Manifest.EntryKey)
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
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(secondServer), &existingServer))
}

func TestUnreferencedRemovedEntryIsDeleted(t *testing.T) {
	catalog := testCatalog()
	entry := managedCatalogEntry(t, catalog, "default-context7-12345678")
	c := newCatalogFakeClient(entry)

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))

	var deleted v1.MCPServerCatalogEntry
	err := c.Get(t.Context(), client.ObjectKeyFromObject(entry), &deleted)
	require.True(t, apierrors.IsNotFound(err))
}

func TestEntriesFromRemovedSourceAreDeleted(t *testing.T) {
	catalog := testCatalog()
	managed := managedCatalogEntry(t, catalog, "default-context7-12345678")
	other := managedCatalogEntry(t, catalog, "default-other-12345678")
	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "ms1context7", Namespace: catalog.Namespace},
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: managed.Name,
		},
	}
	catalog.Spec.SourceURLs = nil
	c := newCatalogFakeClient(managed, other, server)

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))

	for _, entry := range []*v1.MCPServerCatalogEntry{managed, other} {
		var deleted v1.MCPServerCatalogEntry
		err := c.Get(t.Context(), client.ObjectKeyFromObject(entry), &deleted)
		require.True(t, apierrors.IsNotFound(err), "entry %q was not deleted", entry.Name)
	}
}

func TestEditableEntryIsPreserved(t *testing.T) {
	catalog := testCatalog()
	entry := managedCatalogEntry(t, catalog, "default-context7-12345678")
	entry.Spec.Editable = true
	entry.Spec.SourceURL = ""
	catalog.Spec.SourceURLs = nil
	c := newCatalogFakeClient(entry)

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))

	var preserved v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(entry), &preserved))
	assert.Equal(t, entry.Spec, preserved.Spec)
}

func TestEntryFromRemovedSourceWithoutApplyLabelsIsDeleted(t *testing.T) {
	catalog := testCatalog()
	entry := managedCatalogEntry(t, catalog, "default-context7-12345678")
	entry.Labels = nil
	catalog.Spec.SourceURLs = nil
	c := newCatalogFakeClient(entry)

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))

	var deleted v1.MCPServerCatalogEntry
	err := c.Get(t.Context(), client.ObjectKeyFromObject(entry), &deleted)
	require.True(t, apierrors.IsNotFound(err))
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

func TestReconcileRemovedEntriesListsServersOnce(t *testing.T) {
	catalog := testCatalog()
	referenced := managedCatalogEntry(t, catalog, "default-referenced-12345678")
	unused := managedCatalogEntry(t, catalog, "default-unused-12345678")
	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server", Namespace: catalog.Namespace},
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: referenced.Name,
		},
	}
	c := &serverListCountingClient{Client: newCatalogFakeClient(referenced, unused, server)}

	require.NoError(t, reconcileRemovedEntries(t.Context(), c, catalog, nil))
	assert.Equal(t, 1, c.serverListCalls)

	var converted v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(referenced), &converted))
	assert.True(t, converted.Spec.Editable)

	var deleted v1.MCPServerCatalogEntry
	err := c.Get(t.Context(), client.ObjectKeyFromObject(unused), &deleted)
	require.True(t, apierrors.IsNotFound(err))
}

func TestConvertCatalogEntryToEditableIgnoresNotFound(t *testing.T) {
	catalog := testCatalog()

	require.NoError(t, convertCatalogEntryToEditable(t.Context(), newCatalogFakeClient(), catalog, "missing"))
}

func testCatalog() *v1.MCPCatalog {
	return &v1.MCPCatalog{
		TypeMeta: metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "MCPCatalog"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
			UID:       ktypes.UID("catalog-uid"),
		},
		Spec: v1.MCPCatalogSpec{SourceURLs: []string{"github.com/obot-platform/catalog"}},
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
			SourceURL:      "github.com/obot-platform/catalog",
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
		WithIndex(&v1.MCPServerCatalogEntry{}, "spec.mcpCatalogName", func(obj client.Object) []string {
			entry := obj.(*v1.MCPServerCatalogEntry)
			if entry.Spec.MCPCatalogName == "" {
				return nil
			}
			return []string{entry.Spec.MCPCatalogName}
		}).
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

type serverListCountingClient struct {
	client.Client
	serverListCalls int
}

func (c *serverListCountingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*v1.MCPServerList); ok {
		c.serverListCalls++
	}
	return c.Client.List(ctx, list, opts...)
}
