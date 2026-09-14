package mcp

import (
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/tools/cache"
)

func TestVMCPInstancesInheritWebhooks(t *testing.T) {
	indexers := cache.Indexers{}
	for index, resourceType := range map[string]types.ResourceType{
		"server-names":        types.ResourceTypeMCPServer,
		"catalog-entry-names": types.ResourceTypeMCPServerCatalogEntry,
		"catalog-names":       types.ResourceTypeMcpCatalog,
		"selectors":           types.ResourceTypeSelector,
	} {
		indexers[index] = func(obj any) ([]string, error) {
			var ids []string
			for _, resource := range obj.(*v1.MCPWebhookValidation).Spec.Manifest.Resources {
				if resource.Type == resourceType {
					ids = append(ids, resource.ID)
				}
			}
			return ids, nil
		}
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, indexers)
	selectors := types.MCPSelectors{{Method: "tools/call", Identifiers: []string{"search"}}}
	for _, id := range []string{"vmcp1selected", "vmcp1other"} {
		webhook := &v1.MCPWebhookValidation{}
		webhook.Name = id
		webhook.Namespace = system.DefaultNamespace
		webhook.Spec.Manifest = types.MCPWebhookValidationManifest{
			Resources: []types.Resource{{
				Type: types.ResourceTypeMCPServer,
				ID:   id,
			}},
			Selectors:       selectors,
			AllowedToMutate: true,
		}
		webhook.Status.Configured = true
		if err := indexer.Add(webhook); err != nil {
			t.Fatal(err)
		}
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1selected",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{{
				ID:              "one",
				Name:            "one",
				ForceSingleUser: true,
			}},
		}},
	}
	storage := newVMCPTestStorage(vmcp)
	manager := &SessionManager{
		storageClient: storage,
		webhookHelper: NewWebhookHelper(indexer, "https://obot.example.com"),
		backend:       &dockerBackend{hostBaseURLWithPort: "http://172.17.0.1:8080"},
	}
	for _, userID := range []string{"1", "2"} {
		instance := &v1.VMCPInstance{
			Name:      "vmcpi1user" + userID,
			Namespace: vmcp.Namespace,
			Spec: v1.VMCPInstanceSpec{
				UserID:   userID,
				Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
			},
		}
		if err := storage.Create(t.Context(), instance); err != nil {
			t.Fatal(err)
		}
		server := vmcpComponentServer("ms1user"+userID, instance.Name, userID, "one", "one", "https://example.com/mcp")
		if err := storage.Create(t.Context(), server); err != nil {
			t.Fatal(err)
		}
		cfg, err := manager.ServerConfigForVMCP(t.Context(), instance.Name, userID)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.MCPServerName != instance.Name || len(cfg.Webhooks) != 1 || cfg.Webhooks[0].Name != vmcp.Name {
			t.Fatalf("instance %s did not inherit its parent's webhook: %#v", instance.Name, cfg)
		}
		if !reflect.DeepEqual(cfg.Webhooks[0].Definitions, selectors) || !cfg.Webhooks[0].MutateAllowed {
			t.Fatalf("webhook configuration not preserved: %#v", cfg.Webhooks[0])
		}
		webhook := cfg.Webhooks[0]
		wantAudience := system.MCPConnectURL("https://obot.example.com", system.SystemMCPServerPrefix+vmcp.Name)
		wantURL := system.MCPConnectURL("http://172.17.0.1:8080", system.SystemMCPServerPrefix+vmcp.Name)
		if webhook.URL != wantURL || webhook.Audience != wantAudience {
			t.Fatalf("webhook URL = %q, audience = %q; want %q, %q", webhook.URL, webhook.Audience, wantURL, wantAudience)
		}
	}
}
