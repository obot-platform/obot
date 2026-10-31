package adminmcp

import (
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBuiltinServesAdmins(t *testing.T) {
	for name, role := range map[string]types.Role{"admin": types.RoleAdmin, "owner": types.RoleOwner} {
		t.Run(name, func(t *testing.T) {
			// The admin MCP setting is off: vMCP profiles, not the setting, govern the built-in server.
			session := connect(t, newTestServer(t, vmcpUser(role.Groups()...)).url, nil)
			catalogs, result := callTool[listCatalogsOutput](t, session, "list_catalogs", map[string]any{})
			require.False(t, result.IsError, resultText(result))
			require.Len(t, catalogs.Catalogs, 1)
			assert.Equal(t, "default", catalogs.Catalogs[0].ID)
		})
	}
}

func TestBuiltinRefusesNonAdmins(t *testing.T) {
	for name, role := range map[string]types.Role{
		"basic":           types.RoleBasic,
		"auditor":         types.RoleBasic | types.RoleAuditor,
		"power user":      types.RolePowerUser,
		"power user plus": types.RolePowerUserPlus,
	} {
		t.Run(name, func(t *testing.T) {
			// A vMCP's profiles can grant anyone access, so the server checks the caller's role itself.
			client := gomcp.NewClient(&gomcp.Implementation{Name: "test-agent", Version: "1.0.0"}, nil)
			_, err := client.Connect(t.Context(), &gomcp.StreamableClientTransport{Endpoint: newTestServer(t, vmcpUser(role.Groups()...)).url}, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "Forbidden")
		})
	}
}

func TestEnsureBuiltinEntry(t *testing.T) {
	ctx := t.Context()
	c := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: BuiltinEntryName}

	require.NoError(t, EnsureBuiltinEntry(ctx, c, true))
	var entry v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(ctx, key, &entry))
	assert.Equal(t, system.DefaultCatalog, entry.Spec.MCPCatalogName)
	assert.False(t, entry.Spec.Editable)
	assert.Empty(t, entry.Spec.SourceURL)
	assert.Equal(t, types.RuntimeBuiltin, entry.Spec.Manifest.Runtime)
	require.NotNil(t, entry.Spec.Manifest.BuiltinConfig)
	assert.Equal(t, BuiltinName, entry.Spec.Manifest.BuiltinConfig.Name)
	var toolNames []string
	for _, tool := range entry.Spec.Manifest.ToolPreview {
		toolNames = append(toolNames, tool.Name)
	}
	assert.Contains(t, toolNames, "list_catalogs")
	assert.Contains(t, toolNames, "validate_catalog_files")

	// An entry changed by hand is put back.
	entry.Spec.Editable = true
	entry.Spec.Manifest.Name = "Changed"
	entry.Spec.Manifest.ToolPreview = nil
	require.NoError(t, c.Update(ctx, &entry))
	require.NoError(t, EnsureBuiltinEntry(ctx, c, true))
	var restored v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(ctx, key, &restored))
	assert.False(t, restored.Spec.Editable)
	assert.Equal(t, "Obot Admin", restored.Spec.Manifest.Name)
	assert.Len(t, restored.Spec.Manifest.ToolPreview, len(toolNames))

	// An entry that's already current isn't written.
	require.NoError(t, EnsureBuiltinEntry(ctx, c, true))
	var unchanged v1.MCPServerCatalogEntry
	require.NoError(t, c.Get(ctx, key, &unchanged))
	assert.Equal(t, restored.ResourceVersion, unchanged.ResourceVersion)

	// The server flag turning the admin MCP server off removes the entry.
	require.NoError(t, EnsureBuiltinEntry(ctx, c, false))
	assert.True(t, apierrors.IsNotFound(c.Get(ctx, key, &entry)))
	require.NoError(t, EnsureBuiltinEntry(ctx, c, false))
}
