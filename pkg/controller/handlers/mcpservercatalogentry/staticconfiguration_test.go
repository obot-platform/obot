package mcpservercatalogentry

import (
	"context"
	"testing"
	"time"

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
	// created maps each revision's credential name to when it was written.
	created map[string]time.Time
	deleted []string
}

func newFakeStaticConfigurationCredentials(revisions ...string) *fakeStaticConfigurationCredentials {
	f := &fakeStaticConfigurationCredentials{created: make(map[string]time.Time)}
	start := time.Now()
	for i, revision := range revisions {
		f.created[mcp.StaticConfigurationCredentialName(revision)] = start.Add(time.Duration(i) * time.Minute)
	}
	return f
}

func (f *fakeStaticConfigurationCredentials) ListCredentials(_ context.Context, opts gclient.ListCredentialsOptions) ([]gatewaytypes.Credential, error) {
	credentials := make([]gatewaytypes.Credential, 0, len(f.created))
	for name, created := range f.created {
		credentials = append(credentials, gatewaytypes.Credential{Context: opts.CredentialContexts[0], Name: name, CreatedAt: created})
	}
	return credentials, nil
}

func (f *fakeStaticConfigurationCredentials) DeleteCredential(_ context.Context, _, name string) (bool, error) {
	delete(f.created, name)
	f.deleted = append(f.deleted, name)
	return true, nil
}

func credentialNames(revisions ...string) []string {
	names := make([]string, 0, len(revisions))
	for _, revision := range revisions {
		names = append(names, mcp.StaticConfigurationCredentialName(revision))
	}
	return names
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

	credentials := newFakeStaticConfigurationCredentials("current", "server", "vmcp", "legacy", "unused")
	req := router.Request{Ctx: t.Context(), Client: newFakeClient(entry, server, vmcp, instance), Object: entry}

	require.NoError(t, removeStaticConfigurationCredentials(req, credentials))
	// The entry's own revision is not referenced once the entry is gone.
	assert.ElementsMatch(t, credentialNames("current", "unused"), credentials.deleted)
}

func TestCleanupStaticConfigurationDeletesSupersededRevisions(t *testing.T) {
	entry := newMCPServerCatalogEntry("entry1rotate", types.MCPServerCatalogEntryManifest{StaticConfigurationRevision: "current"})
	entry.Status.StaticConfigurationRevision = "old"
	server := &v1.MCPServer{
		Name:      "ms1rotate",
		Namespace: entry.Namespace,
		Spec: v1.MCPServerSpec{
			MCPServerCatalogEntryName: entry.Name,
			Manifest:                  types.MCPServerManifest{StaticConfigurationRevision: "pinned"},
		},
	}
	client := newFakeClient(entry, server)
	req := router.Request{Ctx: t.Context(), Client: client, Object: entry}
	// "pending" was written by an update that has not published its revision yet.
	credentials := newFakeStaticConfigurationCredentials("orphaned", "pinned", "old", "current", "pending")
	reconcile := func() {
		t.Helper()
		require.NoError(t, cleanupStaticConfiguration(req, credentials))
		// The controller framework saves the status the handler sets.
		require.NoError(t, client.Status().Update(t.Context(), entry))
	}

	reconcile()
	assert.ElementsMatch(t, credentialNames("orphaned", "old"), credentials.deleted)
	assert.Equal(t, "current", entry.Status.StaticConfigurationRevision)

	// Nothing is deleted again until the revision changes.
	credentials.deleted = nil
	reconcile()
	assert.Empty(t, credentials.deleted)

	// Removing static configuration supersedes the last published revision.
	entry.Spec.Manifest.StaticConfigurationRevision = ""
	require.NoError(t, client.Update(t.Context(), entry))
	reconcile()
	assert.ElementsMatch(t, credentialNames("current"), credentials.deleted)
	assert.Contains(t, credentials.created, mcp.StaticConfigurationCredentialName("pinned"))
	assert.Contains(t, credentials.created, mcp.StaticConfigurationCredentialName("pending"))
	assert.Empty(t, entry.Status.StaticConfigurationRevision)
}
