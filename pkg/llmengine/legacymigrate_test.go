package llmengine

import (
	"context"
	"fmt"
	"testing"

	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func migrationModel(name, provider, target string) *v1.Model {
	return &v1.Model{
		Namespace: system.DefaultNamespace,
		Name:      name,
		Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			ModelProvider: provider,
			TargetModel:   target,
			Active:        true,
			Usage:         types.ModelUsageLLM,
		}},
	}
}

func migrationPolicy(ids ...string) *v1.ModelAccessPolicy {
	p := &v1.ModelAccessPolicy{
		Namespace: system.DefaultNamespace,
		Name:      "map1-profile",
		Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
			Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "allowed-user"}},
		}},
	}
	for _, id := range ids {
		p.Spec.Manifest.Models = append(p.Spec.Manifest.Models, types.ModelResource{ID: id})
	}
	return p
}

// Model the controller client: wrapped scans read storage without registering dependencies.
func migrationList(t *testing.T) func(context.Context, kclient.WithWatch, kclient.ObjectList, ...kclient.ListOption) error {
	t.Helper()
	return func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
		require.True(t, untriggered.IsWrapped(list), "migration scans must not register controller triggers")
		return c.List(ctx, untriggered.UnwrapList(list), opts...)
	}
}

func TestMigrationRewritesOnlyDefaultAliasesAndProfileReferences(t *testing.T) {
	source := migrationModel("m1-legacy", system.AnthropicModelProvider, "claude-sonnet-4-5")
	source.Spec.Manifest.Active = false
	source.Spec.Manifest.Alias = "custom-claude"
	source.Spec.Manifest.DisplayName = "Old display name"
	source.Spec.Manifest.OverrideCost = &types.ModelCost{Input: 7}
	dated := migrationModel("m1-dated", system.AnthropicModelProvider, "claude-sonnet-4-5-20250929")
	unused := migrationModel("m1-unused", system.OpenAIModelProvider, "unavailable")
	target := migrationModel("m1-gateway", system.LLMGatewayModelProvider, "anthropic/claude-sonnet-4-5-20250929")
	policy := migrationPolicy(source.Name, dated.Name, target.Name, "obot://llm", "claude-*", "anthropic/claude-*")
	wildcard := migrationPolicy("*")
	wildcard.Name = "wildcard"
	dma := &v1.DefaultModelAlias{
		Namespace: system.DefaultNamespace,
		Name:      "llm",
		Spec:      v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: source.Name}},
	}
	provider := ModelProviderResource()
	provider.Annotations = map[string]string{"keep": "admin value"}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(provider, source, dated, unused, target, policy, wildcard, dma).WithInterceptorFuncs(interceptor.Funcs{List: migrationList(t)}).Build()
	originals := []kclient.Object{source.DeepCopy(), dated.DeepCopy(), unused.DeepCopy(), target.DeepCopy(), wildcard.DeepCopy()}
	for _, obj := range originals {
		require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj))
	}
	require.NoError(t, MigrateLegacyModels(t.Context(), c, provider))
	for _, original := range originals {
		current := original.DeepCopyObject().(kclient.Object)
		require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(original), current))
		require.Equal(t, original, current, "models and wildcard-only Profiles must remain untouched")
	}
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, []types.ModelResource{{ID: target.Name}, {ID: "obot://llm"}, {ID: "claude-*"}, {ID: "anthropic/claude-*"}}, policy.Spec.Manifest.Models)
	require.Equal(t, []types.Subject{{Type: types.SubjectTypeUser, ID: "allowed-user"}}, policy.Spec.Manifest.Subjects)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(dma), dma))
	require.Equal(t, target.Name, dma.Spec.Manifest.Model)
	require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation], "unreferenced models must not delay completion")
	require.Equal(t, "admin value", provider.Annotations["keep"])
	require.Equal(t, ModelProviderResource().Spec, provider.Spec)
}

