package mcpservercatalogentry

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStaticConfigurationCredentials struct {
	names   []string
	deleted []string
}

func (f *fakeStaticConfigurationCredentials) ListCredentials(_ context.Context, opts gclient.ListCredentialsOptions) ([]gatewaytypes.Credential, error) {
	credentials := make([]gatewaytypes.Credential, 0, len(f.names))
	for _, name := range f.names {
		credentials = append(credentials, gatewaytypes.Credential{Context: opts.CredentialContexts[0], Name: name})
	}
	return credentials, nil
}

func (f *fakeStaticConfigurationCredentials) DeleteCredential(_ context.Context, _, name string) (bool, error) {
	f.deleted = append(f.deleted, name)
	return true, nil
}

func TestRemoveStaticConfigurationCredentialsKeepsReferencedRevisions(t *testing.T) {
	entry := newMCPServerCatalogEntry("entry1static", types.MCPServerCatalogEntryManifest{StaticConfigurationRevision: "current"})
	server := &v1.MCPServer{
		Name:      "ms1static",
		Namespace: entry.Namespace,
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: entry.Name,
			Manifest:                  types.MCPServerManifest{StaticConfigurationRevision: "server"},
		},
	}
	snapshot := func(revision string) types.VMCPComponent {
		return types.VMCPComponent{
			MCPServerCatalogEntryID: entry.Name,
			CatalogEntry:            types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{StaticConfigurationRevision: revision}},
		}
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1static",
		Namespace: entry.Namespace,
		Spec:      v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{snapshot("vmcp")}}},
	}
	instance := &v1.VMCPInstance{
		Name:      "vi1static",
		Namespace: entry.Namespace,
		Spec:      v1.VMCPInstanceSpec{LegacyComponents: []types.VMCPComponent{snapshot("legacy")}},
	}

	credentials := &fakeStaticConfigurationCredentials{}
	for _, revision := range []string{"current", "server", "vmcp", "legacy", "unused"} {
		credentials.names = append(credentials.names, mcp.StaticConfigurationCredentialName(revision))
	}
	req := router.Request{Ctx: t.Context(), Client: newFakeClient(entry, server, vmcp, instance), Object: entry}

	require.NoError(t, removeStaticConfigurationCredentials(req, credentials))
	// The entry's own revision is not referenced once the entry is gone.
	assert.ElementsMatch(t, []string{
		mcp.StaticConfigurationCredentialName("current"),
		mcp.StaticConfigurationCredentialName("unused"),
	}, credentials.deleted)
}
