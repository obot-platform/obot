package mcpservercatalogentry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeCredentialClient counts the queries a reconcile issues, so that "this entry costs nothing
// to reconcile" is an assertion rather than something inferred from the absence of a panic.
type fakeCredentialClient struct {
	// exists is whether the store holds a credential for the entry under test.
	exists    bool
	revealErr error
	deleteErr error

	reveals int
	deletes int
}

func TestDetectCompositeDriftMarksEntryNeedingUpdateWhenMultiUserComponentDrifts(t *testing.T) {
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:           "Shared Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{
				{
					MCPServerID: "shared-server",
					Manifest:    componentSnapshot,
				},
			},
		},
	})
	sharedServer := newMCPServer("shared-server", types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:2.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	client := newFakeClient(compositeEntry, sharedServer)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.True(t, updated.Status.NeedsUpdate)
}

func TestDetectCompositeDriftIgnoresCatalogOnlyComponentFields(t *testing.T) {
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:    "Catalog Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{{
				CatalogEntryID: "component-entry",
				Manifest:       componentSnapshot,
			}},
		},
	})
	compositeEntry.Status.NeedsUpdate = true
	componentEntry := newMCPServerCatalogEntry("component-entry", types.MCPServerCatalogEntryManifest{
		EntryKey:       "catalog-only-entry-key",
		Name:           "Catalog Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeSingleUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	client := newFakeClient(compositeEntry, componentEntry)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.False(t, updated.Status.NeedsUpdate)
}

func TestDetectCompositeDriftIgnoresComponentUpgradeNote(t *testing.T) {
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:    "Catalog Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{{
				CatalogEntryID: "component-entry",
				Manifest:       componentSnapshot,
			}},
		},
	})
	componentEntry := newMCPServerCatalogEntry("component-entry", componentSnapshot)
	componentEntry.Spec.Manifest.UpgradeNote = "Review settings before upgrading."

	client := newFakeClient(compositeEntry, componentEntry)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.False(t, updated.Status.NeedsUpdate)
}

func TestDetectCompositeDriftIgnoresAdminAddedSecretBindings(t *testing.T) {
	binding := &types.MCPSecretBinding{Name: "admin-secret", Key: "api-key", AdminAdded: true}
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:           "Shared Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
		Env: []types.MCPEnv{{
			Key:       "API_KEY",
			Name:      "API Key",
			Required:  true,
			Sensitive: true}},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{{
				MCPServerID: "shared-server",
				Manifest:    componentSnapshot,
			}},
		},
	})
	compositeEntry.Status.NeedsUpdate = true
	sharedServer := newMCPServer("shared-server", types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
		Env: []types.MCPEnv{{
			Key:           "API_KEY",
			Name:          "API Key",
			Required:      true,
			Sensitive:     true,
			SecretBinding: binding}},
	})
	client := newFakeClient(compositeEntry, sharedServer)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.False(t, updated.Status.NeedsUpdate)
}

func TestDetectCompositeDriftClearsEntryWhenMultiUserComponentMatches(t *testing.T) {
	componentSnapshot := types.MCPServerCatalogEntryManifest{
		Name:           "Shared Component",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	}
	compositeEntry := newMCPServerCatalogEntry("composite-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Composite Entry",
		Runtime: types.RuntimeComposite,
		CompositeConfig: &types.CompositeCatalogConfig{
			ComponentServers: []types.CatalogComponentServer{
				{
					MCPServerID: "shared-server",
					Manifest:    componentSnapshot,
				},
			},
		},
	})
	compositeEntry.Status.NeedsUpdate = true
	sharedServer := newMCPServer("shared-server", types.MCPServerManifest{
		Name:    "Shared Component",
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/component:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	client := newFakeClient(compositeEntry, sharedServer)
	err := (&Handler{}).DetectCompositeDrift(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    compositeEntry,
		Namespace: compositeEntry.Namespace,
		Name:      compositeEntry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(compositeEntry.Namespace, compositeEntry.Name), &updated))
	assert.False(t, updated.Status.NeedsUpdate)
}

func newFakeClient(objects ...kclient.Object) kclient.WithWatch {
	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPServerCatalogEntry{}).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			server := obj.(*v1.MCPServer)
			if server.Spec.MCPServerCatalogEntryName == "" {
				return nil
			}
			return []string{server.Spec.MCPServerCatalogEntryName}
		}).
		WithObjects(objects...).
		Build()
}

func newMCPServerCatalogEntry(name string, manifest types.MCPServerCatalogEntryManifest) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		APIVersion: v1.SchemeGroupVersion.String(),
		Kind:       "MCPServerCatalogEntry",
		Name:       name,
		Namespace:  "default",
		Spec: v1.MCPServerCatalogEntrySpec{
			Manifest: manifest,
		},
	}
}

