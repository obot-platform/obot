package nanobotagent

import (
	"context"
	"testing"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	sigsyaml "sigs.k8s.io/yaml"
)

func TestChooseModelPrefersKnownNames(t *testing.T) {
	models := []v1.Model{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ollama-qwen3"},
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "other",
					TargetModel: "some-other-model",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "openai-gpt-5.4"},
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "gpt-5.4",
					TargetModel: "gpt-5.4",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
	}

	model, err := chooseModel(context.Background(), nil, "", models, types.DefaultModelAliasTypeLLM)
	if err != nil {
		t.Fatalf("expected model, got error: %v", err)
	}

	if model.Name != "openai-gpt-5.4" {
		t.Fatalf("expected openai-gpt-5.4, got %q", model.Name)
	}
}

func TestChooseModelFallsBackToFirstActiveModel(t *testing.T) {
	models := []v1.Model{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "groq-llama-3.1-70b-versatile"},
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "model-a",
					TargetModel: "model-a",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
	}

	model, err := chooseModel(context.Background(), nil, "", models, types.DefaultModelAliasTypeLLM)
	if err != nil {
		t.Fatalf("expected model, got error: %v", err)
	}

	if model.Name != "groq-llama-3.1-70b-versatile" {
		t.Fatalf("expected groq-llama-3.1-70b-versatile, got %q", model.Name)
	}
}

func TestChooseModelPrefersSuggestedOrder(t *testing.T) {
	models := []v1.Model{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "anthropic-claude-sonnet-4-6"},
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "claude-sonnet-4-6",
					TargetModel: "claude-sonnet-4-6",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "openai-gpt-5.4"},
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "gpt-5.4",
					TargetModel: "gpt-5.4",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
	}

	model, err := chooseModel(context.Background(), nil, "", models, types.DefaultModelAliasTypeLLM)
	if err != nil {
		t.Fatalf("expected model, got error: %v", err)
	}

	if model.Name != "openai-gpt-5.4" {
		t.Fatalf("expected openai-gpt-5.4, got %q", model.Name)
	}
}

func TestNanobotParseModelProviderDeclaredDialectDrivesURL(t *testing.T) {
	h := &Handler{serverURL: "https://obot.example.com"}

	for _, tc := range []struct {
		dialect     nanobottypes.Dialect
		wantBaseURL string
	}{
		{nanobottypes.DialectAnthropicMessages, "https://obot.example.com/api/llm-proxy/anthropic"},
		{nanobottypes.DialectOpenAIResponses, "https://obot.example.com/api/llm-proxy/openai"},
		{nanobottypes.DialectOpenAIChatCompletions, "https://obot.example.com/api/llm-proxy"},
		{nanobottypes.DialectOpenResponses, "https://obot.example.com/api/llm-proxy"},
		{nanobottypes.DialectBifrostRequest, "https://obot.example.com/api/llm-proxy"},
	} {
		model := resolvedLLMModel{
			Name:            "some-model",
			ModelProvider:   "custom-model-provider",
			ProviderDialect: tc.dialect,
		}
		p, _ := h.parseModelProvider(model)
		if p.BaseURL != tc.wantBaseURL {
			t.Errorf("dialect %s: baseURL = %q, want %q", tc.dialect, p.BaseURL, tc.wantBaseURL)
		}
		if p.Dialect != tc.dialect {
			t.Errorf("dialect %s: provider dialect = %q, want same", tc.dialect, p.Dialect)
		}
	}
}

func TestNanobotParseModelProviderBuiltinFallbacks(t *testing.T) {
	h := &Handler{serverURL: "https://obot.example.com"}

	for _, tc := range []struct {
		modelProvider string
		wantDialect   nanobottypes.Dialect
		wantBaseURL   string
	}{
		{system.OpenAIModelProviderTool, nanobottypes.DialectOpenAIResponses, "https://obot.example.com/api/llm-proxy/openai"},
		{system.AnthropicModelProviderTool, nanobottypes.DialectAnthropicMessages, "https://obot.example.com/api/llm-proxy/anthropic"},
		{"unknown-model-provider", nanobottypes.DialectOpenResponses, "https://obot.example.com/api/llm-proxy"},
	} {
		model := resolvedLLMModel{Name: "my-model", ModelProvider: tc.modelProvider}
		p, qualifiedName := h.parseModelProvider(model)
		if p.Dialect != tc.wantDialect {
			t.Errorf("%s: dialect = %q, want %q", tc.modelProvider, p.Dialect, tc.wantDialect)
		}
		if p.BaseURL != tc.wantBaseURL {
			t.Errorf("%s: baseURL = %q, want %q", tc.modelProvider, p.BaseURL, tc.wantBaseURL)
		}
		wantName := tc.modelProvider + "/my-model"
		if qualifiedName != wantName {
			t.Errorf("%s: qualified name = %q, want %q", tc.modelProvider, qualifiedName, wantName)
		}
	}
}

