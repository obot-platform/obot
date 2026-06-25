package mcpcatalog

import (
	"context"
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
			{CatalogEntryID: "source" + catalogReferenceSeparator + "tool"},
		}},
	})

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(context.Background(), []client.Object{target, composite})

	assert.Empty(t, errsBySourceURL)
	assert.Len(t, result, 2)
	component := composite.Spec.Manifest.CompositeConfig.ComponentServers[0]
	assert.Equal(t, "target", component.CatalogEntryID)
	assert.Equal(t, target.Spec.Manifest.Name, component.Manifest.Name)
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
			{CatalogEntryID: "source" + catalogReferenceSeparator + "missing"},
		}},
	})

	result, errsBySourceURL := (&Handler{}).resolveCompositeSourceRefs(context.Background(), []client.Object{target, composite})

	assert.Len(t, result, 1)
	assert.Equal(t, "target", result[0].GetName())
	assert.Contains(t, errsBySourceURL["source-url"], `unresolved catalogEntryID source ref "source|missing"`)
}

func testCatalogEntry(name, sourceID, idRef string, manifest types.MCPServerCatalogEntryManifest) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.MCPServerCatalogEntrySpec{
			SourceURL:        "source-url",
			SourceID:         sourceID,
			SourceEntryIDRef: idRef,
			Manifest:         manifest,
			Editable:         false,
		},
	}
}
