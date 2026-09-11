package mcpcatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	yamlSyntaxError = "this line has no colon and is not valid yaml\n"
)

type parseTestResponse struct {
	router.Response
}

func (*parseTestResponse) RetryAfter(time.Duration) {}

func writeParseTestManifest(t *testing.T, dir, name, description string) string {
	t.Helper()

	path := filepath.Join(dir, name+".yaml")
	content := fmt.Sprintf("name: %s\nshortDescription: Test\ndescription: %s\nicon: icon\nruntime: npx\nnpxConfig:\n  package: test\n", name, description)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func corruptParseTestManifest(t *testing.T, path string) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(content, []byte(yamlSyntaxError)...), 0o600))
}

func newParseTestHandler(t *testing.T) *Handler {
	t.Helper()

	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)

	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	return &Handler{gatewayClient: client}
}

func TestCatalogParseFailureSyncLifecycle(t *testing.T) {
	dir, healthyDir := t.TempDir(), t.TempDir()
	referencedPath := writeParseTestManifest(t, dir, "Referenced", "original")
	unreferencedPath := writeParseTestManifest(t, dir, "Unreferenced", "original")
	writeParseTestManifest(t, dir, "Sibling", "original")
	writeParseTestManifest(t, healthyDir, "Healthy", "original")

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir, healthyDir}
	client := newCatalogFakeClient(catalog)
	handler := newParseTestHandler(t)

	syncCatalog := func() *v1.MCPCatalog {
		t.Helper()

		current := &v1.MCPCatalog{}
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), current))

		if current.Annotations == nil {
			current.Annotations = map[string]string{}
		}
		current.Annotations[v1.MCPCatalogSyncAnnotation] = "true"
		require.NoError(t, client.Update(t.Context(), current))

		require.NoError(t, handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: current}, &parseTestResponse{}))

		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), current))
		require.False(t, current.Status.IsSyncing)

		return current
	}

	getEntry := func(name string) v1.MCPServerCatalogEntry {
		t.Helper()

		var entries v1.MCPServerCatalogEntryList
		require.NoError(t, client.List(t.Context(), &entries))

		for _, entry := range entries.Items {
			if entry.Spec.Manifest.Name == name {
				return entry
			}
		}

		t.Fatalf("entry %s is missing", name)
		return v1.MCPServerCatalogEntry{}
	}

	require.Empty(t, syncCatalog().Status.SyncErrors)
	referenced, unreferenced := getEntry("Referenced"), getEntry("Unreferenced")
	require.NotEmpty(t, referenced.OwnerReferences)

	server := &v1.MCPServer{
		Name:      "test-server",
		Namespace: catalog.Namespace,
		Spec:      v1.MCPServerSpec{MCPServerCatalogEntryName: referenced.Name},
	}
	require.NoError(t, client.Create(t.Context(), server))

	corruptParseTestManifest(t, referencedPath)
	corruptParseTestManifest(t, unreferencedPath)
	writeParseTestManifest(t, dir, "Sibling", "updated")
	writeParseTestManifest(t, healthyDir, "Healthy", "updated")
	writeParseTestManifest(t, healthyDir, "New", "new")

	errors := syncCatalog().Status.SyncErrors
	require.Len(t, errors, 1)
	require.Contains(t, errors[dir], "Referenced.yaml")
	require.Contains(t, errors[dir], "Unreferenced.yaml")

	require.Equal(t, referenced, getEntry("Referenced"))
	require.Equal(t, unreferenced, getEntry("Unreferenced"))

	require.Equal(t, "updated", getEntry("Sibling").Spec.Manifest.Description)
	require.Equal(t, "updated", getEntry("Healthy").Spec.Manifest.Description)
	require.Equal(t, "new", getEntry("New").Spec.Manifest.Description)

	var currentServer v1.MCPServer
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &currentServer))
	require.Equal(t, *server, currentServer)

	writeParseTestManifest(t, dir, "Referenced", "repaired")
	writeParseTestManifest(t, dir, "Unreferenced", "repaired")

	require.Empty(t, syncCatalog().Status.SyncErrors)
	require.Equal(t, "repaired", getEntry("Referenced").Spec.Manifest.Description)
	require.Equal(t, "repaired", getEntry("Unreferenced").Spec.Manifest.Description)
	require.False(t, getEntry("Referenced").Spec.Detached)

	require.NoError(t, os.Remove(referencedPath))
	require.NoError(t, os.Remove(unreferencedPath))

	require.Empty(t, syncCatalog().Status.SyncErrors)
	detached := getEntry("Referenced")
	require.True(t, detached.Spec.Detached)
	require.Empty(t, detached.OwnerReferences)

	var deleted v1.MCPServerCatalogEntry
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(&unreferenced), &deleted)))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(server), &currentServer))
	require.Equal(t, *server, currentServer)
}

func TestSystemCatalogParseFailureSyncLifecycle(t *testing.T) {
	dir := t.TempDir()
	brokenPath := writeParseTestManifest(t, dir, "Broken", "original")
	writeParseTestManifest(t, dir, "Sibling", "original")

	catalog := &v1.SystemMCPCatalog{
		APIVersion: v1.SchemeGroupVersion.String(),
		Kind:       "SystemMCPCatalog",
		Name:       "default",
		Namespace:  "default",
		UID:        "system-catalog-uid",
		Spec:       v1.SystemMCPCatalogSpec{SourceURLs: []string{dir}},
	}
	client := newCatalogFakeClient(catalog)
	handler := newParseTestHandler(t)

	syncCatalog := func() *v1.SystemMCPCatalog {
		t.Helper()

		current := &v1.SystemMCPCatalog{}
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), current))

		if current.Annotations == nil {
			current.Annotations = map[string]string{}
		}
		current.Annotations[v1.SystemMCPCatalogSyncAnnotation] = "true"
		require.NoError(t, client.Update(t.Context(), current))

		require.NoError(t, handler.SyncSystem(router.Request{Ctx: t.Context(), Client: client, Object: current}, &parseTestResponse{}))

		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), current))
		require.False(t, current.Status.IsSyncing)

		return current
	}

	getEntry := func(name string) v1.SystemMCPServerCatalogEntry {
		t.Helper()

		var entries v1.SystemMCPServerCatalogEntryList
		require.NoError(t, client.List(t.Context(), &entries))

		for _, entry := range entries.Items {
			if entry.Spec.Manifest.Name == name {
				return entry
			}
		}

		t.Fatalf("entry %s is missing", name)
		return v1.SystemMCPServerCatalogEntry{}
	}

	require.Empty(t, syncCatalog().Status.SyncErrors)
	original := getEntry("Broken")
	require.NotEmpty(t, original.OwnerReferences)

	corruptParseTestManifest(t, brokenPath)
	writeParseTestManifest(t, dir, "Sibling", "updated")

	errors := syncCatalog().Status.SyncErrors
	require.Len(t, errors, 1)
	require.Contains(t, errors[dir], "Broken.yaml")

	require.Equal(t, original, getEntry("Broken"))

	require.Equal(t, "updated", getEntry("Sibling").Spec.Manifest.Description)

	writeParseTestManifest(t, dir, "Broken", "repaired")

	require.Empty(t, syncCatalog().Status.SyncErrors)
	require.Equal(t, "repaired", getEntry("Broken").Spec.Manifest.Description)

	require.NoError(t, os.Remove(brokenPath))

	require.Empty(t, syncCatalog().Status.SyncErrors)

	var deleted v1.SystemMCPServerCatalogEntry
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(&original), &deleted)))
}