func TestBuildNanobotProviderConfigYAMLSingleProvider(t *testing.T) {
	p := nanobotLLMProvider{
		Name:    "openai-model-provider",
		Dialect: nanobottypes.DialectOpenAIResponses,
		APIKey:  "${OPENAI_MODEL_PROVIDER_API_KEY}",
		BaseURL: "https://obot.example.com/api/llm-proxy/openai",
	}

	yaml, err := buildNanobotProviderConfigYAML(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg nanobottypes.Config
	if err := sigsyaml.Unmarshal([]byte(yaml), &cfg); err != nil {
		t.Fatalf("failed to parse output YAML: %v", err)
	}

	if len(cfg.LLMProviders) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.LLMProviders))
	}
	got := cfg.LLMProviders["openai-model-provider"]
	if got.Dialect != nanobottypes.DialectOpenAIResponses {
		t.Errorf("dialect = %q, want OpenAIResponses", got.Dialect)
	}
	if got.BaseURL != p.BaseURL {
		t.Errorf("baseURL = %q, want %q", got.BaseURL, p.BaseURL)
	}
}

func TestBuildNanobotProviderConfigYAMLMultipleProviders(t *testing.T) {
	openai := nanobotLLMProvider{
		Name:    "openai-model-provider",
		Dialect: nanobottypes.DialectOpenAIResponses,
		APIKey:  "${OPENAI_MODEL_PROVIDER_API_KEY}",
		BaseURL: "https://obot.example.com/api/llm-proxy/openai",
	}
	anthropic := nanobotLLMProvider{
		Name:    "anthropic-model-provider",
		Dialect: nanobottypes.DialectAnthropicMessages,
		APIKey:  "${ANTHROPIC_MODEL_PROVIDER_API_KEY}",
		BaseURL: "https://obot.example.com/api/llm-proxy/anthropic",
	}

	yaml, err := buildNanobotProviderConfigYAML(openai, anthropic)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg nanobottypes.Config
	if err := sigsyaml.Unmarshal([]byte(yaml), &cfg); err != nil {
		t.Fatalf("failed to parse output YAML: %v", err)
	}

	if len(cfg.LLMProviders) != 2 {
		t.Fatalf("expected 2 providers, got %d: %v", len(cfg.LLMProviders), cfg.LLMProviders)
	}
	if cfg.LLMProviders["openai-model-provider"].Dialect != nanobottypes.DialectOpenAIResponses {
		t.Errorf("openai dialect = %q, want OpenAIResponses", cfg.LLMProviders["openai-model-provider"].Dialect)
	}
	if cfg.LLMProviders["anthropic-model-provider"].Dialect != nanobottypes.DialectAnthropicMessages {
		t.Errorf("anthropic dialect = %q, want AnthropicMessages", cfg.LLMProviders["anthropic-model-provider"].Dialect)
	}
}

func TestBuildNanobotProviderConfigYAMLDeduplicates(t *testing.T) {
	p := nanobotLLMProvider{
		Name:    "openai-model-provider",
		Dialect: nanobottypes.DialectOpenAIResponses,
		APIKey:  "${OPENAI_MODEL_PROVIDER_API_KEY}",
		BaseURL: "https://obot.example.com/api/llm-proxy/openai",
	}

	yaml, err := buildNanobotProviderConfigYAML(p, p) // same provider twice
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg nanobottypes.Config
	if err := sigsyaml.Unmarshal([]byte(yaml), &cfg); err != nil {
		t.Fatalf("failed to parse output YAML: %v", err)
	}

	if len(cfg.LLMProviders) != 1 {
		t.Errorf("expected deduplication to 1 provider, got %d", len(cfg.LLMProviders))
	}
}

