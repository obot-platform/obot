package llmengine

import (
	"context"
	"fmt"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
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

func publishMigrationModels(t *testing.T, c kclient.Client, desired ...*v1.Model) error {
	t.Helper()
	var objects []kclient.Object
	for _, m := range desired {
		objects = append(objects, m)
	}
	provider := ModelProviderResource()
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
	return MigrateLegacyModels(t.Context(), c, provider, objects, func() error {
		for _, m := range desired {
			var current v1.Model
			err := c.Get(t.Context(), kclient.ObjectKeyFromObject(m), &current)
			if err == nil {
				// Normal discovery keeps the current administrator edits.
				current.Spec = m.Spec
				err = c.Update(t.Context(), &current)
			} else {
				err = c.Create(t.Context(), m)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func TestMigrateLegacyModels(t *testing.T) {
	source := migrationModel("m1-legacy", system.AnthropicModelProvider, "claude-sonnet-4-5")
	source.Spec.Manifest.DisplayName = "My Claude"
	source.Spec.Manifest.Active = false
	source.Spec.Manifest.Alias = "custom-claude"
	source.Spec.Manifest.OverrideCost = &types.ModelCost{Input: 7}
	unmatched := migrationModel("m1-unmatched", system.OpenAIModelProvider, "gpt-unavailable")
	unsupported := migrationModel("m1-unsupported", "ollama-model-provider", "llama3")
	target := migrationModel("m1-gateway", system.LLMGatewayModelProvider, "anthropic/claude-sonnet-4-5-20250929")
	dma := &v1.DefaultModelAlias{
		Namespace: system.DefaultNamespace,
		Name:      "llm",
		Spec:      v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: source.Name}},
	}
	policy := migrationPolicy(source.Name, target.Name, unmatched.Name, "obot://llm", "claude-*", "bedrock/anthropic.*")
	binding := &v1.Alias{
		Name: alias.KeyFromScopeID("Model", source.Spec.Manifest.Alias),
		Spec: v1.AliasSpec{Name: source.Spec.Manifest.Alias, TargetKind: "Model", TargetName: source.Name, TargetNamespace: source.Namespace},
	}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource(), source, unmatched, unsupported, dma, policy, binding).Build()
	require.NoError(t, publishMigrationModels(t, c, target))
	// The published destination starts with the legacy disabled state and edits.
	require.False(t, target.Spec.Manifest.Active)
	require.Equal(t, "My Claude", target.Spec.Manifest.DisplayName)
	require.Equal(t, source.Spec.Manifest.Alias, target.Spec.Manifest.Alias)
	require.Equal(t, source.Spec.Manifest.OverrideCost, target.Spec.Manifest.OverrideCost)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(dma), dma))
	require.Equal(t, target.Name, dma.Spec.Manifest.Model)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, []types.ModelResource{{ID: target.Name}, {ID: unmatched.Name}, {ID: "obot://llm"}, {ID: "claude-*"}, {ID: "bedrock/anthropic.*"}}, policy.Spec.Manifest.Models)
	require.Equal(t, "allowed-user", policy.Spec.Manifest.Subjects[0].ID)
	var retained v1.Model
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(source), &retained))
	require.Equal(t, source.Spec, retained.Spec)
	require.Equal(t, target.Name, retained.Annotations[system.LegacyModelMigrationAnnotation])
	require.Empty(t, retained.GetAliasName(), "retained source cannot reclaim its old alias")
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(unmatched), unmatched))
	require.Empty(t, unmatched.Annotations)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(unsupported), unsupported))
	require.Error(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(binding), binding))
	// Later administrator edits and deletion/recreation never replay consumed settings.
	target.Spec.Manifest.Active = true
	target.Spec.Manifest.DisplayName = "Administrator edit"
	require.NoError(t, publishMigrationModels(t, c, target))
	require.True(t, target.Spec.Manifest.Active)
	require.Equal(t, "Administrator edit", target.Spec.Manifest.DisplayName)
	require.NoError(t, c.Delete(t.Context(), target))
	recreated := migrationModel(target.Name, system.LLMGatewayModelProvider, target.Spec.Manifest.TargetModel)
	require.NoError(t, publishMigrationModels(t, c, recreated))
	require.True(t, recreated.Spec.Manifest.Active)
	require.Empty(t, recreated.Spec.Manifest.Alias)
}

