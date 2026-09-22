package mcpcatalog

import (
	"context"
	"fmt"
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

// Fail only the vMCP apply, after credential preparation has completed.
type vmcpApplyFailureClient struct {
	kclient.WithWatch
	failName string
}

func TestCatalogSyncVMCPFromDirectory(t *testing.T) {
	dir := t.TempDir()
	entry := `type: entry
entryKey: search
name: Search
shortDescription: Search
description: Search
icon: icon
runtime: npx
npxConfig:
  package: search
config:
  - key: TOKEN
    usage: env
    required: true
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(entry), 0o600))

	vmcpPath := filepath.Join(dir, "vmcp.yaml")
	vmcp := `type: vmcp
entryKey: bundle
displayName: Bundle
components:
  - name: Search
    mcpServerCatalogEntryKey: search
    configuration:
      - key: TOKEN
        policy: fixed
        value: secret
`
	require.NoError(t, os.WriteFile(vmcpPath, []byte(vmcp), 0o600))

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir}
	client := newCatalogFakeClient(catalog)
	handler := newParseTestHandler(t)

	sync := func() *v1.MCPCatalog {
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
		return current
	}

	require.Empty(t, sync().Status.SyncErrors)
	var entries v1.MCPServerCatalogEntryList
	require.NoError(t, client.List(t.Context(), &entries))
	require.Len(t, entries.Items, 1)

	var vmcps v1.VMCPList
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	first := vmcps.Items[0]
	require.Equal(t, dir, first.Spec.SourceURL)
	require.Nil(t, first.Spec.Adopted)
	require.True(t, catalogOwnsVMCP(&first, catalog.Name))
	require.Equal(t, entries.Items[0].Name, first.Spec.Manifest.Components[0].MCPServerCatalogEntryID)
	require.Equal(t, "Search", first.Spec.Manifest.Components[0].CatalogEntry.Manifest.Name)
	require.NotEmpty(t, first.Spec.Manifest.Components[0].ID)
	require.Empty(t, first.Spec.Manifest.Components[0].Configuration[0].Value)
	require.NotContains(t, first.Name, "/")

	credential, err := handler.gatewayClient.RevealCredential(t.Context(),
		[]string{vmcpconfig.StaticConfigurationCredentialContext(first.Name)},
		vmcpconfig.StaticConfigurationCredentialName(&first),
	)
	require.NoError(t, err)
	require.Equal(t, "secret", credential.Secrets[vmcpconfig.ConfigurationKey(first.Spec.Manifest.Components[0].ID, "TOKEN")])

	require.Empty(t, sync().Status.SyncErrors)
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	require.Equal(t, first.Name, vmcps.Items[0].Name)
	require.Equal(t, first.Spec.Manifest.Components[0].ID, vmcps.Items[0].Spec.Manifest.Components[0].ID)

	renamed := strings.Replace(vmcp, "name: Search", "name: Renamed Search", 1)
	renamed = strings.Replace(renamed, "        value: secret\n", "", 1)
	require.NoError(t, os.WriteFile(vmcpPath, []byte(renamed), 0o600))
	require.Empty(t, sync().Status.SyncErrors)
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Equal(t, first.Spec.Manifest.Components[0].ID, vmcps.Items[0].Spec.Manifest.Components[0].ID)
	credential, err = handler.gatewayClient.RevealCredential(t.Context(),
		[]string{vmcpconfig.StaticConfigurationCredentialContext(first.Name)},
		vmcpconfig.StaticConfigurationCredentialName(&vmcps.Items[0]))
	require.NoError(t, err)
	require.Equal(t, "secret", credential.Secrets[vmcpconfig.ConfigurationKey(first.Spec.Manifest.Components[0].ID, "TOKEN")])

	updatedEntry := strings.Replace(entry, "description: Search", "description: Updated search", 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(updatedEntry), 0o600))
	require.Empty(t, sync().Status.SyncErrors)
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Equal(t, "Updated search", vmcps.Items[0].Spec.Manifest.Components[0].CatalogEntry.Manifest.Description)
	require.Equal(t, first.Spec.Manifest.Components[0].ID, vmcps.Items[0].Spec.Manifest.Components[0].ID)

	// Simulate a migrated vMCP created before stable references were retained.
	vmcps.Items[0].Spec.ComponentCatalogReferences = nil
	vmcps.Items[0].Spec.Manifest.Components[0].ForceSingleUser = true
	require.NoError(t, client.Update(t.Context(), &vmcps.Items[0]))
	for _, entryName := range []string{"Renamed Entry", "Renamed Again"} {
		renamedEntry := strings.Replace(updatedEntry, "name: Search", "name: "+entryName, 1)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(renamedEntry), 0o600))
		require.Empty(t, sync().Status.SyncErrors)
		require.NoError(t, client.List(t.Context(), &vmcps))
		current := &vmcps.Items[0]
		component := current.Spec.Manifest.Components[0]
		require.Equal(t, first.Spec.Manifest.Components[0].ID, component.ID)
		require.NotEqual(t, first.Spec.Manifest.Components[0].MCPServerCatalogEntryID, component.MCPServerCatalogEntryID)
		require.True(t, component.ForceSingleUser)
		require.Equal(t, dir+"::search", current.Spec.ComponentCatalogReferences[component.ID])
		credential, err = handler.gatewayClient.RevealCredential(t.Context(),
			[]string{vmcpconfig.StaticConfigurationCredentialContext(first.Name)},
			vmcpconfig.StaticConfigurationCredentialName(current))
		require.NoError(t, err)
		require.Equal(t, "secret", credential.Secrets[vmcpconfig.ConfigurationKey(component.ID, "TOKEN")])
	}
	lastEntryID := vmcps.Items[0].Spec.Manifest.Components[0].MCPServerCatalogEntryID

	broken := `type: vmcp
entryKey: bundle
displayName: Bundle
components:
  - name: Search
    mcpServerCatalogEntryKey: missing
`
	require.NoError(t, os.WriteFile(vmcpPath, []byte(broken), 0o600))
	require.Contains(t, sync().Status.SyncErrors[dir], "was not found")
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	require.Equal(t, lastEntryID, vmcps.Items[0].Spec.Manifest.Components[0].MCPServerCatalogEntryID)

	require.NoError(t, os.Remove(vmcpPath))
	require.Empty(t, sync().Status.SyncErrors)
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	require.NotNil(t, vmcps.Items[0].DeletionTimestamp)
}

func TestCatalogSyncVMCPCrossSourceReference(t *testing.T) {
	vmcpDir, entryDir := t.TempDir(), t.TempDir()
	entry := `entryKey: search
name: Search
shortDescription: Search
description: Search
icon: icon
runtime: npx
npxConfig:
  package: search
`
	require.NoError(t, os.WriteFile(filepath.Join(entryDir, "entry.yaml"), []byte(entry), 0o600))

	vmcp := fmt.Sprintf(`type: vmcp
displayName: Bundle
components:
  - name: Search
    id: search-component
    mcpServerCatalogEntryKey: %q
profiles:
  - name: admins
    subjects:
      - type: obotGroup
        id: admin
    vmcpPermissions:
      allowedComponents:
        search-component: {}
`, entryDir+"::search")
	require.NoError(t, os.WriteFile(filepath.Join(vmcpDir, "vmcp.yaml"), []byte(vmcp), 0o600))

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{vmcpDir, entryDir}
	client := newCatalogFakeClient(catalog)
	handler := newParseTestHandler(t)

	require.NoError(t, handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: catalog}, &parseTestResponse{}))
	require.Empty(t, catalog.Status.SyncErrors)

	var vmcps v1.VMCPList
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	require.NotEmpty(t, vmcps.Items[0].Spec.Manifest.Components[0].CatalogEntry.Manifest.Name)
	require.Equal(t, "search-component", vmcps.Items[0].Spec.Manifest.Components[0].ID)
}

func TestCatalogSyncAdoptsMigratedVMCP(t *testing.T) {
	dir := t.TempDir()
	entry := `entryKey: search
name: Search
shortDescription: Search
description: Search
icon: icon
runtime: npx
npxConfig:
  package: search
`
	vmcp := `type: vmcp
entryKey: bundle
displayName: Bundle
components:
  - name: Search
    mcpServerCatalogEntryKey: search
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "entry.yaml"), []byte(entry), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vmcp.yaml"), []byte(vmcp), 0o600))

	catalog := testCatalog()
	catalog.Spec.SourceURLs = []string{dir}
	objects, err := (&Handler{}).readMCPCatalog(t.Context(), catalog.Name, dir, "")
	require.NoError(t, err)
	var desired *v1.VMCP
	for _, object := range objects {
		if candidate, ok := object.(*v1.VMCP); ok {
			desired = candidate
		}
	}
	require.NotNil(t, desired)

	migrated := &v1.VMCP{
		Name:      desired.Name,
		Namespace: catalog.Namespace,
		Spec: v1.VMCPSpec{
			LegacySlug: "old-slug",
			Adopted:    new(false),
			Manifest: types.VMCPManifest{
				DisplayName: "Bundle",
				Components: []types.VMCPComponent{{
					ID:                      "original-component",
					Name:                    "Old Search",
					ForceSingleUser:         true,
					MCPServerCatalogEntryID: objects[0].GetName(),
				}},
			},
		},
	}
	client := newCatalogFakeClient(catalog, migrated)
	handler := newParseTestHandler(t)
	require.NoError(t, handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: catalog}, &parseTestResponse{}))
	require.Empty(t, catalog.Status.SyncErrors)

	var vmcps v1.VMCPList
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	require.Equal(t, migrated.Name, vmcps.Items[0].Name)
	require.Equal(t, "original-component", vmcps.Items[0].Spec.Manifest.Components[0].ID)
	require.True(t, vmcps.Items[0].Spec.Manifest.Components[0].ForceSingleUser)
	require.Equal(t, "old-slug", vmcps.Items[0].Spec.LegacySlug)
	require.True(t, catalogOwnsVMCP(&vmcps.Items[0], catalog.Name))
	require.Equal(t, new(true), vmcps.Items[0].Spec.Adopted)
	require.Equal(t, dir, vmcps.Items[0].Spec.SourceURL)

	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), catalog))
	catalog.Annotations[v1.MCPCatalogSyncAnnotation] = "true"
	require.NoError(t, client.Update(t.Context(), catalog))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vmcp.yaml"), []byte(strings.Replace(vmcp, "displayName: Bundle", "displayName: Renamed", 1)), 0o600))
	require.NoError(t, handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: catalog}, &parseTestResponse{}))
	require.Empty(t, catalog.Status.SyncErrors)
	require.NoError(t, client.List(t.Context(), &vmcps))
	require.Len(t, vmcps.Items, 1)
	require.Equal(t, migrated.Name, vmcps.Items[0].Name)
	require.Equal(t, "Renamed", vmcps.Items[0].Spec.Manifest.DisplayName)
	require.True(t, vmcps.Items[0].Spec.Manifest.Components[0].ForceSingleUser)
	require.Equal(t, new(true), vmcps.Items[0].Spec.Adopted)
}

