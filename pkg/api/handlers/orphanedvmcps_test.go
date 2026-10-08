package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	orphanedTestSourceURL = "https://github.com/example/catalog"
	sharedTestSourceURL   = "https://github.com/example/shared/"
)

func TestListOrphanedVMCPsReturnsVMCPsAwaitingAdoption(t *testing.T) {
	email := orphanedVMCPForTest(orphanedTestSourceURL, "email", "Email", new(false))
	email.Spec.Manifest.Description = "Send email"
	email.Spec.Manifest.Components = []types.VMCPComponent{
		{
			ID:                      "default-gmail",
			Name:                    "Gmail",
			MCPCatalogID:            system.DefaultCatalog,
			MCPServerCatalogEntryID: "default-gmail",
			ForceSingleUser:         true,
			Configuration: []types.VMCPConfigurationPolicy{{
				Key:    "TOKEN",
				Policy: types.VMCPConfigurationPolicyFixed,
			}},
			ToolPrefix: "gmail_",
			ToolOverrides: []types.ToolOverride{{
				Name:         "send",
				OverrideName: "send_email",
			}},
		},
		{
			ID:                      "default-search",
			Name:                    "Search",
			MCPCatalogID:            system.DefaultCatalog,
			MCPServerCatalogEntryID: "default-search",
		},
	}
	email.Spec.Manifest.Profiles = []types.VMCPProfile{{
		Name:     "everyone",
		Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
		Permissions: types.VMCPProfilePermissions{
			AllowedComponents: map[string]types.VMCPComponentSet{
				"default-gmail": {AllowedTools: nil},
			},
		},
	}}

	broken := orphanedVMCPForTest(orphanedTestSourceURL, "broken", "Broken", new(false))
	broken.Spec.Manifest.Components = []types.VMCPComponent{{
		ID:                      "default-no-key",
		Name:                    "No Key",
		MCPServerCatalogEntryID: "default-no-key",
	}}

	storage := newVMCPTestStorage(
		orphanedTestCatalog(),
		syncedCatalogEntryForTest("default-gmail", orphanedTestSourceURL, "obot-gmail"),
		syncedCatalogEntryForTest("default-search", sharedTestSourceURL, "search"),
		syncedCatalogEntryForTest("default-no-key", orphanedTestSourceURL, ""),
		email,
		broken,
		// Already adopted, from another source, and never adoptable.
		orphanedVMCPForTest(orphanedTestSourceURL, "adopted", "Adopted", new(true)),
		orphanedVMCPForTest(sharedTestSourceURL, "other", "Other", new(false)),
		orphanedVMCPForTest(orphanedTestSourceURL, "local", "Local", nil),
	)

	// The source URL matches regardless of scheme and trailing slash.
	result := callListOrphanedVMCPs(t, storage, "http://github.com/example/catalog/")
	require.Len(t, result.Items, 2)

	require.Equal(t, broken.Name, result.Items[0].VMCPID)
	require.Empty(t, result.Items[0].YAML)
	require.Contains(t, result.Items[0].Error, `component "No Key" uses catalog entry "default-no-key" from https://github.com/example/catalog, which has no entryKey`)

	item := result.Items[1]
	require.Equal(t, types.OrphanedVMCPCatalogItem{
		VMCPID:      email.Name,
		DisplayName: "Email",
		EntryKey:    "email",
		YAML: `type: vmcp
entryKey: email
displayName: Email
description: Send email
components:
  - id: default-gmail
    name: Gmail
    mcpServerCatalogEntryKey: obot-gmail
    configuration:
      - key: TOKEN
        policy: fixed
    toolPrefix: gmail_
    toolOverrides:
      - name: send
        overrideName: send_email
  - id: default-search
    name: Search
    mcpServerCatalogEntryKey: github.com/example/shared::search
profiles:
  - name: everyone
    subjects:
      - type: selector
        id: '*'
    vmcpPermissions:
      allowedComponents:
        default-gmail:
          allowedTools: null
`,
	}, item)

	// Catalog sync must decode the YAML into the same vMCP identity, components, and profiles.
	var header struct {
		Type     string `json:"type"`
		EntryKey string `json:"entryKey"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(item.YAML), &header))
	require.Equal(t, "vmcp", header.Type)
	manifest, err := mcpcatalog.DecodeVMCPManifest([]byte(item.YAML))
	require.NoError(t, err)
	require.Equal(t, email.Name, mcpcatalog.VMCPName(system.DefaultCatalog, orphanedTestSourceURL, header.EntryKey, manifest.DisplayName))
	require.Equal(t, email.Spec.Manifest.Profiles, manifest.Profiles)
	require.Len(t, manifest.Components, 2)
	require.Equal(t, "default-gmail", manifest.Components[0].ID)
	require.Equal(t, "obot-gmail", manifest.Components[0].MCPServerCatalogEntryID)
	require.Equal(t, email.Spec.Manifest.Components[0].Configuration, manifest.Components[0].Configuration)
	require.Equal(t, email.Spec.Manifest.Components[0].ToolOverrides, manifest.Components[0].ToolOverrides)
	require.Equal(t, "default-search", manifest.Components[1].ID)
	require.Equal(t, "github.com/example/shared::search", manifest.Components[1].MCPServerCatalogEntryID)
}

func TestListOrphanedVMCPsKeepsEmptyProfiles(t *testing.T) {
	vmcp := orphanedVMCPForTest(orphanedTestSourceURL, "email", "Email", new(false))
	vmcp.Spec.Manifest.Components = []types.VMCPComponent{{
		ID:                      "default-gmail",
		Name:                    "Gmail",
		MCPServerCatalogEntryID: "default-gmail",
	}}
	storage := newVMCPTestStorage(
		orphanedTestCatalog(),
		syncedCatalogEntryForTest("default-gmail", orphanedTestSourceURL, "obot-gmail"),
		vmcp,
	)

	result := callListOrphanedVMCPs(t, storage, orphanedTestSourceURL)
	require.Len(t, result.Items, 1)
	manifest, err := mcpcatalog.DecodeVMCPManifest([]byte(result.Items[0].YAML))
	require.NoError(t, err)
	// A missing list would be defaulted to an admin-only profile, granting access nobody had.
	require.NotNil(t, manifest.Profiles)
	require.Empty(t, manifest.Profiles)
}

func TestListOrphanedVMCPsReportsUnreferenceableComponents(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entryID string
		entry   *v1.MCPServerCatalogEntry
		want    string
	}{
		{
			name:    "multi-user server",
			entryID: system.MCPServerPrefix + "abc",
			want:    "multi-user MCP server",
		},
		{
			name:    "deleted entry",
			entryID: "default-gone",
			want:    "which no longer exists",
		},
		{
			name:    "local entry",
			entryID: "default-local",
			entry:   syncedCatalogEntryForTest("default-local", "", "local"),
			want:    "which is not synced from a catalog source",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := orphanedVMCPForTest(orphanedTestSourceURL, "email", "Email", new(false))
			vmcp.Spec.Manifest.Components = []types.VMCPComponent{{
				ID:                      tc.entryID,
				Name:                    "Component",
				MCPServerCatalogEntryID: tc.entryID,
			}}
			objects := []kclient.Object{
				orphanedTestCatalog(),
				vmcp,
			}
			if tc.entry != nil {
				objects = append(objects, tc.entry)
			}

			result := callListOrphanedVMCPs(t, newVMCPTestStorage(objects...), orphanedTestSourceURL)
			require.Len(t, result.Items, 1)
			require.Empty(t, result.Items[0].YAML)
			require.Contains(t, result.Items[0].Error, tc.want)
		})
	}
}

func TestListOrphanedVMCPsReportsEntriesFromReplacedSources(t *testing.T) {
	vmcp := orphanedVMCPForTest(orphanedTestSourceURL, "email", "Email", new(false))
	vmcp.Spec.Manifest.Components = []types.VMCPComponent{{
		ID:                      "default-gmail",
		Name:                    "Gmail",
		MCPServerCatalogEntryID: "default-gmail",
	}}
	// The shared source was replaced with a branch, but the entry has not been
	// synced from it yet.
	catalog := orphanedTestCatalog()
	catalog.Spec.SourceURLs = []string{orphanedTestSourceURL, "https://github.com/example/shared/v2"}
	storage := newVMCPTestStorage(
		catalog,
		syncedCatalogEntryForTest("default-gmail", sharedTestSourceURL, "obot-gmail"),
		vmcp,
	)

	result := callListOrphanedVMCPs(t, storage, orphanedTestSourceURL)
	require.Len(t, result.Items, 1)
	require.Empty(t, result.Items[0].YAML)
	require.Equal(t, `component "Gmail" uses catalog entry "default-gmail" from https://github.com/example/shared/, which is no longer a source of catalog "default"; sync the catalog and generate the YAML again`, result.Items[0].Error)
}

func TestListOrphanedVMCPsRequiresSourceURL(t *testing.T) {
	storage := newVMCPTestStorage(orphanedTestCatalog())
	err := (&MCPCatalogHandler{}).ListOrphanedVMCPs(orphanedVMCPsRequest(httptest.NewRecorder(), storage, ""))
	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
}

func callListOrphanedVMCPs(t *testing.T, storage *vmcpTestStorage, sourceURL string) types.OrphanedVMCPCatalogItemList {
	t.Helper()
	recorder := httptest.NewRecorder()
	require.NoError(t, (&MCPCatalogHandler{}).ListOrphanedVMCPs(orphanedVMCPsRequest(recorder, storage, sourceURL)))

	var result types.OrphanedVMCPCatalogItemList
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&result))
	return result
}