func TestUnsupportedLegacyReferencesDoNotBlockCompletion(t *testing.T) {
	for _, gatewayExists := range []bool{false, true} {
		t.Run(fmt.Sprintf("gatewayExists=%t", gatewayExists), func(t *testing.T) {
			unsupported := migrationModel("m1-vertex", "gemini-vertex-model-provider", "gemini-2.5-pro")
			source := migrationModel("m1-openai", system.OpenAIModelProvider, "gpt-5")
			target := migrationModel("m1-gateway", system.LLMGatewayModelProvider, "openai/gpt-5")
			policy := migrationPolicy(unsupported.Name, source.Name)
			dma := &v1.DefaultModelAlias{
				Namespace: system.DefaultNamespace,
				Name:      "llm",
				Spec:      v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: unsupported.Name}},
			}
			provider := ModelProviderResource()
			objects := []kclient.Object{provider, unsupported, source, target, policy, dma}
			if gatewayExists {
				objects = append(objects, migrationModel("m1-gateway-vertex", system.LLMGatewayModelProvider, "vertex/gemini-2.5-pro"))
			}
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).WithInterceptorFuncs(interceptor.Funcs{List: migrationList(t)}).Build()
			require.NoError(t, MigrateLegacyModels(t.Context(), c, provider))
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
			require.Equal(t, []types.ModelResource{{ID: unsupported.Name}, {ID: target.Name}}, policy.Spec.Manifest.Models)
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(dma), dma))
			require.Equal(t, unsupported.Name, dma.Spec.Manifest.Model)
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(unsupported), &v1.Model{}))
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
			require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation])
		})
	}
}

func TestMigrationRetainsUnmatchedReferencesAndRetriesAfterDiscovery(t *testing.T) {
	source := migrationModel("m1-source", system.AzureEntraModelProvider, "deployment")
	policy := migrationPolicy(source.Name)
	dma := &v1.DefaultModelAlias{
		Namespace: system.DefaultNamespace,
		Name:      "llm",
		Spec:      v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: source.Name}},
	}
	provider := ModelProviderResource()
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(provider, source, policy, dma).WithInterceptorFuncs(interceptor.Funcs{List: migrationList(t)}).Build()
	require.NoError(t, MigrateLegacyModels(t.Context(), c, provider))
	require.Empty(t, provider.Annotations[legacyModelMigrationCompleteAnnotation])
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, source.Name, policy.Spec.Manifest.Models[0].ID)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(dma), dma))
	require.Equal(t, source.Name, dma.Spec.Manifest.Model)
	target := migrationModel("m1-target", system.LLMGatewayModelProvider, "azure/deployment")
	require.NoError(t, c.Create(t.Context(), target))
	require.NoError(t, MigrateLegacyModels(t.Context(), c, provider))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, target.Name, policy.Spec.Manifest.Models[0].ID)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(dma), dma))
	require.Equal(t, target.Name, dma.Spec.Manifest.Model)
	require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation])
}

