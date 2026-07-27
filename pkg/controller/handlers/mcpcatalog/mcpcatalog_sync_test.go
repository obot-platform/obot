package mcpcatalog

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSyncSkipsRecentCatalogAndForceSyncPrunesRemovedEntries(t *testing.T) {
	ctx := context.Background()
	catalogDir := t.TempDir()
	writeCatalogEntry(t, catalogDir, "alpha.yaml", "Alpha")
	writeCatalogEntry(t, catalogDir, "beta.yaml", "Beta")

	catalog := &v1.MCPCatalog{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "MCPCatalog",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultCatalog,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPCatalogSpec{
			DisplayName: "Default",
			SourceURLs:  []string{catalogDir},
		},
	}
	storageClient := newCatalogFakeClient(catalog)
	gatewayClient := newCatalogGatewayClient(t, storageClient)
	handler := &Handler{gatewayClient: gatewayClient}

	resp := &router.ResponseWrapper{}
	require.NoError(t, handler.Sync(syncRequest(ctx, storageClient, catalog), resp))
	require.ElementsMatch(t, []string{"Alpha", "Beta"}, catalogEntryManifestNames(ctx, t, storageClient))

	require.NoError(t, os.Remove(filepath.Join(catalogDir, "beta.yaml")))
	synced := getCatalog(ctx, t, storageClient)

	resp = &router.ResponseWrapper{}
	require.NoError(t, handler.Sync(syncRequest(ctx, storageClient, synced), resp))
	require.True(t, resp.Delay > 0 && resp.Delay <= time.Hour)
	require.ElementsMatch(t, []string{"Alpha", "Beta"}, catalogEntryManifestNames(ctx, t, storageClient))

	forceSync := getCatalog(ctx, t, storageClient)
	forceSync.Annotations[v1.MCPCatalogSyncAnnotation] = "true"
	require.NoError(t, storageClient.Update(ctx, forceSync))

	resp = &router.ResponseWrapper{}
	require.NoError(t, handler.Sync(syncRequest(ctx, storageClient, getCatalog(ctx, t, storageClient)), resp))
	require.Equal(t, time.Hour, resp.Delay)
	require.Equal(t, []string{"Alpha"}, catalogEntryManifestNames(ctx, t, storageClient))

	afterForce := getCatalog(ctx, t, storageClient)
	require.NotContains(t, afterForce.Annotations, v1.MCPCatalogSyncAnnotation)
}

func newCatalogFakeClient(objects ...kclient.Object) kclient.WithWatch {
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1.SchemeGroupVersion})
	restMapper.Add(v1.SchemeGroupVersion.WithKind("MCPCatalog"), meta.RESTScopeNamespace)
	restMapper.Add(v1.SchemeGroupVersion.WithKind("MCPServerCatalogEntry"), meta.RESTScopeNamespace)

	return fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithRESTMapper(restMapper).
		WithStatusSubresource(&v1.MCPCatalog{}, &v1.MCPServerCatalogEntry{}).
		WithObjects(objects...).
		Build()
}

func newCatalogGatewayClient(t *testing.T, storageClient kclient.WithWatch) *gatewayclient.Client {
	t.Helper()

	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	ctx, cancel := context.WithCancel(context.Background())
	client := gatewayclient.New(ctx, db, storageClient, nil, nil, nil, nil, time.Hour, 100, 1, 1, true)
	t.Cleanup(func() {
		cancel()
		require.NoError(t, client.Close())
	})
	return client
}

func syncRequest(ctx context.Context, storageClient kclient.WithWatch, catalog *v1.MCPCatalog) router.Request {
	return router.Request{
		Client:    storageClient,
		Object:    catalog,
		Ctx:       ctx,
		Namespace: catalog.Namespace,
		Name:      catalog.Name,
		Key:       catalog.Namespace + "/" + catalog.Name,
	}
}

func writeCatalogEntry(t *testing.T, dir, filename, name string) {
	t.Helper()

	content := `name: ` + name + `
description: ` + name + ` MCP
runtime: npx
serverUserType: multiUser
npxConfig:
  package: test-server
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o600))
}

func catalogEntryManifestNames(ctx context.Context, t *testing.T, storageClient kclient.Client) []string {
	t.Helper()

	var entries v1.MCPServerCatalogEntryList
	require.NoError(t, storageClient.List(ctx, &entries, kclient.InNamespace(system.DefaultNamespace)))
	names := make([]string, 0, len(entries.Items))
	for _, entry := range entries.Items {
		names = append(names, entry.Spec.Manifest.Name)
	}
	slices.Sort(names)
	return names
}

func getCatalog(ctx context.Context, t *testing.T, storageClient kclient.Client) *v1.MCPCatalog {
	t.Helper()

	var catalog v1.MCPCatalog
	require.NoError(t, storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultCatalog}, &catalog))
	if catalog.Annotations == nil {
		catalog.Annotations = map[string]string{}
	}
	return &catalog
}