func TestMigrationPreservesExistingGatewaySettingsAndUnrelatedAlias(t *testing.T) {
	source := migrationModel("m1-legacy", system.OpenAIModelProvider, "gpt-5")
	source.Spec.Manifest.Active = false
	source.Spec.Manifest.Alias = "gpt-custom"
	target := migrationModel("m1-gateway", system.LLMGatewayModelProvider, "openai/gpt-5")
	target.Spec.Manifest.DisplayName = "Existing admin name"
	policy := migrationPolicy(source.Name)
	binding := &v1.Alias{Name: alias.KeyFromScopeID("Model", source.Spec.Manifest.Alias), Spec: v1.AliasSpec{TargetKind: "Model", TargetNamespace: source.Namespace, TargetName: "another-model"}}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource(), source, target, policy, binding).Build()
	require.NoError(t, publishMigrationModels(t, c, target))
	require.True(t, target.Spec.Manifest.Active)
	require.Equal(t, "Existing admin name", target.Spec.Manifest.DisplayName)
	require.Empty(t, target.Spec.Manifest.Alias)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(binding), binding))
	require.Equal(t, "another-model", binding.Spec.TargetName)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, target.Name, policy.Spec.Manifest.Models[0].ID)
}

func TestMigrationRetriesPartialReferencesWithoutOverwritingAdminEdits(t *testing.T) {
	for _, failure := range []string{"policy", "completion"} {
		t.Run(failure, func(t *testing.T) {
			source := migrationModel("m1-source", system.OpenAIModelProvider, "gpt-5")
			source.Spec.Manifest.DisplayName = "Old name"
			target := migrationModel("m1-target", system.LLMGatewayModelProvider, "openai/gpt-5")
			policy := migrationPolicy(source.Name)
			fail := true
			c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource(), source, policy).WithInterceptorFuncs(interceptor.Funcs{
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					_, policyUpdate := obj.(*v1.ModelAccessPolicy)
					sourceModel, modelUpdate := obj.(*v1.Model)
					sourceUpdate := modelUpdate && sourceModel.Name == source.Name
					if fail && ((failure == "policy" && policyUpdate) || (failure == "completion" && sourceUpdate)) {
						fail = false
						return fmt.Errorf("simulated write failure")
					}
					return c.Update(ctx, obj, opts...)
				},
			}).Build()
			require.ErrorContains(t, publishMigrationModels(t, c, target), "simulated write failure")
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(source), source))
			require.Empty(t, source.Annotations)
			target.Spec.Manifest.DisplayName = "New admin name"
			require.NoError(t, publishMigrationModels(t, c, target))
			require.Equal(t, "New admin name", target.Spec.Manifest.DisplayName)
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
			require.Equal(t, target.Name, policy.Spec.Manifest.Models[0].ID)
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(source), source))
			require.Equal(t, target.Name, source.Annotations[system.LegacyModelMigrationAnnotation])
		})
	}
}

func TestMigrationPublicationFailureLeavesReferencesAndSourcesUntouched(t *testing.T) {
	source := migrationModel("m1-source", system.OpenAIModelProvider, "gpt-5")
	target := migrationModel("m1-target", system.LLMGatewayModelProvider, "openai/gpt-5")
	policy := migrationPolicy(source.Name)
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource(), source, policy).Build()
	require.Error(t, MigrateLegacyModels(t.Context(), c, ModelProviderResource(), []kclient.Object{target}, func() error { return fmt.Errorf("publication failed") }))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, source.Name, policy.Spec.Manifest.Models[0].ID)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(source), source))
	require.Empty(t, source.Annotations)
	require.NoError(t, publishMigrationModels(t, c, target))
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

func TestMigrationWaitsForDiscoveryAndDoesNotReplayLaterSourceEdits(t *testing.T) {
	source := migrationModel("m1-later", system.AzureModelProvider, "my-deployment")
	source.Spec.Manifest.Active = false
	policy := migrationPolicy(source.Name)
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource(), source, policy).Build()
	require.NoError(t, publishMigrationModels(t, c))
	provider := ModelProviderResource()
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
	require.Empty(t, provider.Annotations[legacyModelMigrationCompleteAnnotation], "unmatched models must remain eligible for a later discovery")
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(source), source))
	require.Empty(t, source.Annotations)
	target := migrationModel("m1-gateway", system.LLMGatewayModelProvider, "azure/my-deployment")
	require.NoError(t, publishMigrationModels(t, c, target))
	require.False(t, target.Spec.Manifest.Active)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
	require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation])
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(source), source))
	source.Spec.Manifest.Active = true
	require.NoError(t, c.Update(t.Context(), source))
	require.NoError(t, publishMigrationModels(t, c, target))
	require.False(t, target.Spec.Manifest.Active)
}