func TestCatalogSyncDoesNotOverwriteUnmanagedVMCP(t *testing.T) {
	catalog := testCatalog()
	existing := &v1.VMCP{
		Name:      "vmcp1conflict",
		Namespace: catalog.Namespace,
		Spec: v1.VMCPSpec{
			LegacySlug: "local-composite",
			Manifest:   types.VMCPManifest{DisplayName: "Manual"},
		},
	}
	desired := &v1.VMCP{
		Name:      existing.Name,
		Namespace: catalog.Namespace,
		Spec: v1.VMCPSpec{
			SourceURL: "source",
			Manifest:  types.VMCPManifest{DisplayName: "Catalog"},
		},
	}

	objects, syncErrors, err := (&Handler{}).prepareCatalogVMCPs(t.Context(), newCatalogFakeClient(existing), catalog, []kclient.Object{desired})
	require.NoError(t, err)
	require.Empty(t, objects)
	require.Contains(t, syncErrors["source"], "conflicts")
}

func (c *vmcpApplyFailureClient) Patch(ctx context.Context, obj kclient.Object, patch kclient.Patch, opts ...kclient.PatchOption) error {
	if obj.GetName() == c.failName {
		return fmt.Errorf("injected vMCP apply failure")
	}
	return c.WithWatch.Patch(ctx, obj, patch, opts...)
}

