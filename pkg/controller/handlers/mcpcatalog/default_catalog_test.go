package mcpcatalog

import (
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestSetUpDefaultMCPCatalogRequestsSyncAfterMigratingSource(t *testing.T) {
	const newSource = "https://github.com/obot-platform/mcp-catalog/v2-schema"
	c := newCatalogFakeClient(&v1.MCPCatalog{
		Name:      system.DefaultCatalog,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPCatalogSpec{
			SourceURLs: []string{"https://github.com/obot-platform/mcp-catalog", "https://example.com/other"},
		},
	})

	require.NoError(t, (&Handler{defaultCatalogPath: newSource}).SetUpDefaultMCPCatalog(t.Context(), c))

	var catalog v1.MCPCatalog
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultCatalog}, &catalog))
	assert.Equal(t, []string{newSource, "https://example.com/other"}, catalog.Spec.SourceURLs)
	assert.Equal(t, "true", catalog.Annotations[v1.MCPCatalogSyncAnnotation])
}

func TestSetUpDefaultMCPCatalogDoesNotRequestSyncWithoutMigration(t *testing.T) {
	c := newCatalogFakeClient(&v1.MCPCatalog{
		Name:      system.DefaultCatalog,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPCatalogSpec{
			SourceURLs: []string{"https://github.com/obot-platform/mcp-catalog/v2-schema"},
		},
	})

	require.NoError(t, (&Handler{defaultCatalogPath: "https://github.com/obot-platform/mcp-catalog/v2-schema"}).SetUpDefaultMCPCatalog(t.Context(), c))

	var catalog v1.MCPCatalog
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultCatalog}, &catalog))
	assert.NotContains(t, catalog.Annotations, v1.MCPCatalogSyncAnnotation)
}

func TestSetUpDefaultSystemMCPCatalogRequestsSyncAfterMigratingSource(t *testing.T) {
	const newSource = "https://github.com/obot-platform/system-mcp-catalog/v2-schema"
	c := newCatalogFakeClient(&v1.SystemMCPCatalog{
		Name:      system.DefaultCatalog,
		Namespace: system.DefaultNamespace,
		Spec: v1.SystemMCPCatalogSpec{
			SourceURLs: []string{"https://github.com/obot-platform/system-mcp-catalog"},
		},
	})

	require.NoError(t, (&Handler{defaultSystemCatalogPath: newSource}).SetUpDefaultSystemMCPCatalog(t.Context(), c))

	var catalog v1.SystemMCPCatalog
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultCatalog}, &catalog))
	assert.Equal(t, []string{newSource}, catalog.Spec.SourceURLs)
	assert.Equal(t, "true", catalog.Annotations[v1.SystemMCPCatalogSyncAnnotation])
}
