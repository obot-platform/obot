package mcpcatalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type syncProfileData struct {
	has bool
}

func (d *syncProfileData) ProfilesWithData(_ context.Context, _ string, profileIDs []string) ([]string, error) {
	if d.has {
		return profileIDs, nil
	}
	return nil, nil
}

func (*syncProfileData) DeleteProfileData(context.Context, string, []string) error {
	return nil
}

func TestCatalogSyncVMCPProfileIDs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(`type: entry
entryKey: search
name: Search
shortDescription: Search
description: Search
icon: icon
runtime: npx
npxConfig:
  package: search
`), 0o600))
	vmcpPath := filepath.Join(dir, "vmcp.yaml")
	write := func(profiles string) {
		t.Helper()
		require.NoError(t, os.WriteFile(vmcpPath, []byte(`type: vmcp
entryKey: bundle
displayName: Bundle
components:
  - name: Search
    id: search
    mcpServerCatalogEntryKey: search
profiles:
`+profiles), 0o600))
	}

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir}
	client := newCatalogFakeClient(catalog)
	handler := newParseTestHandler(t)
	data := &syncProfileData{}
	handler.profileData = data
	sync := func() (*v1.MCPCatalog, v1.VMCP) {
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
		var vmcps v1.VMCPList
		require.NoError(t, client.List(t.Context(), &vmcps))
		require.Len(t, vmcps.Items, 1)
		return current, vmcps.Items[0]
	}
	ids := func(vmcp v1.VMCP) map[string]string {
		result := map[string]string{}
		for _, profile := range vmcp.Spec.Manifest.Profiles {
			result[profile.Name] = profile.ID
		}
		return result
	}

	write(`  - name: everyone
    subjects:
      - type: selector
        id: "*"
  - name: pinned
    id: pinned-profile
    subjects:
      - type: group
        id: engineering
`)
	status, first := sync()
	require.Empty(t, status.Status.SyncErrors)
	require.Equal(t, map[string]string{
		"everyone": vmcpconfig.DerivedProfileID(first.Name, "everyone"),
		"pinned":   "pinned-profile",
	}, ids(first))

	// Syncing unchanged YAML keeps every ID.
	status, again := sync()
	require.Empty(t, status.Status.SyncErrors)
	require.Equal(t, ids(first), ids(again))

	// Renaming a profile with an explicit ID keeps it; renaming one without makes it a new profile.
	write(`  - name: all
    subjects:
      - type: selector
        id: "*"
  - name: engineering
    id: pinned-profile
    subjects:
      - type: group
        id: engineering
`)
	status, renamed := sync()
	require.Empty(t, status.Status.SyncErrors)
	require.Equal(t, map[string]string{
		"all":         vmcpconfig.DerivedProfileID(first.Name, "all"),
		"engineering": "pinned-profile",
	}, ids(renamed))

	// A new catalog vMCP must follow the wildcard rule.
	twoWildcards := `  - name: all
    subjects:
      - type: selector
        id: "*"
  - name: engineering
    id: pinned-profile
    subjects:
      - type: selector
        id: "*"
`
	write(twoWildcards)
	status, unchanged := sync()
	require.Len(t, status.Status.SyncErrors, 1)
	require.True(t, strings.Contains(status.Status.SyncErrors[dir], "both use the selector wildcard"), status.Status.SyncErrors[dir])
	require.Equal(t, ids(renamed), ids(unchanged))

	// A vMCP stored before the rule keeps syncing while its profiles' subjects are unchanged...
	unchanged.Spec.Manifest.Profiles[1].Subjects = []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}
	require.NoError(t, client.Update(t.Context(), &unchanged))
	status, _ = sync()
	require.Empty(t, status.Status.SyncErrors)

	// ...until a profile has data stored for it.
	data.has = true
	status, _ = sync()
	require.Len(t, status.Status.SyncErrors, 1)
	require.Contains(t, status.Status.SyncErrors[dir], "profile configuration or OAuth grants")
}
