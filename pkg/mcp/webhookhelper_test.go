package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/client-go/tools/cache"
)

func TestWebhookHelperDeduplicatesAndExcludesUnavailableFilters(t *testing.T) {
	indexForResourceType := func(resourceType types.MCPWebhookValidationResourceType) cache.IndexFunc {
		return func(obj any) ([]string, error) {
			validation := obj.(*v1.MCPWebhookValidation)
			var result []string
			for _, resource := range validation.Spec.Manifest.Resources {
				if resource.Type == resourceType {
					result = append(result, resource.ID)
				}
			}
			return result, nil
		}
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		"server-names":        indexForResourceType(types.MCPWebhookValidationResourceTypeMCPServer),
		"catalog-entry-names": indexForResourceType(types.MCPWebhookValidationResourceTypeMCPServerCatalogEntry),
		"selectors":           indexForResourceType(types.MCPWebhookValidationResourceTypeSelector),
		"catalog-names":       indexForResourceType(types.MCPWebhookValidationResourceTypeMCPCatalog),
	})
	active := &v1.MCPWebhookValidation{
		Name:      "filter-active",
		Namespace: "default",
		Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
			Name: "Active Filter",
			Resources: []types.MCPWebhookValidationResource{
				{
					Type: types.MCPWebhookValidationResourceTypeMCPServer,
					ID:   "server-1",
				},
				{
					Type: types.MCPWebhookValidationResourceTypeSelector,
					ID:   "*",
				},
			},
		}},
		Status: v1.MCPWebhookValidationStatus{Configured: true},
	}
	disabled := active.DeepCopy()
	disabled.Name = "filter-disabled"
	disabled.Spec.Manifest.Disabled = true
	unconfigured := active.DeepCopy()
	unconfigured.Name = "filter-unconfigured"
	unconfigured.Status.Configured = false
	for _, validation := range []*v1.MCPWebhookValidation{active, disabled, unconfigured} {
		if err := indexer.Add(validation); err != nil {
			t.Fatal(err)
		}
	}

	webhooks, err := NewWebhookHelper(indexer, "https://obot.example").GetWebhooksForMCPServer(ServerConfig{
		MCPServerName:      "server-1",
		MCPServerNamespace: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(webhooks) != 1 || webhooks[0].Name != "filter-active" || webhooks[0].DisplayName != "Active Filter" {
		t.Fatalf("unexpected resolved Filters: %#v", webhooks)
	}
}