func TestEnsureUserCountMultiUserEntry(t *testing.T) {
	entry := newMCPServerCatalogEntry("multi-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Multi User Template",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	server1 := newMCPServer("server-1", types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
		},
	})
	server1.Spec.MCPServerCatalogEntryName = entry.Name
	server1.Spec.UserID = "admin1"
	server1.Status.MCPServerInstanceUserCount = new(2)

	server2 := newMCPServer("server-2", types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
		},
	})
	server2.Spec.MCPServerCatalogEntryName = entry.Name
	server2.Spec.UserID = "admin2"
	server2.Status.MCPServerInstanceUserCount = new(1)

	client := newFakeClient(entry, server1, server2)
	err := (&Handler{}).EnsureUserCount(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &updated))
	assert.Equal(t, 3, updated.Status.UserCount, "should sum server instance user counts across servers")
}

func TestEnsureUserCountMultiUserEntryExcludesComposite(t *testing.T) {
	entry := newMCPServerCatalogEntry("multi-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Multi User Template",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeMultiUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	activeServer := newMCPServer("active-server", types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
		},
	})
	activeServer.Spec.MCPServerCatalogEntryName = entry.Name
	activeServer.Spec.UserID = "admin1"
	activeServer.Status.MCPServerInstanceUserCount = new(1)

	compositeChild := newMCPServer("composite-child", types.MCPServerManifest{
		Runtime: types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
		},
	})
	compositeChild.Spec.MCPServerCatalogEntryName = entry.Name
	compositeChild.Spec.UserID = "admin2"
	compositeChild.Spec.CompositeName = "parent-composite"
	compositeChild.Status.MCPServerInstanceUserCount = new(1)

	client := newFakeClient(entry, activeServer, compositeChild)
	err := (&Handler{}).EnsureUserCount(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &updated))
	assert.Equal(t, 1, updated.Status.UserCount, "should only count active non-composite servers")
}

func TestEnsureUserCountSingleUserEntryCountsUniqueServerUsers(t *testing.T) {
	entry := newMCPServerCatalogEntry("single-entry", types.MCPServerCatalogEntryManifest{
		Name:           "Single User Template",
		Runtime:        types.RuntimeContainerized,
		ServerUserType: types.ServerUserTypeSingleUser,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/mcp:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})

	server1 := newMCPServer("server-1", types.MCPServerManifest{Runtime: types.RuntimeContainerized})
	server1.Spec.MCPServerCatalogEntryName = entry.Name
	server1.Spec.UserID = "user1"

	server2 := newMCPServer("server-2", types.MCPServerManifest{Runtime: types.RuntimeContainerized})
	server2.Spec.MCPServerCatalogEntryName = entry.Name
	server2.Spec.UserID = "user1"

	server3 := newMCPServer("server-3", types.MCPServerManifest{Runtime: types.RuntimeContainerized})
	server3.Spec.MCPServerCatalogEntryName = entry.Name
	server3.Spec.UserID = "user2"

	client := newFakeClient(entry, server1, server2, server3)
	err := (&Handler{}).EnsureUserCount(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &updated))
	assert.Equal(t, 2, updated.Status.UserCount, "should only count active non-composite server")
}

func newMCPServer(name string, manifest types.MCPServerManifest) *v1.MCPServer {
	return &v1.MCPServer{
		APIVersion: v1.SchemeGroupVersion.String(),
		Kind:       "MCPServer",
		Name:       name,
		Namespace:  "default",
		Spec: v1.MCPServerSpec{
			Manifest: manifest,
		},
	}
}

func (f *fakeCredentialClient) RevealCredential(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
	f.reveals++
	if f.revealErr != nil {
		return gatewaytypes.Credential{}, f.revealErr
	}
	if !f.exists {
		return gatewaytypes.Credential{}, gclient.CredentialNotFoundError{Contexts: contexts, Name: name}
	}
	return gatewaytypes.Credential{Context: contexts[0], Name: name}, nil
}

func (f *fakeCredentialClient) DeleteCredential(_ context.Context, _, _ string) (bool, error) {
	f.deletes++
	if f.deleteErr != nil {
		return false, f.deleteErr
	}
	deleted := f.exists
	f.exists = false
	return deleted, nil
}

func remoteEntry(staticOAuthRequired bool) *v1.MCPServerCatalogEntry {
	return newMCPServerCatalogEntry("remote-entry", types.MCPServerCatalogEntryManifest{
		Name:    "Remote Template",
		Runtime: types.RuntimeRemote,
		RemoteConfig: &types.RemoteCatalogConfig{
			FixedURL:            "https://example.com/mcp",
			StaticOAuthRequired: staticOAuthRequired,
		},
	})
}

