package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/tools/cache"
)

func TestWebhooksIncludeGuardrailFiltersInNameOrder(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		"server-names":        func(any) ([]string, error) { return nil, nil },
		"catalog-entry-names": func(any) ([]string, error) { return nil, nil },
		"selectors":           func(any) ([]string, error) { return nil, nil },
		"catalog-names": func(obj any) ([]string, error) {
			var ids []string
			for _, resource := range obj.(*v1.MCPWebhookValidation).Spec.Manifest.Resources {
				ids = append(ids, resource.ID)
			}
			return ids, nil
		},
	})
	add := func(name string, configured bool, manifest types.MCPWebhookValidationManifest) {
		t.Helper()
		filter := &v1.MCPWebhookValidation{}
		filter.Name = name
		filter.Namespace = system.DefaultNamespace
		manifest.Resources = []types.Resource{{Type: types.ResourceTypeMcpCatalog, ID: "default"}}
		filter.Spec.Manifest = manifest
		filter.Status.Configured = configured
		if err := indexer.Add(filter); err != nil {
			t.Fatal(err)
		}
	}
	guardrail := &types.MCPGuardrailFilterConfig{GuardrailIDs: []string{"secrets"}, Action: types.MCPGuardrailActionBlock}
	add("mwv1c-webhook", true, types.MCPWebhookValidationManifest{Name: "Webhook", URL: "https://policy.example"})
	// A guardrail filter whose guardrail was deleted still runs, and fails at runtime.
	add("mwv1a-guardrail", false, types.MCPWebhookValidationManifest{Name: "Guardrail", Guardrail: guardrail})
	add("mwv1b-disabled", true, types.MCPWebhookValidationManifest{Name: "Disabled", Guardrail: guardrail, Disabled: true})
	add("mwv1d-unconfigured", false, types.MCPWebhookValidationManifest{Name: "Unconfigured", URL: "https://policy.example"})

	manager := &SessionManager{webhookHelper: NewWebhookHelper(indexer, "https://obot.example.com"), backend: &dockerBackend{hostBaseURLWithPort: "http://172.17.0.1:8080"}}
	server := ServerConfig{MCPServerName: "ms1one", MCPServerNamespace: system.DefaultNamespace, MCPCatalogName: "default"}
	webhooks, err := manager.webhooksForServerConfig(server)
	if err != nil {
		t.Fatal(err)
	}
	if len(webhooks) != 2 || webhooks[0].Name != "mwv1a-guardrail" || webhooks[1].Name != "mwv1c-webhook" {
		t.Fatalf("unexpected webhooks: %#v", webhooks)
	}
	if g := webhooks[0]; g.Guardrail == nil || g.URL != "" || g.ToolName != "" || g.DisplayName != "Guardrail" {
		t.Fatalf("unexpected guardrail filter: %#v", g)
	}
	if webhooks[1].Guardrail != nil || webhooks[1].ToolName != defaultWebhookToolName {
		t.Fatalf("unexpected webhook filter: %#v", webhooks[1])
	}

	// Guardrail filters have the same scope as other filters: none for system servers.
	server.SystemMCPServer = true
	if webhooks, err := manager.webhooksForServerConfig(server); err != nil || len(webhooks) != 0 {
		t.Fatalf("system servers must not be filtered: %#v %v", webhooks, err)
	}
}
