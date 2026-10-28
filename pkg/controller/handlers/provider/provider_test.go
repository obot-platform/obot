package provider

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	openai "github.com/obot-platform/chat-completion-client"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeLister []string

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

	if err := os.WriteFile(filepath.Join(modelProvidersDir, "openai-model-provider.yaml"), []byte(`name: "${openai_model_provider_name}"
command: bin/openai-model-provider
dialect: OpenAIResponses
requiredConfigurationParameters:
  - name: API_KEY
    friendlyName: "${openai_model_provider_api_key_friendly_name}"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelProvidersDir, "ignored.json"), []byte(`{"name":"ignored"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authProvidersDir, "github-auth-provider.yaml"), []byte(`name: "${github_auth_provider_name}"
command: bin/github-auth-provider
groupIDPrefix: github/
`), 0o644); err != nil {
		t.Fatal(err)
	}

	objs, err := readRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Registries still ship model providers, which the LLM gateway replaced; they're ignored.
	if len(objs) != 1 {
		t.Fatalf("expected 1 provider object, got %d", len(objs))
	}

	var foundAuth bool
	for _, obj := range objs {
		switch provider := obj.(type) {
		case *v1.ModelProvider:
			t.Fatalf("model provider %q from the registry should be ignored", provider.Name)
		case *v1.AuthProvider:
			foundAuth = true
			if provider.Name != "github-auth-provider" {
				t.Fatalf("expected auth provider name github-auth-provider, got %q", provider.Name)
			}
			if provider.Spec.Name != "${github_auth_provider_name}" {
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
	if !foundAuth {
		t.Fatal("expected the auth provider")
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

	objects := appendProviders("/registry", providers)
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

func TestKeepModelEdits(t *testing.T) {
	override := &types.ModelCost{Input: 1}
	discovered := types.ModelManifest{Name: "gpt-5", DisplayName: "GPT-5", TargetModel: "openai/gpt-5", Active: true, Usage: types.ModelUsageLLM, Dialect: "openai"}
	keepModelEdits(&discovered, types.ModelManifest{DisplayName: "Our GPT", Active: false, Alias: "main", OverrideCost: override, Usage: types.ModelUsageEmbedding})
	want := types.ModelManifest{Name: "gpt-5", DisplayName: "Our GPT", TargetModel: "openai/gpt-5", Alias: "main", Active: false, Usage: types.ModelUsageEmbedding, Dialect: "openai", OverrideCost: override}
	if discovered != want {
		t.Fatalf("got %+v, want %+v", discovered, want)
	}
}

func (f fakeLister) ConfiguredProviders(context.Context) ([]string, error) { return f, nil }

func TestUnlistedProviderModels(t *testing.T) {
	model := func(name, target string) v1.Model {
		return v1.Model{Namespace: "default", Name: name, Spec: v1.ModelSpec{Manifest: types.ModelManifest{TargetModel: target, ModelProvider: system.LLMGatewayModelProvider, DisplayName: "edited " + name}}}
	}
	existing := []v1.Model{
		model("m-openai", "openai/gpt-5"),
		model("m-anthropic", "anthropic/claude-sonnet-5"),
		model("m-groq", "groq/llama"),
	}
	info := &v1.ModelInfo{Namespace: "default", Name: v1.ModelInfoName(system.LLMGatewayModelProvider, "anthropic/claude-sonnet-5")}
	client := kfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(info).Build()
	// OpenAI listed its models; Anthropic is configured but its list call failed; Groq was
	// removed from the gateway.
	listed := &openai.ModelsList{Models: []openai.Model{{ID: "openai/gpt-5"}}}

	kept, err := unlistedProviderModels(context.Background(), client, fakeLister{"openai", "anthropic"}, listed, existing)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, o := range kept {
		names = append(names, o.GetName())
	}
	want := []string{"m-anthropic", info.Name}
	if !slices.Equal(names, want) {
		t.Fatalf("kept %v, want %v", names, want)
	}
	if m := kept[0].(*v1.Model); m.Spec.Manifest.DisplayName != "edited m-anthropic" {
		t.Fatalf("the kept model lost its edits: %+v", m.Spec.Manifest)
	}
}