// runOAuthReconcile runs the handler against a fake API and returns the entry as it was
// left in storage.
func runOAuthReconcile(t *testing.T, creds *fakeCredentialClient, entry *v1.MCPServerCatalogEntry) (v1.MCPServerCatalogEntry, error) {
	t.Helper()

	client := newFakeClient(entry)
	err := reconcileOAuthCredential(router.Request{
		Client:    client,
		Ctx:       t.Context(),
		Object:    entry,
		Namespace: entry.Namespace,
		Name:      entry.Name,
	}, creds)

	var stored v1.MCPServerCatalogEntry
	require.NoError(t, client.Get(t.Context(), router.Key(entry.Namespace, entry.Name), &stored))
	return stored, err
}

func TestReconcileOAuthCredential(t *testing.T) {
	tests := []struct {
		name string
		// entry is the object as the controller sees it at the start of the pass.
		entry func() *v1.MCPServerCatalogEntry
		// exists is whether the credential store already holds a credential for it.
		exists         bool
		wantReveals    int
		wantDeletes    int
		wantConfigured bool
	}{
		{
			// The steady state for most of a catalog, and the case this handler exists to keep
			// free: a remote entry that never had a static OAuth credential.
			name:  "entry with no credential recorded is left alone",
			entry: func() *v1.MCPServerCatalogEntry { return remoteEntry(false) },
		},
		{
			// The other steady state: a configured entry that still requires static OAuth. The
			// status already answers the question, so the store is not read.
			name: "configured entry that still requires static OAuth is left alone",
			entry: func() *v1.MCPServerCatalogEntry {
				e := remoteEntry(true)
				e.Status.OAuthCredentialConfigured = true
				return e
			},
			exists:         true,
			wantConfigured: true,
		},
		{
			name: "credential is deleted once the entry stops requiring static OAuth",
			entry: func() *v1.MCPServerCatalogEntry {
				e := remoteEntry(false)
				e.Status.OAuthCredentialConfigured = true
				return e
			},
			exists:      true,
			wantDeletes: 1,
		},
		{
			// An entry converted away from the remote runtime keeps its credential under the same
			// name, and nothing else would ever remove it.
			name: "credential is deleted once the entry stops being remote",
			entry: func() *v1.MCPServerCatalogEntry {
				e := newMCPServerCatalogEntry("converted-entry", types.MCPServerCatalogEntryManifest{
					Name:    "Converted Template",
					Runtime: types.RuntimeContainerized,
					ContainerizedConfig: &types.ContainerizedRuntimeConfig{
						Image: "example/mcp:1.0.0",
					},
				})
				e.Status.OAuthCredentialConfigured = true
				return e
			},
			exists:      true,
			wantDeletes: 1,
		},
		{
			// SetOAuthCredentials writes the credential before it sets the annotation, so a
			// failure in between leaves a credential the controller has no record of. Reading the
			// store whenever the status is clear is what repairs that.
			name:           "credential written without the annotation landing is recorded",
			entry:          func() *v1.MCPServerCatalogEntry { return remoteEntry(true) },
			exists:         true,
			wantReveals:    1,
			wantConfigured: true,
		},
		{
			name:        "entry that requires static OAuth but has no credential stays unconfigured",
			entry:       func() *v1.MCPServerCatalogEntry { return remoteEntry(true) },
			wantReveals: 1,
		},
		{
			// How a delete through the API reaches the status: the annotation forces a read of a
			// store the status would otherwise be trusted for.
			name: "sync annotation rechecks an entry recorded as configured",
			entry: func() *v1.MCPServerCatalogEntry {
				e := remoteEntry(true)
				e.Status.OAuthCredentialConfigured = true
				e.Annotations = map[string]string{v1.MCPServerCatalogEntrySyncAnnotation: "true"}
				return e
			},
			wantReveals: 1,
		},
		{
			// The race the annotation covers on the cleanup side: the manifest stops requiring
			// static OAuth between the API writing a credential and this handler observing it, so
			// the status is clear even though a credential exists.
			name: "sync annotation cleans up a credential the status never recorded",
			entry: func() *v1.MCPServerCatalogEntry {
				e := remoteEntry(false)
				e.Annotations = map[string]string{v1.MCPServerCatalogEntrySyncAnnotation: "true"}
				return e
			},
			exists:      true,
			wantDeletes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := tt.entry()
			_, hadAnnotation := entry.Annotations[v1.MCPServerCatalogEntrySyncAnnotation]
			creds := &fakeCredentialClient{exists: tt.exists}

			stored, err := runOAuthReconcile(t, creds, entry)
			require.NoError(t, err)

			assert.Equal(t, tt.wantReveals, creds.reveals, "credential reads")
			assert.Equal(t, tt.wantDeletes, creds.deletes, "credential deletes")
			assert.Equal(t, tt.wantConfigured, stored.Status.OAuthCredentialConfigured)
			if hadAnnotation {
				assert.NotContains(t, stored.Annotations, v1.MCPServerCatalogEntrySyncAnnotation,
					"a completed recheck should clear the annotation")
			}
		})
	}
}

