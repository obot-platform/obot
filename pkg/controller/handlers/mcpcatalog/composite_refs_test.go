package mcpcatalog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(context.Background(), []client.Object{target, composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 2)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "target", component.CatalogEntryID)
	assert.Equal(t, target.Spec.Manifest.Name, component.Manifest.Name)
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
	objs, err := h.readMCPCatalog(context.Background(), "default", dir, "")
	assert.NoError(t, err)

	objs, errsBySourceURL := h.resolveCompositeSourceRefs(context.Background(), objs)
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
		assert.Equal(t, "Tool", component.Manifest.Name)
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
	objs, err := h.readMCPCatalog(context.Background(), "default", dir, "")
	assert.NoError(t, err)

	objs, errsBySourceURL := h.resolveCompositeSourceRefs(context.Background(), objs)
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
		assert.Equal(t, "Tool", component.Manifest.Name)
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

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(context.Background(), []client.Object{composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 1)
	assert.Equal(t, "internal-id", composite.Spec.Manifest.CompositeConfig.ComponentServers[0].CatalogEntryID)
}

func TestResolveCompositeSourceRefsHydratesInternalIDComponents(t *testing.T) {
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

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(context.Background(), []client.Object{target, composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 2)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "default-gmail-8a99d8be", component.CatalogEntryID)
	assert.Equal(t, "Gmail", component.Manifest.Name)
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
	firstObjs, err := h.readMCPCatalog(context.Background(), "default", first, "")
	assert.NoError(t, err)
	secondObjs, err := h.readMCPCatalog(context.Background(), "default", second, "")
	assert.NoError(t, err)

	objs, errsBySourceURL := h.resolveCompositeSourceRefs(context.Background(), append(firstObjs, secondObjs...))
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
		assert.Equal(t, "Tool", component.Manifest.Name)
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

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(context.Background(), []client.Object{target, composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 2)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "target", component.CatalogEntryID)
	assert.Equal(t, "Tool", component.Manifest.Name)
}

func TestResolveCompositeSourceRefsSkipsUnresolvedComposite(t *testing.T) {
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

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(context.Background(), []client.Object{target, composite})

	assert.Len(t, result, 1)
	assert.Equal(t, "target", result[0].GetName())
	assert.Contains(t, errsBySourceURL["source"], `unresolved catalogEntryID source ref "source::missing"`)
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

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(context.Background(), []client.Object{composite})

	assert.Empty(t, result)
	assert.Contains(t, errsBySourceURL["source"], `invalid catalogEntryID source ref "source::"`)
}

func testCatalogEntry(name, sourceID, entryKey string, manifest types.MCPServerCatalogEntryManifest) *v1.MCPServerCatalogEntry {
	manifest.EntryKey = entryKey
	return &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.MCPServerCatalogEntrySpec{
			SourceURL: sourceID,
			Manifest:  manifest,
			Editable:  false,
		},
	}
}