func TestMigrationRetriesPartialWritesAndPreservesLaterReferenceEdits(t *testing.T) {
	for _, failure := range []string{"policy", "completion", "conflict"} {
		t.Run(failure, func(t *testing.T) {
			source := migrationModel("m1-source", system.OpenAIModelProvider, "gpt-5")
			target := migrationModel("m1-target", system.LLMGatewayModelProvider, "openai/gpt-5")
			policy := migrationPolicy(source.Name)
			dma := &v1.DefaultModelAlias{
				Namespace: system.DefaultNamespace,
				Name:      "llm",
				Spec:      v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: source.Name}},
			}
			provider := ModelProviderResource()
			fail := true
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(provider, source, target, policy, dma).WithInterceptorFuncs(interceptor.Funcs{
				List: migrationList(t),
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					if _, ok := obj.(*v1.ModelAccessPolicy); ok && fail {
						if failure == "policy" {
							fail = false
							return fmt.Errorf("simulated policy failure")
						}
						if failure == "conflict" {
							fail = false
							// A concurrent administrator edit advances the stored resource version.
							var current v1.ModelAccessPolicy
							require.NoError(t, c.Get(ctx, kclient.ObjectKeyFromObject(obj), &current))
							current.Spec.Manifest.DisplayName = "Concurrent administrator edit"
							require.NoError(t, c.Update(ctx, &current))
						}
					}
					return c.Update(ctx, obj, opts...)
				},
				Patch: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, patch kclient.Patch, opts ...kclient.PatchOption) error {
					if fail && failure == "completion" {
						fail = false
						return fmt.Errorf("simulated completion failure")
					}
					return c.Patch(ctx, obj, patch, opts...)
				},
			}).Build()
			err := MigrateLegacyModels(t.Context(), c, provider)
			if failure == "conflict" {
				require.True(t, apierrors.IsConflict(err), "stale writes must fail rather than replace administrator edits")
			} else {
				require.ErrorContains(t, err, "simulated")
			}
			require.Empty(t, provider.Annotations[legacyModelMigrationCompleteAnnotation])
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(dma), dma))
			require.Equal(t, target.Name, dma.Spec.Manifest.Model)
			// An administrator changes an already migrated default before the retry.
			other := migrationModel("m1-other", system.LLMGatewayModelProvider, "openai/gpt-other")
			require.NoError(t, c.Create(t.Context(), other))
			dma.Spec.Manifest.Model = other.Name
			require.NoError(t, c.Update(t.Context(), dma))
			// Simulate a leader change: use the provider as persisted, without process-local state.
			provider = ModelProviderResource()
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
			require.NoError(t, MigrateLegacyModels(t.Context(), c, provider))
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(dma), dma))
			require.Equal(t, other.Name, dma.Spec.Manifest.Model)
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
			require.Equal(t, target.Name, policy.Spec.Manifest.Models[0].ID)
			if failure == "conflict" {
				require.Equal(t, "Concurrent administrator edit", policy.Spec.Manifest.DisplayName)
			}
			require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation])
		})
	}
}

func TestCompletedMigrationUsesLoadedProviderWithoutStorageReads(t *testing.T) {
	provider := ModelProviderResource()
	provider.Annotations = map[string]string{legacyModelMigrationCompleteAnnotation: "true"}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, kclient.WithWatch, kclient.ObjectKey, kclient.Object, ...kclient.GetOption) error {
			t.Fatal("completed migration must use the already loaded provider")
			return nil
		},
		List: func(context.Context, kclient.WithWatch, kclient.ObjectList, ...kclient.ListOption) error {
			t.Fatal("completed migration must not scan legacy records or references")
			return nil
		},
	}).Build()
	require.NoError(t, MigrateLegacyModels(t.Context(), c, provider))
}

func TestGatewayModelMatchStaysWithinProviderAndPrefersExactThenLatestDate(t *testing.T) {
	exact := migrationModel("exact", system.LLMGatewayModelProvider, "anthropic/claude-sonnet-4-5")
	old := migrationModel("old", system.LLMGatewayModelProvider, "anthropic/claude-sonnet-4-5-20250101")
	latest := migrationModel("latest", system.LLMGatewayModelProvider, "anthropic/claude-sonnet-4-5-20250929")
	unrelated := migrationModel("other", system.LLMGatewayModelProvider, "azure/claude-sonnet-4-5-20260101")
	byTarget := map[string]*v1.Model{}
	for _, m := range []*v1.Model{exact, old, latest, unrelated} {
		byTarget[m.Spec.Manifest.TargetModel] = m
	}
	got, ok := gatewayModelFor(byTarget, "anthropic", "claude-sonnet-4-5")
	require.True(t, ok)
	require.Equal(t, exact, got)
	delete(byTarget, exact.Spec.Manifest.TargetModel)
	got, ok = gatewayModelFor(byTarget, "anthropic", "claude-sonnet-4-5")
	require.True(t, ok)
	require.Equal(t, latest, got)
	_, ok = gatewayModelFor(byTarget, "anthropic", "claude-sonnet-4")
	require.False(t, ok)
}