func TestLegacyAliasAndDatedModelWithDefaultLabelsShareOneDestination(t *testing.T) {
	first := migrationModel("m1-alias", system.AnthropicModelProvider, "claude-haiku-4-5")
	second := migrationModel("m1-dated", system.AnthropicModelProvider, "claude-haiku-4-5-20251001")
	first.Spec.Manifest.DisplayName = first.Spec.Manifest.TargetModel
	second.Spec.Manifest.DisplayName = second.Spec.Manifest.TargetModel
	target := migrationModel("m1-gateway", system.LLMGatewayModelProvider, "anthropic/claude-haiku-4-5-20251001")
	target.Spec.Manifest.DisplayName = "Claude Haiku"
	policy := migrationPolicy(first.Name, second.Name)
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource(), first, second, policy).Build()
	require.NoError(t, publishMigrationModels(t, c, target))
	require.True(t, target.Spec.Manifest.Active)
	require.Equal(t, "Claude Haiku", target.Spec.Manifest.DisplayName)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, []types.ModelResource{{ID: target.Name}}, policy.Spec.Manifest.Models)
}

func TestCompletedMigrationSkipsInventoryButStillPublishesNewModels(t *testing.T) {
	source := migrationModel("m1-source", system.OpenAIModelProvider, "gpt-5")
	target := migrationModel("m1-target", system.LLMGatewayModelProvider, "openai/gpt-5")
	scans := 0
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource(), source).WithInterceptorFuncs(interceptor.Funcs{
		List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			if _, ok := list.(*v1.ModelList); ok {
				scans++
			}
			return c.List(ctx, list, opts...)
		},
	}).Build()
	require.NoError(t, publishMigrationModels(t, c, target))
	require.Equal(t, 1, scans)
	provider := ModelProviderResource()
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
	require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation])
	newer := migrationModel("m1-new", system.LLMGatewayModelProvider, "openai/gpt-new")
	require.NoError(t, publishMigrationModels(t, c, target, newer))
	require.Equal(t, 1, scans, "completed migration must not scan retained source models")
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(newer), newer))
}

func TestMigrationWithNoSourcesCompletesAndPreservesProviderConfiguration(t *testing.T) {
	provider := ModelProviderResource()
	provider.Annotations = map[string]string{"keep": "admin value"}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(provider).Build()
	require.NoError(t, publishMigrationModels(t, c))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
	require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation])
	require.Equal(t, "admin value", provider.Annotations["keep"])
	require.Equal(t, ModelProviderResource().Spec, provider.Spec)
}

func TestMigrationCompletionMarkerFailureRetriesWithoutReplayingSettings(t *testing.T) {
	source := migrationModel("m1-source", system.OpenAIModelProvider, "gpt-5")
	source.Spec.Manifest.Active = false
	target := migrationModel("m1-target", system.LLMGatewayModelProvider, "openai/gpt-5")
	fail := true
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource(), source).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, patch kclient.Patch, opts ...kclient.PatchOption) error {
			if _, ok := obj.(*v1.ModelProvider); ok && fail {
				fail = false
				return fmt.Errorf("simulated completion marker failure")
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	require.ErrorContains(t, publishMigrationModels(t, c, target), "simulated completion marker failure")
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(source), source))
	require.Equal(t, target.Name, source.Annotations[system.LegacyModelMigrationAnnotation])
	target.Spec.Manifest.Active = true
	require.NoError(t, publishMigrationModels(t, c, target))
	require.True(t, target.Spec.Manifest.Active)
	provider := ModelProviderResource()
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
	require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation])
}

func TestCompletedMigrationUsesLoadedProviderWithoutStorageReads(t *testing.T) {
	provider := ModelProviderResource()
	provider.Annotations = map[string]string{legacyModelMigrationCompleteAnnotation: "true"}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, kclient.WithWatch, kclient.ObjectKey, kclient.Object, ...kclient.GetOption) error {
			t.Fatal("completed migration must use the provider already loaded by the controller")
			return nil
		},
		List: func(context.Context, kclient.WithWatch, kclient.ObjectList, ...kclient.ListOption) error {
			t.Fatal("completed migration must not scan legacy records")
			return nil
		},
	}).Build()
	published := false
	require.NoError(t, MigrateLegacyModels(t.Context(), c, provider, nil, func() error { published = true; return nil }))
	require.True(t, published)
}

