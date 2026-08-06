package mcpcatalog

import (
	"context"
	"strings"
	"testing"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestRemovedEntryWithServerBecomesDetached(t *testing.T) {
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
	assert.False(t, updated.Spec.Editable)
	assert.Equal(t, entry.Spec.SourceURL, updated.Spec.SourceURL)
	assert.Equal(t, entry.Spec.Manifest.EntryKey, updated.Spec.Manifest.EntryKey)
	assert.True(t, updated.Spec.Detached)
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
	other.Spec.Editable = false
	other.Spec.Detached = true
	delete(other.Labels, apply.LabelHash)
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

func TestFilterConflictingCatalogEntriesReportsObotManagedConflict(t *testing.T) {
	existing := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-context7-12345678",
			Namespace: "default",
		},
		Spec: v1.MCPServerCatalogEntrySpec{Editable: true},
	}
	desired := existing.DeepCopy()
	desired.Spec.Editable = false
	desired.Spec.SourceURL = "github.com/example/catalog"
	desired.Spec.Manifest.Name = "Context7"
	c := newCatalogFakeClient(existing)

	filtered, errs, err := filterConflictingCatalogEntries(t.Context(), c, "default", []client.Object{desired})
	require.NoError(t, err)
	assert.Empty(t, filtered)
	assert.Contains(t, errs[desired.Spec.SourceURL], "conflicts with an Obot-managed entry")
}

func TestFilterConflictingCatalogEntriesChecksExactEntryAfterStaleList(t *testing.T) {
	existing := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-context7-12345678",
			Namespace: "default",
		},
		Spec: v1.MCPServerCatalogEntrySpec{Editable: true},
	}
	desired := existing.DeepCopy()
	desired.Spec.Editable = false
	desired.Spec.SourceURL = "github.com/example/catalog"
	desired.Spec.Manifest.Name = "Context7"
	c := &staleCatalogEntryListClient{Client: newCatalogFakeClient(existing)}

	filtered, errs, err := filterConflictingCatalogEntries(t.Context(), c, "default", []client.Object{desired})
	require.NoError(t, err)
	assert.Empty(t, filtered)
	assert.Contains(t, errs[desired.Spec.SourceURL], "conflicts with an Obot-managed entry")
}

func TestFilterConflictingCatalogEntriesAllowsDetachedEntryReattachment(t *testing.T) {
	catalog := testCatalog()
	existing := managedCatalogEntry(t, catalog, "default-context7-12345678")
	desired := existing.DeepCopy()
	desired.Labels = nil
	desired.Annotations = nil
	desired.OwnerReferences = nil
	existing.Spec.Detached = true
	for key := range existing.Annotations {
		if strings.HasPrefix(key, apply.LabelPrefix) {
			delete(existing.Annotations, key)
		}
	}
	for key := range existing.Labels {
		if strings.HasPrefix(key, apply.LabelPrefix) {
			delete(existing.Labels, key)
		}
	}
	existing.OwnerReferences = nil
	c := newCatalogFakeClient(existing)

	filtered, errs, err := filterConflictingCatalogEntries(t.Context(), c, catalog.Namespace, []client.Object{desired})
	require.NoError(t, err)
	assert.Empty(t, errs)
	require.Len(t, filtered, 1)
	assert.False(t, filtered[0].(*v1.MCPServerCatalogEntry).Spec.Detached)

	require.NoError(t, apply.New(c).WithOwnerSubContext("catalog-"+catalog.Name).WithNoPrune().Apply(t.Context(), catalog, filtered...))

	var reattached v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(existing), &reattached))
	assert.False(t, reattached.Spec.Detached)
	assert.False(t, reattached.Spec.Editable)
	assert.True(t, reattached.IsGitManaged())
	assert.NotEmpty(t, reattached.Labels[apply.LabelHash])
}

func TestFilterConflictingCatalogEntriesAllowsGitManagedEntry(t *testing.T) {
	catalog := testCatalog()
	existing := managedCatalogEntry(t, catalog, "default-context7-12345678")
	desired := existing.DeepCopy()
	desired.Labels = nil
	desired.Annotations = nil
	desired.OwnerReferences = nil
	c := newCatalogFakeClient(existing)

	filtered, errs, err := filterConflictingCatalogEntries(t.Context(), c, catalog.Namespace, []client.Object{desired})
	require.NoError(t, err)
	assert.Empty(t, errs)
	require.Len(t, filtered, 1)
	assert.False(t, filtered[0].(*v1.MCPServerCatalogEntry).Spec.Detached)
}

func TestFilterConflictingCatalogEntriesPreservesDuplicateOrder(t *testing.T) {
	first := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: "same", Namespace: "default"}, Spec: v1.MCPServerCatalogEntrySpec{SourceURL: "first"}}
	second := first.DeepCopy()
	second.Spec.SourceURL = "second"
	c := newCatalogFakeClient()

	filtered, errs, err := filterConflictingCatalogEntries(t.Context(), c, "default", []client.Object{first, second})
	require.NoError(t, err)
	assert.Empty(t, errs)
	require.Len(t, filtered, 2)
	assert.Equal(t, "first", filtered[0].(*v1.MCPServerCatalogEntry).Spec.SourceURL)
	assert.Equal(t, "second", filtered[1].(*v1.MCPServerCatalogEntry).Spec.SourceURL)
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
	assert.False(t, converted.Spec.Editable)
	assert.True(t, converted.Spec.Detached)

	var deleted v1.MCPServerCatalogEntry
	err := c.Get(t.Context(), client.ObjectKeyFromObject(unused), &deleted)
	require.True(t, apierrors.IsNotFound(err))
}

func TestDetachCatalogEntryIgnoresNotFound(t *testing.T) {
	catalog := testCatalog()

	require.NoError(t, detachCatalogEntry(t.Context(), newCatalogFakeClient(), catalog, "missing"))
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
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1.SchemeGroupVersion})
	restMapper.Add(v1.SchemeGroupVersion.WithKind("MCPCatalog"), meta.RESTScopeNamespace)
	restMapper.Add(v1.SchemeGroupVersion.WithKind("MCPServerCatalogEntry"), meta.RESTScopeNamespace)
	return fake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithRESTMapper(restMapper).
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

func (c *serverListCountingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*v1.MCPServerList); ok {
		c.serverListCalls++
	}
	return c.Client.List(ctx, list, opts...)
}