func TestCatalogSyncPreservesCredentialsOnApplyFailure(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(fmt.Sprintf("remove configuration=%t", remove), func(t *testing.T) {
			dir := t.TempDir()
			content := `- type: entry
  entryKey: search
  name: Search
  shortDescription: Search
  description: Search
  icon: icon
  runtime: npx
  npxConfig:
    package: search
  config:
    - key: TOKEN
      usage: env
- type: vmcp
  entryKey: bundle
  displayName: Bundle
  components:
    - name: Search
      mcpServerCatalogEntryKey: search
      configuration:
        - key: TOKEN
          policy: fixed
          value: original-secret
`
			path := filepath.Join(dir, "catalog.yaml")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			catalog := testCatalog()
			catalog.Spec.SourceURLs = []string{dir}
			client := &vmcpApplyFailureClient{WithWatch: newCatalogFakeClient(catalog)}
			handler := newParseTestHandler(t)
			sync := func() error {
				t.Helper()
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), catalog))
				return handler.Sync(router.Request{Ctx: t.Context(), Client: client, Object: catalog}, &parseTestResponse{})
			}
			require.NoError(t, sync())
			var list v1.VMCPList
			require.NoError(t, client.List(t.Context(), &list))
			require.Len(t, list.Items, 1)
			original := list.Items[0]
			key := vmcpconfig.ConfigurationKey(original.Spec.Manifest.Components[0].ID, "TOKEN")
			read := func(vmcp *v1.VMCP) map[string]string {
				t.Helper()
				credential, err := handler.gatewayClient.RevealCredential(t.Context(),
					[]string{vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)},
					vmcpconfig.StaticConfigurationCredentialName(vmcp))
				require.NoError(t, err)
				return credential.Secrets
			}
			require.Equal(t, "original-secret", read(&original)[key])

			updated := strings.Replace(content, "value: original-secret", "value: replacement-secret", 1)
			if remove {
				updated = strings.Split(content, "      configuration:")[0]
			}
			require.NoError(t, os.WriteFile(path, []byte(updated), 0o600))
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(catalog), catalog))
			catalog.Annotations[v1.MCPCatalogSyncAnnotation] = "true"
			require.NoError(t, client.Update(t.Context(), catalog))
			client.failName = original.Name
			for range 2 {
				require.ErrorContains(t, sync(), "injected vMCP apply failure")
				var stored v1.VMCP
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&original), &stored))
				require.Equal(t, original.Spec, stored.Spec)
				require.Equal(t, "original-secret", read(&stored)[key])
			}

			client.failName = ""
			require.NoError(t, sync())
			var stored v1.VMCP
			require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(&original), &stored))
			if remove {
				require.Empty(t, read(&stored))
			} else {
				require.Equal(t, "replacement-secret", read(&stored)[key])
			}
			require.Equal(t, "original-secret", read(&original)[key])
		})
	}
}

