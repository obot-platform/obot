package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogChildrenDoNotWatchCatalogForDeletion(t *testing.T) {
	entry := &MCPServerCatalogEntry{Spec: MCPServerCatalogEntrySpec{
		MCPCatalogName:       "default",
		PowerUserWorkspaceID: "workspace",
	}}
	entryRefs := entry.DeleteRefs()
	require.Len(t, entryRefs, 1)
	assert.IsType(t, &PowerUserWorkspace{}, entryRefs[0].ObjType)
	assert.Equal(t, "workspace", entryRefs[0].Name)

	server := &MCPServer{Spec: MCPServerSpec{
		MCPCatalogID:         "default",
		PowerUserWorkspaceID: "workspace",
		CompositeName:        "composite",
	}}
	for _, ref := range server.DeleteRefs() {
		_, isCatalogRef := ref.ObjType.(*MCPCatalog)
		assert.False(t, isCatalogRef)
	}

	_, implementsDeleteRefs := any(&SystemMCPServerCatalogEntry{}).(DeleteRefs)
	assert.False(t, implementsDeleteRefs)
}
