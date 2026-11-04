package mcp

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestAuthorizationSurvivesContentReadiness(t *testing.T) {
	resource := &v1.MCPWebhookValidation{
		Name:      "filter1",
		Namespace: "default",
		Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
			URL:           "https://content.example",
			Authorization: &types.MCPFilterAuthorization{Enabled: true, SigningEnabled: true, URL: "https://policy.example"},
			Selectors:     types.MCPSelectors{{Method: "resources/read", Identifiers: []string{"crm://customer"}}},
		}},
	}
	helper := &WebhookHelper{baseURL: "https://obot.example"}
	webhooks := helper.appendWebhooks("default", []any{resource}, map[string]struct{}{}, nil, func(s string) string { return s })
	if len(webhooks) != 1 {
		t.Fatal("required authorization skipped because content is not ready")
	}
	mappings := hookDefinitions(webhooks, strings.NewReplacer())
	if len(mappings) != 1 || mappings[0].Authorization == nil || !mappings[0].Authorization.SigningEnabled || len(mappings[0].Targets) != 0 {
		t.Fatalf("wrong mappings: %+v", mappings)
	}
	if !mappings[0].Matches("resources/read", map[string]string{"name": "crm://customer"}) || mappings[0].Matches("tools/call", nil) {
		t.Fatal("selector compilation changed")
	}
	resource.Spec.Manifest.Authorization = nil
	if got := helper.appendWebhooks("default", []any{resource}, map[string]struct{}{}, nil, func(s string) string { return s }); len(got) != 0 {
		t.Fatal("legacy readiness changed")
	}
}
