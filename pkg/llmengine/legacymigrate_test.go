package llmengine

import (
	"context"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestMigrateLegacyModels(t *testing.T) {
	ns := system.DefaultNamespace
	model := func(name, provider, target string, edit func(*types.ModelManifest)) *v1.Model {
		m := &v1.Model{Namespace: ns, Name: name, Spec: v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: provider, TargetModel: target, Active: true}}}
		if edit != nil {
			edit(&m.Spec.Manifest)
		}
		return m
	}
	provider := func(name string) *v1.ModelProvider {
		return &v1.ModelProvider{Namespace: ns, Name: name}
	}
	alias := func(name, modelName string) *v1.DefaultModelAlias {
		return &v1.DefaultModelAlias{Namespace: ns, Name: name, Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: name, Model: modelName}}}
	}
	override := &types.ModelCost{Input: 1}

	objects := []kclient.Object{
		provider(system.LLMGatewayModelProvider),
		provider(system.OpenAIModelProvider),
		provider(system.AnthropicModelProvider),
		provider("ollama-model-provider"),
		provider(system.ModelProxyModelProvider),
		// The legacy GPT-5 was deactivated and given a cost override.
		model("legacy-gpt5", system.OpenAIModelProvider, "gpt-5", func(m *types.ModelManifest) { m.Active = false; m.OverrideCost = override }),
		model("legacy-mini", system.OpenAIModelProvider, "gpt-5-mini", nil),
		model("legacy-unused", system.OpenAIModelProvider, "gpt-3.5-turbo", nil),
		// The gateway doesn't serve this one yet, and an alias refers to it.
		model("legacy-sonnet", system.AnthropicModelProvider, "claude-sonnet-9", nil),
		// An undated alias the gateway lists only in its dated forms.
		model("legacy-haiku", system.AnthropicModelProvider, "claude-haiku-4-5", nil),
		// Its name is a prefix of the alias's dated versions, but it isn't one of them.
		model("legacy-haiku-4", system.AnthropicModelProvider, "claude-haiku-4", nil),
		// No import plan; nothing refers to it.
		model("legacy-llama", "ollama-model-provider", "llama3", nil),
		// Deactivated, and the gateway doesn't list it yet; nothing refers to it.
		model("legacy-disabled", system.OpenAIModelProvider, "gpt-9", func(m *types.ModelManifest) { m.Active = false }),
		model("proxy-model", system.ModelProxyModelProvider, "claude", nil),
		model("gw-gpt5", system.LLMGatewayModelProvider, "openai/gpt-5", nil),
		model("gw-mini", system.LLMGatewayModelProvider, "openai/gpt-5-mini", nil),
		model("gw-haiku-old", system.LLMGatewayModelProvider, "anthropic/claude-haiku-4-5-20250101", nil),
		model("gw-haiku", system.LLMGatewayModelProvider, "anthropic/claude-haiku-4-5-20251001", nil),
		alias("llm", "legacy-gpt5"),
		alias("llm-mini", "legacy-mini"),
		alias("vision", "legacy-sonnet"),
		alias("text-embedding", "legacy-haiku-4"),
		alias("image-generation", "legacy-haiku"),
		&v1.ModelAccessPolicy{Namespace: ns, Name: "map1", Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
			Models: []types.ModelResource{
				{ID: "legacy-gpt5"}, {ID: "gw-gpt5"}, {ID: "legacy-mini"}, {ID: "legacy-haiku"}, {ID: "obot://llm"},
				// Patterns for provider-native names: one that matched OpenAI models, one for a
				// provider without an import plan, and one already in gateway form.
				{ID: "gpt-5*"}, {ID: "llama*"}, {ID: "anthropic/claude*"},
			},
		}}},
	}
	c := kfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).
		WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
			return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
		}).Build()
	ctx := context.Background()

	get := func(obj kclient.Object, name string) error {
		return c.Get(ctx, kclient.ObjectKey{Namespace: ns, Name: name}, obj)
	}
	modelOf := func(name string) string {
		var a v1.DefaultModelAlias
		if err := get(&a, name); err != nil {
			t.Fatal(err)
		}
		return a.Spec.Manifest.Model
	}
	exists := func(obj kclient.Object, name string) bool {
		err := get(obj, name)
		if err != nil && !apierrors.IsNotFound(err) {
			t.Fatal(err)
		}
		return err == nil
	}

	// Before credentials are imported, references move but nothing is deleted.
	if err := MigrateLegacyModels(ctx, c, false); err != nil {
		t.Fatal(err)
	}
	if got := modelOf("llm"); got != "gw-gpt5" {
		t.Errorf("llm alias = %s, want gw-gpt5", got)
	}
	if got := modelOf("llm-mini"); got != "gw-mini" {
		t.Errorf("llm-mini alias = %s, want gw-mini", got)
	}
	if got := modelOf("image-generation"); got != "gw-haiku" {
		t.Errorf("image-generation alias = %s, want gw-haiku, the latest dated version of the undated alias", got)
	}
	if got := modelOf("text-embedding"); got != "legacy-haiku-4" {
		t.Errorf("text-embedding alias = %s, want it kept on legacy-haiku-4, which no gateway model serves", got)
	}
	if got := modelOf("vision"); got != "legacy-sonnet" {
		t.Errorf("vision alias = %s, want it kept on the legacy model the gateway doesn't serve", got)
	}
	var policy v1.ModelAccessPolicy
	if err := get(&policy, "map1"); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range policy.Spec.Manifest.Models {
		ids = append(ids, m.ID)
	}
	if want := []string{"gw-gpt5", "gw-mini", "gw-haiku", "obot://llm", "openai/gpt-5*", "llama*", "anthropic/claude*"}; !slices.Equal(ids, want) {
		t.Errorf("policy models = %v, want %v", ids, want)
	}
	var gpt5 v1.Model
	if err := get(&gpt5, "gw-gpt5"); err != nil {
		t.Fatal(err)
	}
	if gpt5.Spec.Manifest.Active || gpt5.Spec.Manifest.OverrideCost == nil {
		t.Errorf("edits of the legacy model weren't carried over: %+v", gpt5.Spec.Manifest)
	}
	if !exists(&v1.Model{}, "legacy-unused") {
		t.Error("legacy models must be kept until credentials are imported")
	}

	// Once imported, migrated and unreferenced legacy models go, and then their providers.
	if err := MigrateLegacyModels(ctx, c, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"legacy-gpt5", "legacy-mini", "legacy-haiku", "legacy-unused", "legacy-llama"} {
		if exists(&v1.Model{}, name) {
			t.Errorf("legacy model %s should be deleted", name)
		}
	}
	for _, name := range []string{"legacy-sonnet", "legacy-disabled", "proxy-model", "gw-gpt5"} {
		if !exists(&v1.Model{}, name) {
			t.Errorf("model %s should be kept", name)
		}
	}
	for _, name := range []string{"ollama-model-provider"} {
		if exists(&v1.ModelProvider{}, name) {
			t.Errorf("legacy model provider %s should be deleted", name)
		}
	}
	// OpenAI keeps the deactivated model the gateway doesn't list yet.
	for _, name := range []string{system.OpenAIModelProvider, system.AnthropicModelProvider, system.LLMGatewayModelProvider, system.ModelProxyModelProvider} {
		if !exists(&v1.ModelProvider{}, name) {
			t.Errorf("model provider %s should be kept", name)
		}
	}
}
