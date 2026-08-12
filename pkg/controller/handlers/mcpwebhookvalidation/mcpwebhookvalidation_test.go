package mcpwebhookvalidation

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestCleanupResourcesPreservesDeviceResource(t *testing.T) {
	filter := &v1.MCPWebhookValidation{
		ObjectMeta: metav1.ObjectMeta{Name: "filter", Namespace: system.DefaultNamespace},
		Spec: v1.MCPWebhookValidationSpec{Manifest: types.MCPWebhookValidationManifest{
			Resources: []types.Resource{
				{
					Type: types.ResourceTypeDevice,
					ID:   "*",
				},
				{
					Type: types.ResourceTypeMCPServer,
					ID:   "missing",
				},
			},
		}},
	}
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(filter).Build()

	if err := new(Handler).CleanupResources(router.Request{
		Client:    client,
		Object:    filter,
		Ctx:       t.Context(),
		Namespace: system.DefaultNamespace,
	}, nil); err != nil {
		t.Fatal(err)
	}

	if len(filter.Spec.Manifest.Resources) != 1 || filter.Spec.Manifest.Resources[0].Type != types.ResourceTypeDevice {
		t.Fatalf("cleanup removed device resource: %v", filter.Spec.Manifest.Resources)
	}
}
