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
command: bin/openai-model-provider
dialect: OpenAIResponses
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelProvidersDir, "ignored.json"), []byte(`{"name":"ignored"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authProvidersDir, "github-auth-provider.yaml"), []byte(`name: GitHub
command: bin/github-auth-provider
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authProvidersDir, "generic-oauth-auth-provider.yaml"), []byte(`name: Custom OAuth / OIDC
command: bin/generic-oauth-auth-provider
requiredConfigurationParameters:
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_NAME
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_SECRET
  - name: OBOT_AUTH_PROVIDER_EMAIL_DOMAINS
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING
optionalConfigurationParameters:
  - name: OBOT_GENERIC_OAUTH_AUTH_PROVIDER_SCOPE
`), 0o644); err != nil {
		t.Fatal(err)
	}

	objs, err := readRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 3 {
		t.Fatalf("expected 3 provider objects, got %d", len(objs))
	}

	var foundModel, foundAuth, foundGenericAuth bool
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
			if provider.Spec.Command != filepath.Join(dir, "bin/openai-model-provider") {
				t.Fatalf("expected model provider command %q, got %q", filepath.Join(dir, "bin/openai-model-provider"), provider.Spec.Command)
			}
			if provider.Spec.Dialect != "OpenAIResponses" {
				t.Fatalf("expected model provider dialect OpenAIResponses, got %q", provider.Spec.Dialect)
			}
		case *v1.AuthProvider:
			if provider.Name == "generic-oauth-auth-provider" {
				foundGenericAuth = true
				if provider.Spec.Name != "Custom OAuth / OIDC" {
					t.Fatalf("expected generic auth provider display name Custom OAuth / OIDC, got %q", provider.Spec.Name)
				}
				if provider.Spec.Command != filepath.Join(dir, "bin/generic-oauth-auth-provider") {
					t.Fatalf("expected generic auth provider command %q, got %q", filepath.Join(dir, "bin/generic-oauth-auth-provider"), provider.Spec.Command)
				}
				if len(provider.Spec.RequiredConfigurationParameters) != 6 {
					t.Fatalf("expected 6 required generic auth provider params, got %d", len(provider.Spec.RequiredConfigurationParameters))
				}
				continue
			}

			foundAuth = true
			if provider.Name != "github-auth-provider" {
				t.Fatalf("expected auth provider name github-auth-provider, got %q", provider.Name)
			}
			if provider.Spec.Name != "GitHub" {
				t.Fatalf("expected auth provider display name GitHub, got %q", provider.Spec.Name)
			}
			if provider.Spec.Command != filepath.Join(dir, "bin/github-auth-provider") {
				t.Fatalf("expected auth provider command %q, got %q", filepath.Join(dir, "bin/github-auth-provider"), provider.Spec.Command)
			}
		default:
			t.Fatalf("unexpected object type %T", obj)
		}
	}
	if !foundModel || !foundAuth || !foundGenericAuth {
		t.Fatalf("expected model, github auth, and generic auth providers, foundModel=%v foundAuth=%v foundGenericAuth=%v", foundModel, foundAuth, foundGenericAuth)
	}
}
