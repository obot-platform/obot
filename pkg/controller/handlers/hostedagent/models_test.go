package hostedagent

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func modelObj(name, target, provider string) *v1.Model {
	return &v1.Model{
		Name: name, Namespace: "obot",
		Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			Name:          name,
			TargetModel:   target,
			ModelProvider: provider,
			Active:        true,
			Usage:         types.ModelUsageLLM,
		}},
	}
}

func aliasObj(name, model string) *v1.DefaultModelAlias {
	return &v1.DefaultModelAlias{
		Name: name, Namespace: "obot",
		Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: name, Model: model}},
	}
}

// A wildcard grants every model. Telling the sandbox about none of them leaves
// an agent unable to use what it was granted, with no way to discover it.
func TestWildcardListsEveryModel(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		modelObj("m1sonnet", "claude-sonnet-4-5", system.AnthropicModelProvider),
		modelObj("m1gpt", "gpt-5", system.OpenAIModelProvider),
		modelObj("m1mini", "gpt-5-mini", system.OpenAIModelProvider),
		aliasObj("llm", "m1sonnet"),
		aliasObj("llm-mini", "m1mini"),
	).Build()

	models, err := modelConfigs(context.Background(), client, "obot", "https://obot.example.com", []string{"*"}, nil)
	if err != nil {
		t.Fatalf("modelConfigs: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("models = %d, want 3: %+v", len(models), models)
	}

	var gotDefault string
	defaults := 0
	for _, model := range models {
		if model.BaseURL == "" || len(model.APIs) == 0 {
			t.Errorf("%s: incomplete endpoint %+v", model.ID, model)
		}
		if model.Default {
			gotDefault = model.ID
			defaults++
		}
	}
	// The default follows the installation's own llm alias.
	if gotDefault != "m1sonnet" {
		t.Errorf("default = %q, want m1sonnet (what obot://llm points at)", gotDefault)
	}
	// Exactly one: an image asking for "the default" must not have to choose.
	if defaults != 1 {
		t.Errorf("models marked default = %d, want 1", defaults)
	}
}

// An alias names one model, and that is all the agent gets.
func TestAliasSelectionResolvesToOneModel(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		modelObj("m1sonnet", "claude-sonnet-4-5", system.AnthropicModelProvider),
		modelObj("m1gpt", "gpt-5", system.OpenAIModelProvider),
		aliasObj("llm", "m1sonnet"),
	).Build()

	models, err := modelConfigs(context.Background(), client, "obot", "https://obot.example.com",
		[]string{types.DefaultModelAliasRefPrefix + "llm"}, nil)
	if err != nil {
		t.Fatalf("modelConfigs: %v", err)
	}
	if len(models) != 1 || models[0].ID != "m1sonnet" || !models[0].Default {
		t.Fatalf("models = %+v", models)
	}
}

// With no alias bound there is still a model to reach for; an image that wants
// one should not have to choose arbitrarily.
func TestSomethingIsAlwaysDefault(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		modelObj("m1gpt", "gpt-5", system.OpenAIModelProvider),
	).Build()

	models, err := modelConfigs(context.Background(), client, "obot", "https://obot.example.com", []string{"*"}, nil)
	if err != nil {
		t.Fatalf("modelConfigs: %v", err)
	}
	if len(models) != 1 || !models[0].Default {
		t.Fatalf("expected a default even with no aliases bound: %+v", models)
	}
}

// A model whose provider has no proxy route cannot be reached, so listing it
// would offer an endpoint that does not exist.
func TestUnroutableProviderIsSkipped(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		modelObj("m1weird", "some-model", "not-a-real-provider"),
		modelObj("m1gpt", "gpt-5", system.OpenAIModelProvider),
	).Build()

	models, err := modelConfigs(context.Background(), client, "obot", "https://obot.example.com", []string{"*"}, nil)
	if err != nil {
		t.Fatalf("modelConfigs: %v", err)
	}
	if len(models) != 1 || models[0].ID != "m1gpt" {
		t.Fatalf("models = %+v", models)
	}
}

