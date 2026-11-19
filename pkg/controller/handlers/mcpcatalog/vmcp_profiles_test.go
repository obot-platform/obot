package mcpcatalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// vmcpSyncFixture is a catalog directory with one catalog entry and one vMCP whose profiles a test writes, synced by
// the catalog handler.
type vmcpSyncFixture struct {
	dir      string
	vmcpPath string
	catalog  *v1.MCPCatalog
	client   kclient.WithWatch
	handler  *Handler
}

func newVMCPSyncFixture(t *testing.T) *vmcpSyncFixture {
	t.Helper()
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
	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir}
	return &vmcpSyncFixture{
		dir:      dir,
		vmcpPath: filepath.Join(dir, "vmcp.yaml"),
		catalog:  catalog,
		client:   newCatalogFakeClient(catalog),
		handler:  newParseTestHandler(t),
	}
}

// write writes the catalog vMCP with profiles, the YAML of its profiles list.
func (f *vmcpSyncFixture) write(t *testing.T, profiles string) {
	t.Helper()
	require.NoError(t, os.WriteFile(f.vmcpPath, []byte(`type: vmcp
entryKey: bundle
displayName: Bundle
components:
  - name: Search
    id: search
    mcpServerCatalogEntryKey: search
profiles:
`+profiles), 0o600))
}

// sync syncs the catalog, and returns its status and the one vMCP it holds.
func (f *vmcpSyncFixture) sync(t *testing.T) (*v1.MCPCatalog, v1.VMCP) {
	t.Helper()
	current := &v1.MCPCatalog{}
	require.NoError(t, f.client.Get(t.Context(), kclient.ObjectKeyFromObject(f.catalog), current))
	if current.Annotations == nil {
		current.Annotations = map[string]string{}
	}
	current.Annotations[v1.MCPCatalogSyncAnnotation] = "true"
	require.NoError(t, f.client.Update(t.Context(), current))
	require.NoError(t, f.handler.Sync(router.Request{Ctx: t.Context(), Client: f.client, Object: current}, &parseTestResponse{}))
	require.NoError(t, f.client.Get(t.Context(), kclient.ObjectKeyFromObject(f.catalog), current))
	var vmcps v1.VMCPList
	require.NoError(t, f.client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	return current, vmcps.Items[0]
}

func profileIDsByName(vmcp v1.VMCP) map[string]string {
	result := map[string]string{}
	for _, profile := range vmcp.Spec.Manifest.Profiles {
		result[profile.Name] = profile.ID
	}
	return result
}

func TestCatalogSyncVMCPProfileIDs(t *testing.T) {
	f := newVMCPSyncFixture(t)
	f.write(t, `  - name: everyone
    subjects:
      - type: selector
        id: "*"
  - name: pinned
    id: pinned-profile
    subjects:
      - type: group
        id: engineering
`)
	status, first := f.sync(t)
	require.Empty(t, status.Status.SyncErrors)
	require.Equal(t, map[string]string{
		"everyone": vmcpconfig.DerivedProfileID(first.Name, "everyone"),
		"pinned":   "pinned-profile",
	}, profileIDsByName(first))

	// Syncing unchanged YAML keeps every ID.
	status, again := f.sync(t)
	require.Empty(t, status.Status.SyncErrors)
	require.Equal(t, profileIDsByName(first), profileIDsByName(again))

	// Renaming a profile with an explicit ID keeps it; renaming one without makes it a new profile.
	f.write(t, `  - name: all
    subjects:
      - type: selector
        id: "*"
  - name: engineering
    id: pinned-profile
    subjects:
      - type: group
        id: engineering
`)
	status, renamed := f.sync(t)
	require.Empty(t, status.Status.SyncErrors)
	require.Equal(t, map[string]string{
		"all":         vmcpconfig.DerivedProfileID(first.Name, "all"),
		"engineering": "pinned-profile",
	}, profileIDsByName(renamed))

	// Several profiles may name every user, as they always could.
	f.write(t, `  - name: all
    subjects:
      - type: selector
        id: "*"
  - name: engineering
    id: pinned-profile
    subjects:
      - type: selector
        id: "*"
`)
	status, _ = f.sync(t)
	require.Empty(t, status.Status.SyncErrors)
}

// catalogAgent returns the principal of the agent with id, as its authenticator records it.
func catalogAgent(id uint) kuser.Info {
	extra := map[string][]string{}
	principal.RecordKind(extra, principal.KindAgent)
	uid := principal.Agent(id).String()
	return &kuser.DefaultInfo{Name: uid, UID: uid, Extra: extra}
}

// TestCatalogSyncVMCPAgentSubjects checks the agent subjects of catalog vMCPs: a profile may name an agent by ID, and
// grants it; an ID that names no agent of this deployment syncs, and grants nobody; and an agent named by two
// profiles, the agent wildcard, or an ID that is not an agent's canonical ID, is a sync error that keeps the vMCP as
// last synced.
func TestCatalogSyncVMCPAgentSubjects(t *testing.T) {
	f := newVMCPSyncFixture(t)
	f.write(t, `  - name: everyone
    subjects:
      - type: selector
        id: "*"
  - name: builder
    id: builder
    subjects:
      - type: agent
        id: "7"
`)
	status, synced := f.sync(t)
	require.Empty(t, status.Status.SyncErrors)
	require.Contains(t, synced.Spec.Manifest.Profiles[1].Subjects, types.Subject{Type: types.SubjectTypeAgent, ID: "7"})
	profile := vmcpconfig.AgentProfile(catalogAgent(7), synced.Spec.Manifest)
	require.NotNil(t, profile, "the catalog vMCP does not grant the agent it names")
	require.Equal(t, "builder", profile.ID)
	// The profile for every user grants no agent, and no other agent is named.
	require.Nil(t, vmcpconfig.AgentProfile(catalogAgent(8), synced.Spec.Manifest))

	// No agent of this deployment has the ID: the vMCP still syncs, and grants no agent.
	f.write(t, `  - name: everyone
    subjects:
      - type: selector
        id: "*"
  - name: builder
    id: builder
    subjects:
      - type: agent
        id: "999999"
`)
	status, synced = f.sync(t)
	require.Empty(t, status.Status.SyncErrors)
	require.Nil(t, vmcpconfig.AgentProfile(catalogAgent(7), synced.Spec.Manifest))

	for _, tt := range []struct {
		name     string
		profiles string
		wantErr  string
	}{
		{
			name: "an agent named by two profiles",
			profiles: `  - name: builder
    subjects:
      - type: agent
        id: "7"
  - name: reviewer
    subjects:
      - type: agent
        id: "7"
`,
			wantErr: `agent "7" is named by profiles "builder" and "reviewer"`,
		},
		{
			name: "the agent wildcard",
			profiles: `  - name: agents
    subjects:
      - type: agent
        id: "*"
`,
			wantErr: "agents must be named by ID",
		},
		{
			name: "an agent's storage encoding",
			profiles: `  - name: builder
    subjects:
      - type: agent
        id: "agent:7"
`,
			wantErr: "agent subject ID must be the decimal ID of an agent",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f.write(t, tt.profiles)
			status, kept := f.sync(t)
			require.Len(t, status.Status.SyncErrors, 1)
			require.Contains(t, status.Status.SyncErrors[f.dir], tt.wantErr)
			require.Equal(t, profileIDsByName(synced), profileIDsByName(kept))
		})
	}
}