func TestCatalogVMCPComponentReferenceMatching(t *testing.T) {
	for _, source := range []string{"source", "other-source"} {
		t.Run(source, func(t *testing.T) {
			existing := &v1.VMCP{Spec: v1.VMCPSpec{
				ComponentCatalogReferences: map[string]string{"stable-id": source + "::search"},
				Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
					ID:                      "stable-id",
					Name:                    "Search",
					MCPServerCatalogEntryID: "old-entry",
					ForceSingleUser:         true,
				}}},
			}}
			desired := &v1.VMCP{Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
				Name:                    "Search",
				MCPServerCatalogEntryID: "search",
			}}}}}
			entry := &v1.MCPServerCatalogEntry{Name: "renamed-entry"}
			require.NoError(t, resolveCatalogVMCPComponents(desired, existing, "source", map[string]*v1.MCPServerCatalogEntry{"source::search": entry}))
			component := desired.Spec.Manifest.Components[0]
			if source == "source" {
				require.Equal(t, "stable-id", component.ID)
				require.True(t, component.ForceSingleUser)
			} else {
				require.NotEqual(t, "stable-id", component.ID)
				require.False(t, component.ForceSingleUser)
			}
			require.Equal(t, "source::search", desired.Spec.ComponentCatalogReferences[component.ID])
		})
	}
}

func TestCatalogVMCPComponentMatching(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{Name: "entry-id"}
	entry.Spec.Manifest.Name = "Search"
	previous := types.VMCPComponent{
		ID:                      "stable-id",
		ForceSingleUser:         true,
		Name:                    "Old Name",
		MCPServerCatalogEntryID: entry.Name,
	}
	other := types.VMCPComponent{
		ID:                      "second-id",
		Name:                    "Other",
		MCPServerCatalogEntryID: entry.Name,
	}

	for _, tc := range []struct {
		name     string
		existing []types.VMCPComponent
		id       string
		wantErr  string
	}{
		{
			name:     "renamed component",
			existing: []types.VMCPComponent{previous},
		},
		{
			name:     "ambiguous reference",
			existing: []types.VMCPComponent{previous, other},
			wantErr:  "matches multiple existing components",
		},
		{
			name:     "explicit ID resolves ambiguous reference",
			existing: []types.VMCPComponent{previous, other},
			id:       "stable-id",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			existing := &v1.VMCP{Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: tc.existing}}}
			desired := &v1.VMCP{Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
				ID:                      tc.id,
				Name:                    "Renamed",
				MCPServerCatalogEntryID: "search",
			}}}}}
			err := resolveCatalogVMCPComponents(desired, existing, "source", map[string]*v1.MCPServerCatalogEntry{"source::search": entry})
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "stable-id", desired.Spec.Manifest.Components[0].ID)
			require.True(t, desired.Spec.Manifest.Components[0].ForceSingleUser)
		})
	}
}
