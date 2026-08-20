package mcpcatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResolveCompositeSourceRefs(t *testing.T) {
	target := testCatalogEntry("target", "source", "tool", types.MCPServerCatalogEntryManifest{
		Name:             "Tool",
		ShortDescription: "Tool",
		Description:      "Tool",
		Icon:             "icon",
		Runtime:          types.RuntimeNPX,
		NPXConfig:        &types.NPXRuntimeConfig{Package: "tool"},
		ServerUserType:   types.ServerUserTypeSingleUser,
	})
	composite := testCatalogEntry("composite", "source", "composite", types.MCPServerCatalogEntryManifest{
		Name:             "Composite",
		ShortDescription: "Composite",
		Description:      "Composite",
		Icon:             "icon",
		Runtime:          types.RuntimeComposite,
		ServerUserType:   types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: sourceRef("source", "tool")},
		}},
	})

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), nil, "", "", []kclient.Object{target, composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 2)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "target", component.CatalogEntryID)
}

func TestReadMCPCatalogResolvesCompositeSourceRefs(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "target.yaml"), []byte(`entryKey: tool
name: Tool
shortDescription: Tool
description: Tool
icon: icon
runtime: npx
npxConfig:
  package: tool
`), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "composite.yaml"), fmt.Appendf(nil, `entryKey: composite
name: Composite
shortDescription: Composite
description: Composite
icon: icon
runtime: composite
compositeConfig:
  componentServers:
    - catalogEntryID: %s
`, sourceRef(dir, "tool")), 0o600))

	h := &Handler{}
	objs, err := h.readMCPCatalog(t.Context(), "default", dir, "")
	assert.NoError(t, err)

	objs, errsBySourceURL := h.resolveCompositeSourceRefs(t.Context(), nil, "", "", objs)
	assert.Empty(t, errsBySourceURL)
	assert.Len(t, objs, 2)

	var composite, target *v1.MCPServerCatalogEntry
	for _, obj := range objs {
		entry := obj.(*v1.MCPServerCatalogEntry)
		if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
			composite = entry
		} else {
			target = entry
		}
	}
	if assert.NotNil(t, composite) && assert.NotNil(t, target) {
		component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
		assert.Equal(t, target.Name, component.CatalogEntryID)
	}
}

func TestReadMCPCatalogResolvesSameSourceEntryKeyShorthand(t *testing.T) {
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "target.yaml"), []byte(`entryKey: tool
name: Tool
shortDescription: Tool
description: Tool
icon: icon
runtime: npx
npxConfig:
  package: tool
`), 0o600))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "composite.yaml"), []byte(`entryKey: composite
name: Composite
shortDescription: Composite
description: Composite
icon: icon
runtime: composite
compositeConfig:
  componentServers:
    - catalogEntryID: tool
`), 0o600))

	h := &Handler{}
	objs, err := h.readMCPCatalog(t.Context(), "default", dir, "")
	assert.NoError(t, err)

	objs, errsBySourceURL := h.resolveCompositeSourceRefs(t.Context(), nil, "", "", objs)
	assert.Empty(t, errsBySourceURL)
	assert.Len(t, objs, 2)

	var composite, target *v1.MCPServerCatalogEntry
	for _, obj := range objs {
		entry := obj.(*v1.MCPServerCatalogEntry)
		if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
			composite = entry
		} else {
			target = entry
		}
	}
	if assert.NotNil(t, composite) && assert.NotNil(t, target) {
		component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
		assert.Equal(t, target.Name, component.CatalogEntryID)
	}
}

func TestResolveCompositeSourceRefsLeavesUnknownShorthandAsInternalID(t *testing.T) {
	composite := testCatalogEntry("composite", "source", "composite", types.MCPServerCatalogEntryManifest{
		Name:             "Composite",
		ShortDescription: "Composite",
		Description:      "Composite",
		Icon:             "icon",
		Runtime:          types.RuntimeComposite,
		ServerUserType:   types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "internal-id"},
		}},
	})

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), nil, "", "", []kclient.Object{composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 1)
	assert.Equal(t, "internal-id", composite.Spec.Manifest.CompositeConfig.ComponentServers[0].CatalogEntryID)
}

func TestResolveCompositeSourceRefsKeepsInternalIDComponents(t *testing.T) {
	target := testCatalogEntry("default-gmail-8a99d8be", "source", "gmail.yaml", types.MCPServerCatalogEntryManifest{
		Name:             "Gmail",
		ShortDescription: "Gmail",
		Description:      "Gmail",
		Icon:             "icon",
		Runtime:          types.RuntimeNPX,
		NPXConfig:        &types.NPXRuntimeConfig{Package: "gmail"},
		ServerUserType:   types.ServerUserTypeSingleUser,
	})
	composite := testCatalogEntry("composite", "source", "composite.yaml", types.MCPServerCatalogEntryManifest{
		Name:             "Composite",
		ShortDescription: "Composite",
		Description:      "Composite",
		Icon:             "icon",
		Runtime:          types.RuntimeComposite,
		ServerUserType:   types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "default-gmail-8a99d8be"},
		}},
	})

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), nil, "", "", []kclient.Object{target, composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 2)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "default-gmail-8a99d8be", component.CatalogEntryID)
}

