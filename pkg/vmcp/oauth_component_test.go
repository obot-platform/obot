package vmcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestReplacingComponentCannotReuseOAuthIdentity(t *testing.T) {
	existing := types.VMCPManifest{Components: []types.VMCPComponent{{ID: "original", Name: "server", MCPServerCatalogEntryID: "entry-one"}}}
	desired := types.VMCPManifest{Components: []types.VMCPComponent{{Name: "server", MCPServerCatalogEntryID: "entry-two"}}}
	require.NoError(t, ReconcileComponentIDs(existing, &desired))
	require.NotEqual(t, "original", desired.Components[0].ID)
	desired.Components[0].ID = "original"
	require.ErrorContains(t, ReconcileComponentIDs(existing, &desired), "different catalog entry")
}
