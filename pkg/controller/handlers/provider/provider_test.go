package provider

import (
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestReadLocalProviderRegistryFromSubdirectories(t *testing.T) {
	dir := t.TempDir()
	modelProvidersDir := filepath.Join(dir, modelProvidersRegistryDir)
	authProvidersDir := filepath.Join(dir, authProvidersRegistryDir)
	if err := os.MkdirAll(modelProvidersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(authProvidersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(modelProvidersDir, "openai-model-provider.yaml"), []byte(`name: OpenAI
image: ghcr.io/example/openai-model-provider
port: 8080
dialect: OpenAIResponses
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelProvidersDir, "ignored.json"), []byte(`{"name":"ignored"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authProvidersDir, "github-auth-provider.yaml"), []byte(`name: GitHub
image: ghcr.io/example/github-auth-provider
port: 8080
`), 0o644); err != nil {
		t.Fatal(err)
	}

	objs, err := readLocalProviderRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 provider objects, got %d", len(objs))
	}

	var foundModel, foundAuth bool
	for _, obj := range objs {
		switch provider := obj.(type) {
		case *v1.ModelProvider:
			foundModel = true
			if provider.Name != "openai-model-provider" {
				t.Fatalf("expected model provider name openai-model-provider, got %q", provider.Name)
			}
			if provider.Spec.Name != "OpenAI" {
				t.Fatalf("expected model provider display name OpenAI, got %q", provider.Spec.Name)
			}
			if provider.Spec.Image != "ghcr.io/example/openai-model-provider" {
				t.Fatalf("expected model provider image ghcr.io/example/openai-model-provider, got %q", provider.Spec.Image)
			}
			if provider.Spec.Port != 8080 {
				t.Fatalf("expected model provider port 8080, got %d", provider.Spec.Port)
			}
			if provider.Spec.Dialect != "OpenAIResponses" {
				t.Fatalf("expected model provider dialect OpenAIResponses, got %q", provider.Spec.Dialect)
			}
		case *v1.AuthProvider:
			foundAuth = true
			if provider.Name != "github-auth-provider" {
				t.Fatalf("expected auth provider name github-auth-provider, got %q", provider.Name)
			}
			if provider.Spec.Name != "GitHub" {
				t.Fatalf("expected auth provider display name GitHub, got %q", provider.Spec.Name)
			}
			if provider.Spec.Image != "ghcr.io/example/github-auth-provider" {
				t.Fatalf("expected auth provider image ghcr.io/example/github-auth-provider, got %q", provider.Spec.Image)
			}
			if provider.Spec.Port != 8080 {
				t.Fatalf("expected auth provider port 8080, got %d", provider.Spec.Port)
			}
		default:
			t.Fatalf("unexpected object type %T", obj)
		}
	}
	if !foundModel || !foundAuth {
		t.Fatalf("expected both model and auth providers, foundModel=%v foundAuth=%v", foundModel, foundAuth)
	}
}