func TestResolveCompositeSourceRefsResolvesUICreatedSameCatalogEntry(t *testing.T) {
	target := testCatalogEntry("ui-created-component", "", "", types.MCPServerCatalogEntryManifest{
		Name:             "UI Created Component",
		ShortDescription: "UI Created Component",
		Description:      "UI Created Component",
		Icon:             "icon",
		Runtime:          types.RuntimeNPX,
		NPXConfig:        &types.NPXRuntimeConfig{Package: "ui-created-component"},
		ServerUserType:   types.ServerUserTypeSingleUser,
	})
	target.Namespace = "default"
	target.Spec.MCPCatalogName = "default"
	target.Spec.Editable = true
	composite := testCatalogEntry("composite", "source", "composite.yaml", types.MCPServerCatalogEntryManifest{
		Name:           "Composite",
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "ui-created-component"},
		}},
	})
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(target).Build()

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), c, "default", "default", []kclient.Object{composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 1)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "ui-created-component", component.CatalogEntryID)
}

func TestResolveCompositeSourceRefsKeepsMultiUserServerIDComponents(t *testing.T) {
	server := testMCPServer("shared-server", "default", types.MCPServerManifest{
		Name:            "Shared Server",
		Runtime:         types.RuntimeContainerized,
		MultiUserConfig: &types.MultiUserConfig{UserDefinedHeaders: []types.MCPHeader{{Key: "API_KEY", Name: "API Key"}}},
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/shared:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})
	server.Spec.MCPCatalogID = "default"
	composite := testCatalogEntry("composite", "source", "composite.yaml", types.MCPServerCatalogEntryManifest{
		Name:           "Composite",
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{MCPServerID: "shared-server"},
		}},
	})
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(server).Build()

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), c, "default", "default", []kclient.Object{composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 1)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "shared-server", component.MCPServerID)
	assert.Empty(t, component.CatalogEntryID)
}

func TestResolveCompositeSourceRefsRejectsMultiUserServerOutsideCatalog(t *testing.T) {
	server := testMCPServer("shared-server", "default", types.MCPServerManifest{
		Name:            "Shared Server",
		Runtime:         types.RuntimeContainerized,
		MultiUserConfig: &types.MultiUserConfig{},
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/shared:1.0.0",
			Port:  8080,
			Path:  "/mcp",
		},
	})
	server.Spec.PowerUserWorkspaceID = "workspace-1"
	composite := testCatalogEntry("composite", "source", "composite.yaml", types.MCPServerCatalogEntryManifest{
		Name:           "Composite",
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{MCPServerID: "shared-server"},
		}},
	})
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(server).Build()

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), c, "default", "default", []kclient.Object{composite})

	assert.NotEmpty(t, errsBySourceURL)
	assert.Empty(t, result)
	assert.Equal(t, "shared-server", composite.Spec.Manifest.CompositeConfig.ComponentServers[0].MCPServerID)
}

func TestReadMCPCatalogResolvesCompositeSourceRefsAcrossSources(t *testing.T) {
	first := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(first, "target.yaml"), []byte(`entryKey: tool
name: Tool
shortDescription: Tool
description: Tool
icon: icon
runtime: npx
npxConfig:
  package: tool
`), 0o600))

	second := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(second, "composite.yaml"), fmt.Appendf(nil, `entryKey: composite
name: Composite
shortDescription: Composite
description: Composite
icon: icon
runtime: composite
compositeConfig:
  componentServers:
    - catalogEntryID: %s
`, sourceRef(first, "tool")), 0o600))

	h := &Handler{}
	firstObjs, err := h.readMCPCatalog(t.Context(), "default", first, "")
	assert.NoError(t, err)
	secondObjs, err := h.readMCPCatalog(t.Context(), "default", second, "")
	assert.NoError(t, err)

	objs, errsBySourceURL := h.resolveCompositeSourceRefs(t.Context(), nil, "", "", append(firstObjs, secondObjs...))
	assert.Empty(t, errsBySourceURL)
	assert.Len(t, objs, 2)

	var composite, target *v1.MCPServerCatalogEntry
	for _, obj := range objs {
		entry := obj.(*v1.MCPServerCatalogEntry)
		if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
			composite = entry
		} else {
			target = entry
		}
	}
	if assert.NotNil(t, composite) && assert.NotNil(t, target) {
		component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
		assert.Equal(t, target.Name, component.CatalogEntryID)
	}
}

