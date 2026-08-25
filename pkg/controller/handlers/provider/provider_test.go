package provider

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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
groupIDPrefix: github/
`), 0o644); err != nil {
		t.Fatal(err)
	}

	objs, err := readRegistry(dir)
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
			if provider.Spec.Command != filepath.Join(dir, "bin/openai-model-provider") {
				t.Fatalf("expected model provider command %q, got %q", filepath.Join(dir, "bin/openai-model-provider"), provider.Spec.Command)
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
			if provider.Spec.Command != filepath.Join(dir, "bin/github-auth-provider") {
				t.Fatalf("expected auth provider command %q, got %q", filepath.Join(dir, "bin/github-auth-provider"), provider.Spec.Command)
			}
			if provider.Spec.GroupIDPrefix != "github/" {
				t.Fatalf("expected auth provider group ID prefix github/, got %q", provider.Spec.GroupIDPrefix)
			}
		default:
			t.Fatalf("unexpected object type %T", obj)
		}
	}
	if !foundModel || !foundAuth {
		t.Fatalf("expected both model and auth providers, foundModel=%v foundAuth=%v", foundModel, foundAuth)
	}
}

func TestAppendProvidersSkipsInvalidGroupIDPrefix(t *testing.T) {
	providers := []providerFromFile[types.AuthProviderManifest]{
		{
			Name: "valid",
			Manifest: types.AuthProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					Command: "bin/valid",
				},
				GroupIDPrefix: "valid/",
			},
		},
		{
			Name: "invalid",
			Manifest: types.AuthProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					Command: "bin/invalid",
				},
				GroupIDPrefix: "invalid%/",
			},
		},
	}

	objects := appendProviders("/registry", providers, nil)
	if len(objects) != 1 {
		t.Fatalf("providers = %d, want 1", len(objects))
	}
	provider, ok := objects[0].(*v1.AuthProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *v1.AuthProvider", objects[0])
	}
	if provider.Name != "valid" || provider.Spec.GroupIDPrefix != "valid/" {
		t.Fatalf("provider = %#v, want valid provider", provider)
	}
}

func TestValidateUniqueAuthProviderGroupIDPrefixes(t *testing.T) {
	tests := []struct {
		name      string
		objects   []kclient.Object
		wantError bool
	}{
		{
			name: "unique prefixes",
			objects: []kclient.Object{
				&v1.AuthProvider{
					Name: "entra",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "entra/",
						},
					},
				},
				&v1.AuthProvider{
					Name: "okta",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "okta/",
						},
					},
				},
				&v1.AuthProvider{
					Name: "local",
				},
				&v1.ModelProvider{
					Name: "model",
				},
			},
		},
		{
			name: "duplicate prefixes",
			objects: []kclient.Object{
				&v1.AuthProvider{
					Name: "first",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "entra/",
						},
					},
				},
				&v1.AuthProvider{
					Name: "second",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "entra/",
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "case variants are duplicate prefixes",
			objects: []kclient.Object{
				&v1.AuthProvider{
					Name: "first",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "Entra/",
						},
					},
				},
				&v1.AuthProvider{
					Name: "second",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "entra/",
						},
					},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUniqueAuthProviderGroupIDPrefixes(tt.objects)
			if tt.wantError && err == nil {
				t.Fatal("expected an error")
			}
			if !tt.wantError && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModelDialectPrefersMetadataDialect(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata map[string]string
		fallback string
		want     string
	}{
		{
			name:     "metadata dialect wins",
			metadata: map[string]string{"dialect": "AnthropicMessages"},
			fallback: "OpenAIResponses",
			want:     "AnthropicMessages",
		},
		{
			name:     "empty metadata dialect falls back",
			metadata: map[string]string{"dialect": ""},
			fallback: "OpenAIResponses",
			want:     "OpenAIResponses",
		},
		{
			name:     "missing metadata dialect falls back",
			metadata: map[string]string{"usage": "llm"},
			fallback: "OpenAIResponses",
			want:     "OpenAIResponses",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := modelDialect(tc.metadata, tc.fallback); got != tc.want {
				t.Fatalf("modelDialect() = %q, want %q", got, tc.want)
			}
		})
	}
}