// A template is copied to every installation, where a model's resource id is a
// different hash. Naming a model the way its provider does is the only
// preference that survives that copy, so it has to resolve.
func TestProviderNativeIDSelectsAModel(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		modelObj("m1aaa", "claude-haiku-4-5", system.AnthropicModelProvider),
		modelObj("m1zzz", "claude-opus-4-8", system.AnthropicModelProvider),
	).Build()

	models, err := modelConfigs(context.Background(), client, "obot", "https://obot.example.com",
		[]string{"claude-opus-4-8", "*"}, nil)
	if err != nil {
		t.Fatalf("modelConfigs: %v", err)
	}

	// Named first, so it leads the list and is what Default falls back to --
	// rather than m1aaa, which merely sorts first.
	if models[0].Model != "claude-opus-4-8" || !models[0].Default {
		t.Fatalf("expected the named model to lead and be default: %+v", models)
	}
	// The wildcard still contributes the rest, so a preference narrows nothing.
	if len(models) != 2 {
		t.Fatalf("expected the wildcard to still add the others: %+v", models)
	}
}

// The point of listing preferences: without them the fallback is whichever
// resource id sorts first, which has nothing to do with what the agent needs.
// This is the Claude Code case -- a coding agent that was landing on the small
// fast model because the installation's llm alias names a provider it cannot use.
func TestPreferenceBeatsSortOrderWhenTheAliasDoesNotApply(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		modelObj("m1aaa", "claude-haiku-4-5", system.AnthropicModelProvider),
		modelObj("m1zzz", "claude-opus-4-8", system.AnthropicModelProvider),
		modelObj("m1gpt", "gpt-5", system.OpenAIModelProvider),
		// Bound to a model this agent may not use, so it cannot decide.
		aliasObj("llm", "m1gpt"),
	).Build()

	models, err := modelConfigs(context.Background(), client, "obot", "https://obot.example.com",
		[]string{"claude-opus-4-8", "*"}, []string{system.AnthropicModelProvider})
	if err != nil {
		t.Fatalf("modelConfigs: %v", err)
	}
	for _, model := range models {
		if model.Default && model.Model != "claude-opus-4-8" {
			t.Fatalf("default is %q, want the stated preference", model.Model)
		}
	}
	if models[0].Model != "claude-opus-4-8" {
		t.Fatalf("models = %+v", models)
	}
}

// An installation that has the named model is the easy case; one that does not
// still has to produce a working agent, or a template pinning a model would
// break every installation without it.
func TestUnknownPreferenceFallsBackToTheWildcard(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		modelObj("m1aaa", "claude-haiku-4-5", system.AnthropicModelProvider),
	).Build()

	models, err := modelConfigs(context.Background(), client, "obot", "https://obot.example.com",
		[]string{"claude-opus-4-8", "*"}, nil)
	if err != nil {
		t.Fatalf("modelConfigs: %v", err)
	}
	if len(models) != 1 || models[0].Model != "claude-haiku-4-5" || !models[0].Default {
		t.Fatalf("expected the wildcard to carry the agent: %+v", models)
	}
}

// The same provider-native id can be served by more than one provider. Honouring
// a preference from a provider the agent may not use would drop it later, and
// the template would look configured while changing nothing.
func TestPreferenceRespectsTheAgentsProviders(t *testing.T) {
	client := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		modelObj("m1bedrock", "claude-opus-4-8", "bedrock-model-provider"),
		modelObj("m1anthropic", "claude-opus-4-8", system.AnthropicModelProvider),
	).Build()

	models, err := modelConfigs(context.Background(), client, "obot", "https://obot.example.com",
		[]string{"claude-opus-4-8"}, []string{system.AnthropicModelProvider})
	if err != nil {
		t.Fatalf("modelConfigs: %v", err)
	}
	if len(models) != 1 || models[0].ID != "m1anthropic" {
		t.Fatalf("models = %+v", models)
	}
}