func TestConflictingLegacySettingsChooseWinnerAndComplete(t *testing.T) {
	for _, tc := range []struct {
		name           string
		firstProvider  string
		firstTarget    string
		winnerProvider string
		winnerTarget   string
		target         string
		winnerActive   bool
		existing       bool
	}{
		{
			name:           "exact native ID wins over undated alias",
			firstProvider:  system.AnthropicModelProvider,
			firstTarget:    "claude-sonnet-4-5",
			winnerProvider: system.AnthropicModelProvider,
			winnerTarget:   "claude-sonnet-4-5-20250929",
			target:         "anthropic/claude-sonnet-4-5-20250929",
			winnerActive:   true,
		},
		{
			name:           "legacy provider name breaks exact-match tie",
			firstProvider:  system.AzureModelProvider,
			firstTarget:    "deployment",
			winnerProvider: system.AzureEntraModelProvider,
			winnerTarget:   "deployment",
			target:         "azure/deployment",
			winnerActive:   true,
		},
		{
			name:           "record name breaks same-provider tie",
			firstProvider:  system.OpenAIModelProvider,
			firstTarget:    "gpt-5",
			winnerProvider: system.OpenAIModelProvider,
			winnerTarget:   "gpt-5",
			target:         "openai/gpt-5",
			winnerActive:   false,
		},
		{
			name:           "existing gateway settings override all sources",
			firstProvider:  system.AzureModelProvider,
			firstTarget:    "deployment",
			winnerProvider: system.AzureEntraModelProvider,
			winnerTarget:   "deployment",
			target:         "azure/deployment",
			winnerActive:   false,
			existing:       true,
		},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reversed=%t", tc.name, reverse), func(t *testing.T) {
				first := migrationModel("m1-b-loser", tc.firstProvider, tc.firstTarget)
				winner := migrationModel("m1-a-winner", tc.winnerProvider, tc.winnerTarget)
				if tc.firstProvider != tc.winnerProvider || tc.firstTarget != tc.winnerTarget {
					first.Name, winner.Name = "m1-a-loser", "m1-z-winner"
				}
				first.Spec.Manifest.Active = !tc.winnerActive
				first.Spec.Manifest.Alias = "loser-alias"
				first.Spec.Manifest.DisplayName = "Loser label"
				first.Spec.Manifest.OverrideCost = &types.ModelCost{Input: 9}
				winner.Spec.Manifest.Active = tc.winnerActive
				winner.Spec.Manifest.Alias = "winner-alias"
				winner.Spec.Manifest.DisplayName = "Winner label"
				winner.Spec.Manifest.OverrideCost = &types.ModelCost{Input: 2}
				target := migrationModel("m1-gateway", system.LLMGatewayModelProvider, tc.target)
				target.Spec.Manifest.DisplayName = "Existing gateway label"
				policy := migrationPolicy(first.Name, winner.Name)
				dma := &v1.DefaultModelAlias{
					Namespace: system.DefaultNamespace,
					Name:      "llm",
					Spec:      v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: first.Name}},
				}
				objects := []kclient.Object{ModelProviderResource(), first, winner, policy, dma}
				if tc.existing {
					objects = append(objects, target)
				}
				scans := 0
				c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).WithInterceptorFuncs(interceptor.Funcs{
					List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
						if err := c.List(ctx, list, opts...); err != nil {
							return err
						}
						if models, ok := list.(*v1.ModelList); ok {
							scans++
							if reverse {
								for i, j := 0, len(models.Items)-1; i < j; i, j = i+1, j-1 {
									models.Items[i], models.Items[j] = models.Items[j], models.Items[i]
								}
							}
						}
						return nil
					},
				}).Build()
				require.NoError(t, publishMigrationModels(t, c, target))
				if tc.existing {
					require.True(t, target.Spec.Manifest.Active)
					require.Equal(t, "Existing gateway label", target.Spec.Manifest.DisplayName)
					require.Empty(t, target.Spec.Manifest.Alias)
					require.Nil(t, target.Spec.Manifest.OverrideCost)
				} else {
					require.Equal(t, tc.winnerActive, target.Spec.Manifest.Active)
					require.Equal(t, "Winner label", target.Spec.Manifest.DisplayName)
					require.Equal(t, "winner-alias", target.Spec.Manifest.Alias)
					require.Equal(t, winner.Spec.Manifest.OverrideCost, target.Spec.Manifest.OverrideCost)
				}
				require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
				require.Equal(t, []types.ModelResource{{ID: target.Name}}, policy.Spec.Manifest.Models)
				require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(dma), dma))
				require.Equal(t, target.Name, dma.Spec.Manifest.Model)
				for _, source := range []*v1.Model{first, winner} {
					require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(source), source))
					require.Equal(t, target.Name, source.Annotations[system.LegacyModelMigrationAnnotation])
				}
				provider := ModelProviderResource()
				require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(provider), provider))
				require.Equal(t, "true", provider.Annotations[legacyModelMigrationCompleteAnnotation])
				require.Equal(t, 1, scans)
				target.Spec.Manifest.Active = true
				target.Spec.Manifest.DisplayName = "Later admin edit"
				require.NoError(t, publishMigrationModels(t, c, target))
				require.Equal(t, 1, scans, "resolved differences must not keep migration scanning")
				require.Equal(t, "Later admin edit", target.Spec.Manifest.DisplayName)
			})
		}
	}
}