func TestResolveModelCarriesProviderAndDialect(t *testing.T) {
	c := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			&v1.DefaultModelAlias{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "DefaultModelAlias"},
				ObjectMeta: metav1.ObjectMeta{Name: "llm"},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: "groq-llama"},
				},
			},
			&v1.Model{
				TypeMeta:   metav1.TypeMeta{APIVersion: v1.SchemeGroupVersion.String(), Kind: "Model"},
				ObjectMeta: metav1.ObjectMeta{Name: "groq-llama"},
				Spec: v1.ModelSpec{
					Manifest: types.ModelManifest{
						Name:          "groq-llama",
						TargetModel:   "llama-3.1-70b-versatile",
						ModelProvider: "groq-model-provider",
						Active:        true,
						Usage:         types.ModelUsageLLM,
						Dialect:       string(nanobottypes.DialectOpenAIChatCompletions),
					},
				},
			},
		).Build()

	model, err := resolveModel(context.Background(), c, "", types.DefaultModelAliasTypeLLM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model.Name != "groq-llama" {
		t.Errorf("Name = %q, want groq-llama", model.Name)
	}
	if model.ModelProvider != "groq-model-provider" {
		t.Errorf("ModelProvider = %q, want groq-model-provider", model.ModelProvider)
	}
	if model.ProviderDialect != nanobottypes.DialectOpenAIChatCompletions {
		t.Errorf("ProviderDialect = %q, want OpenAIChatCompletions", model.ProviderDialect)
	}
}

// TestMultipleProvidersWhenLLMAndMiniDiffer verifies that when the default LLM and
// mini-LLM models are on different providers, both providers appear in the generated
// nanobot config YAML.
func TestMultipleProvidersWhenLLMAndMiniDiffer(t *testing.T) {
	h := &Handler{serverURL: "https://obot.example.com"}

	llmModel := resolvedLLMModel{
		Name:          "anthropic-claude-sonnet-4-6",
		ModelProvider: system.AnthropicModelProviderTool,
	}
	miniModel := resolvedLLMModel{
		Name:          "openai-gpt-4.1-mini",
		ModelProvider: system.OpenAIModelProviderTool,
	}

	llmProvider, llmDefault := h.parseModelProvider(llmModel)
	miniProvider, miniDefault := h.parseModelProvider(miniModel)

	if llmDefault != system.AnthropicModelProviderTool+"/anthropic-claude-sonnet-4-6" {
		t.Errorf("llmDefault = %q, want %s/anthropic-claude-sonnet-4-6", llmDefault, system.AnthropicModelProviderTool)
	}
	if miniDefault != system.OpenAIModelProviderTool+"/openai-gpt-4.1-mini" {
		t.Errorf("miniDefault = %q, want %s/openai-gpt-4.1-mini", miniDefault, system.OpenAIModelProviderTool)
	}

	yaml, err := buildNanobotProviderConfigYAML(llmProvider, miniProvider)
	if err != nil {
		t.Fatalf("buildNanobotProviderConfigYAML: %v", err)
	}

	var cfg nanobottypes.Config
	if err := sigsyaml.Unmarshal([]byte(yaml), &cfg); err != nil {
		t.Fatalf("failed to parse output YAML: %v", err)
	}

	if len(cfg.LLMProviders) != 2 {
		t.Fatalf("expected 2 providers (one per model), got %d:\n%s", len(cfg.LLMProviders), yaml)
	}
	if _, ok := cfg.LLMProviders[system.AnthropicModelProviderTool]; !ok {
		t.Errorf("anthropic-model-provider missing from YAML")
	}
	if _, ok := cfg.LLMProviders[system.OpenAIModelProviderTool]; !ok {
		t.Errorf("openai-model-provider missing from YAML")
	}
}

func TestChooseModelMiniFallsBackToResolvedLLM(t *testing.T) {
	client := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			&v1.DefaultModelAlias{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "DefaultModelAlias",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "llm",
				},
				Spec: v1.DefaultModelAliasSpec{
					Manifest: types.DefaultModelAliasManifest{
						Alias: "llm",
						Model: "openai-gpt-5.4",
					},
				},
			},
			&v1.Model{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "Model",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "openai-gpt-5.4",
				},
				Spec: v1.ModelSpec{
					Manifest: types.ModelManifest{
						Name:        "gpt-5.4",
						TargetModel: "gpt-5.4",
						Active:      true,
						Usage:       types.ModelUsageLLM,
					},
				},
			},
		).
		Build()

	models := []v1.Model{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "openai-gpt-5.4"},
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:        "gpt-5.4",
					TargetModel: "gpt-5.4",
					Active:      true,
					Usage:       types.ModelUsageLLM,
				},
			},
		},
	}

	model, err := chooseModel(context.Background(), client, "", models, types.DefaultModelAliasTypeLLMMini)
	if err != nil {
		t.Fatalf("expected model, got error: %v", err)
	}

	if model.Name != "openai-gpt-5.4" {
		t.Fatalf("expected openai-gpt-5.4, got %q", model.Name)
	}
}