// The status is the only record that a credential exists, so it must not be cleared on a pass
// where the delete did not happen. nah runs every handler registered for a type even after an
// earlier one returns an error, which is why the delete and the status update have to be one
// handler rather than two.
func TestReconcileOAuthCredentialKeepsStatusWhenDeleteFails(t *testing.T) {
	entry := remoteEntry(false)
	entry.Status.OAuthCredentialConfigured = true
	creds := &fakeCredentialClient{exists: true, deleteErr: errors.New("connection refused")}

	stored, err := runOAuthReconcile(t, creds, entry)

	require.ErrorContains(t, err, "connection refused")
	assert.Equal(t, 1, creds.deletes)
	assert.True(t, stored.Status.OAuthCredentialConfigured,
		"status must survive a failed delete so the next pass retries it")
}

// Same reasoning for the annotation: it is the only signal that the status cannot be trusted, so
// a pass that fails has to leave it in place.
func TestReconcileOAuthCredentialKeepsSyncAnnotationWhenRecheckFails(t *testing.T) {
	entry := remoteEntry(true)
	entry.Status.OAuthCredentialConfigured = true
	entry.Annotations = map[string]string{v1.MCPServerCatalogEntrySyncAnnotation: "true"}
	creds := &fakeCredentialClient{revealErr: errors.New("connection refused")}

	stored, err := runOAuthReconcile(t, creds, entry)

	require.ErrorContains(t, err, "connection refused")
	assert.Contains(t, stored.Annotations, v1.MCPServerCatalogEntrySyncAnnotation,
		"annotation must survive a failed recheck so the next pass retries it")
}

// The sync annotation is the only sign a credential exists on an entry the controller has not
// reconciled since the API wrote one, so the finalizer has to honor it too. Otherwise an entry
// that is set up, converted away from remote, and deleted before any reconcile runs takes its
// credential with it and nothing is left to delete it.
func TestRemoveOAuthCredentialsOnDeletedEntry(t *testing.T) {
	converted := func() *v1.MCPServerCatalogEntry {
		return newMCPServerCatalogEntry("converted-entry", types.MCPServerCatalogEntryManifest{
			Name:    "Converted Template",
			Runtime: types.RuntimeContainerized,
			ContainerizedConfig: &types.ContainerizedRuntimeConfig{
				Image: "example/mcp:1.0.0",
			},
		})
	}

	tests := []struct {
		name        string
		entry       func() *v1.MCPServerCatalogEntry
		wantDeletes int
	}{
		{
			name:        "remote entry is always swept",
			entry:       func() *v1.MCPServerCatalogEntry { return remoteEntry(false) },
			wantDeletes: 1,
		},
		{
			name:        "ordinary entry of another runtime is left alone",
			entry:       converted,
			wantDeletes: 0,
		},
		{
			name: "entry of another runtime recorded as configured is swept",
			entry: func() *v1.MCPServerCatalogEntry {
				e := converted()
				e.Status.OAuthCredentialConfigured = true
				return e
			},
			wantDeletes: 1,
		},
		{
			name: "entry of another runtime being deleted is swept",
			entry: func() *v1.MCPServerCatalogEntry {
				e := converted()
				e.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				e.Finalizers = []string{v1.MCPServerCatalogEntryFinalizer}
				return e
			},
			wantDeletes: 1,
		},
		{
			name: "entry of another runtime with a pending recheck is swept",
			entry: func() *v1.MCPServerCatalogEntry {
				e := converted()
				e.Annotations = map[string]string{v1.MCPServerCatalogEntrySyncAnnotation: "true"}
				return e
			},
			wantDeletes: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := tt.entry()
			creds := &fakeCredentialClient{exists: true}

			err := removeOAuthCredentials(router.Request{
				Client:    newFakeClient(entry),
				Ctx:       t.Context(),
				Object:    entry,
				Namespace: entry.Namespace,
				Name:      entry.Name,
			}, creds)

			require.NoError(t, err)
			assert.Equal(t, tt.wantDeletes, creds.deletes)
		})
	}
}