func TestResolveCompositeSourceRefsResolvesExplicitSourceRefWithoutCurrentSource(t *testing.T) {
	target := testCatalogEntry("target", "external-source", "tool", types.MCPServerCatalogEntryManifest{
		Name:             "Tool",
		ShortDescription: "Tool",
		Description:      "Tool",
		Icon:             "icon",
		Runtime:          types.RuntimeNPX,
		NPXConfig:        &types.NPXRuntimeConfig{Package: "tool"},
		ServerUserType:   types.ServerUserTypeSingleUser,
	})
	composite := testCatalogEntry("composite", "", "", types.MCPServerCatalogEntryManifest{
		Name:             "Composite",
		ShortDescription: "Composite",
		Description:      "Composite",
		Icon:             "icon",
		Runtime:          types.RuntimeComposite,
		ServerUserType:   types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: sourceRef("external-source", "tool")},
		}},
	})

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), nil, "", "", []kclient.Object{target, composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 2)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "target", component.CatalogEntryID)
}

// A well-formed portable ref whose target is gone must not remove the composite from the catalog.
// A malformed ref is still a sync error - see TestResolveCompositeSourceRefsSkipsMalformedRef.
func TestResolveCompositeSourceRefsAppliesUnresolvedPortableRef(t *testing.T) {
	target := testCatalogEntry("target", "source", "tool", types.MCPServerCatalogEntryManifest{
		Name:             "Tool",
		ShortDescription: "Tool",
		Description:      "Tool",
		Icon:             "icon",
		Runtime:          types.RuntimeNPX,
		NPXConfig:        &types.NPXRuntimeConfig{Package: "tool"},
		ServerUserType:   types.ServerUserTypeSingleUser,
	})
	composite := testCatalogEntry("composite", "source", "composite", types.MCPServerCatalogEntryManifest{
		Name:             "Composite",
		ShortDescription: "Composite",
		Description:      "Composite",
		Icon:             "icon",
		Runtime:          types.RuntimeComposite,
		ServerUserType:   types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: sourceRef("source", "missing")},
		}},
	})

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), nil, "", "", []kclient.Object{target, composite})

	assert.Len(t, result, 2)
	assert.Equal(t, "target", result[0].GetName())
	assert.Equal(t, "composite", result[1].GetName())
	assert.Empty(t, errsBySourceURL)

	// The reference is left as authored so the entry controller can report it missing.
	applied := result[1].(*v1.MCPServerCatalogEntry)
	assert.Equal(t, sourceRef("source", "missing"), applied.Spec.Manifest.CompositeConfig.ComponentServers[0].CatalogEntryID)
}

func TestResolveCompositeSourceRefsSkipsMalformedRef(t *testing.T) {
	composite := testCatalogEntry("composite", "source", "composite", types.MCPServerCatalogEntryManifest{
		Name:             "Composite",
		ShortDescription: "Composite",
		Description:      "Composite",
		Icon:             "icon",
		Runtime:          types.RuntimeComposite,
		ServerUserType:   types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "source::"},
		}},
	})

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), nil, "", "", []kclient.Object{composite})

	assert.Empty(t, result)
	assert.Contains(t, errsBySourceURL["source"], `invalid catalogEntryID source ref "source::"`)
}

func TestResolveCompositeSourceRefsAppliesDanglingComponentReferences(t *testing.T) {
	composite := testCatalogEntry("composite", "source", "composite.yaml", types.MCPServerCatalogEntryManifest{
		Name:           "Composite",
		Runtime:        types.RuntimeComposite,
		ServerUserType: types.ServerUserTypeSingleUser,
		CompositeConfig: &types.CompositeCatalogConfig{ComponentServers: []types.CatalogComponentServer{
			{CatalogEntryID: "deleted-entry", ToolPrefix: "gh"},
			{MCPServerID: "deleted-server", ToolPrefix: "slack"},
		}},
	})
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(t.Context(), c, "default", "default", []kclient.Object{composite})

	// Failing the entry instead would let one hard-deleted upstream remove the whole composite.
	assert.Empty(t, errsBySourceURL)
	require.Len(t, result, 1)
	components := result[0].(*v1.MCPServerCatalogEntry).Spec.Manifest.CompositeConfig.ComponentServers
	require.Len(t, components, 2)
	assert.Equal(t, "deleted-entry", components[0].CatalogEntryID)
	assert.Equal(t, "gh", components[0].ToolPrefix)
	assert.Equal(t, "deleted-server", components[1].MCPServerID)
	assert.Equal(t, "slack", components[1].ToolPrefix)
}

func testCatalogEntry(name, sourceID, entryKey string, manifest types.MCPServerCatalogEntryManifest) *v1.MCPServerCatalogEntry {
	manifest.EntryKey = entryKey
	return &v1.MCPServerCatalogEntry{
		Name: name,
		Spec: v1.MCPServerCatalogEntrySpec{
			SourceURL: sourceID,
			Manifest:  manifest,
			Editable:  false,
		},
	}
}

func testMCPServer(name, namespace string, manifest types.MCPServerManifest) *v1.MCPServer {
	return &v1.MCPServer{
		Name: name, Namespace: namespace,
		Spec: v1.MCPServerSpec{
			Manifest: manifest,
		},
	}
}