func orphanedVMCPsRequest(recorder *httptest.ResponseRecorder, storage *vmcpTestStorage, sourceURL string) api.Context {
	request := httptest.NewRequest(http.MethodGet, "/api/mcp-catalogs/default/orphaned-vmcps?"+url.Values{"sourceURL": {sourceURL}}.Encode(), nil)
	request.SetPathValue("catalog_id", system.DefaultCatalog)
	return api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
		User: &user.DefaultInfo{
			Name:   "admin",
			UID:    "admin",
			Groups: []string{types.GroupAPI, types.GroupAdmin},
		},
	}
}

func orphanedTestCatalog() *v1.MCPCatalog {
	return &v1.MCPCatalog{
		Name:      system.DefaultCatalog,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPCatalogSpec{
			SourceURLs: []string{orphanedTestSourceURL, sharedTestSourceURL},
		},
	}
}

func orphanedVMCPForTest(sourceURL, entryKey, displayName string, adopted *bool) *v1.VMCP {
	vmcp := &v1.VMCP{
		Name:      mcpcatalog.VMCPName(system.DefaultCatalog, sourceURL, entryKey, displayName),
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Adopted:  adopted,
			Manifest: types.VMCPManifest{DisplayName: displayName},
		},
	}
	if adopted != nil {
		vmcp.Spec.AdoptionSourceURL = sourceURL
		vmcp.Spec.AdoptionEntryKey = entryKey
	}
	return vmcp
}

func syncedCatalogEntryForTest(name, sourceURL, entryKey string) *v1.MCPServerCatalogEntry {
	entry := vmcpCatalogEntryForTest(name)
	entry.Spec.MCPCatalogName = system.DefaultCatalog
	entry.Spec.SourceURL = sourceURL
	entry.Spec.Manifest.Name = name
	entry.Spec.Manifest.EntryKey = entryKey
	return entry
}
