package mcpwebhookvalidation

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestDesiredSystemServer_CopiesProvidedManifest(t *testing.T) {
	validation := &v1.MCPWebhookValidation{}
	validation.Name = "validation-1"
	validation.Namespace = "default"
	validation.Spec.Manifest.URL = "https://ignored.example.com/webhook"
	validation.Spec.Manifest.SystemMCPServerManifest = &types.SystemMCPServerManifest{
		Name:             "custom-validator",
		ShortDescription: "Custom validation server",
		Enabled:          new(true),
		Runtime:          types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/image:latest",
			Port:  9999,
			Path:  "/custom",
		},
		Env: []types.MCPEnv{{
			MCPHeader: types.MCPHeader{Key: "CUSTOM", Value: "1"},
		}},
	}

	server := desiredSystemServer(validation, "ignored-image")

	if server.Spec.Manifest.Name != "custom-validator" {
		t.Fatalf("expected manifest name to be copied, got %q", server.Spec.Manifest.Name)
	}
	if server.Spec.Manifest.ContainerizedConfig == nil || server.Spec.Manifest.ContainerizedConfig.Image != "example/image:latest" {
		t.Fatalf("expected containerized config image to be copied, got %#v", server.Spec.Manifest.ContainerizedConfig)
	}
	if len(server.Spec.Manifest.Env) != 1 || server.Spec.Manifest.Env[0].Key != "CUSTOM" {
		t.Fatalf("expected env to be copied, got %#v", server.Spec.Manifest.Env)
	}
	if server.Spec.WebhookValidationName != validation.Name {
		t.Fatalf("expected webhook validation name %q, got %q", validation.Name, server.Spec.WebhookValidationName)
	}

	validation.Spec.Manifest.SystemMCPServerManifest.Name = "mutated"
	if server.Spec.Manifest.Name != "custom-validator" {
		t.Fatalf("expected copied manifest to be independent after mutation, got %q", server.Spec.Manifest.Name)
	}
	if server.Spec.Manifest.Env[0].Key == "WEBHOOK_URL" {
		t.Fatalf("expected provided manifest to be used instead of derived webhook env, got %#v", server.Spec.Manifest.Env)
	}
}
