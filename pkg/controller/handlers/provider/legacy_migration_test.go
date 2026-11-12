package provider

import (
	"context"
	"fmt"
	"testing"

	openai "github.com/obot-platform/chat-completion-client"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type migrationTestProvider struct{}

func (migrationTestProvider) Ready() bool { return true }

func (migrationTestProvider) HasUpstreamProviders(context.Context) bool { return true }

func (migrationTestProvider) ListModels(context.Context) (*openai.ModelsList, error) {
	return &openai.ModelsList{Models: []openai.Model{{ID: "openai/gpt-5.4"}}}, nil
}

// Production controller clients unwrap dependency-free scans before reading storage.
func listControllerObjects(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
	return c.List(ctx, untriggered.UnwrapList(list), opts...)
}

func TestGatewayModelSyncMigratesReferencesAndRetainsSources(t *testing.T) {
	mp := llmengine.ModelProviderResource()
	// An old completion marker must not re-enable destructive cleanup.
	mp.Annotations = map[string]string{"obot.ai/llm-gateway-legacy-import-v2": "true"}
	legacyProvider := &v1.ModelProvider{
		Namespace: system.DefaultNamespace,
		Name:      system.OpenAIModelProvider,
	}
	legacyModel := &v1.Model{
		Namespace: system.DefaultNamespace,
		Name:      "legacy-model",
		Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			ModelProvider: system.OpenAIModelProvider,
			TargetModel:   "gpt-5.4",
			Active:        false,
			DisplayName:   "Legacy administrator name",
			Usage:         types.ModelUsageLLM,
		}},
	}
	alias := &v1.DefaultModelAlias{
		Namespace: system.DefaultNamespace,
		Name:      "legacy-alias",
		Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{
			Alias: "llm",
			Model: legacyModel.Name,
		}},
	}
	policy := &v1.ModelAccessPolicy{
		Namespace: system.DefaultNamespace,
		Name:      "legacy-policy",
		Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
			Models: []types.ModelResource{{ID: legacyModel.Name}},
		}},
	}
	objects := []kclient.Object{legacyProvider, legacyModel, alias, policy}
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1.SchemeGroupVersion})
	mapper.Add(v1.SchemeGroupVersion.WithKind("Model"), meta.RESTScopeNamespace)
	mapper.Add(v1.SchemeGroupVersion.WithKind("ModelProvider"), meta.RESTScopeNamespace)
	mapper.Add(v1.SchemeGroupVersion.WithKind("ModelInfo"), meta.RESTScopeNamespace)
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithRESTMapper(mapper).
		WithObjects(append(objects, mp)...).
		WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
			return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
		}).WithInterceptorFuncs(interceptor.Funcs{List: listControllerObjects}).Build()
	for _, original := range objects {
		require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(original), original))
	}
	d := dispatcher.New(nil, c, nil, nil, "", "", "")
	d.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, migrationTestProvider{})
	// Earlier handlers compute status before back-population; marking completion must retain it.
	mp.Status.Configured = true
	// A failed publication must leave stored references untouched for retry.
	failing := interceptor.NewClient(c, interceptor.Funcs{Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
		if _, ok := obj.(*v1.Model); ok {
			return fmt.Errorf("simulated publication failure")
		}
		return c.Create(ctx, obj, opts...)
	}})
	require.ErrorContains(t, BackPopulateModels(t.Context(), failing, d, mp), "simulated publication failure")
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(alias), alias))
	require.Equal(t, legacyModel.Name, alias.Spec.Manifest.Model)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, legacyModel.Name, policy.Spec.Manifest.Models[0].ID)
	require.Empty(t, mp.Annotations["obot.ai/llm-gateway-model-migration-complete"])
	require.NoError(t, BackPopulateModels(t.Context(), c, d, mp))
	require.True(t, mp.Status.Configured)

	targetName := modelName(mp.Name, "openai/gpt-5.4")
	var target v1.Model
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKey{Namespace: mp.Namespace, Name: targetName}, &target))
	require.True(t, target.Spec.Manifest.Active)
	require.NotEqual(t, "Legacy administrator name", target.Spec.Manifest.DisplayName)
	var source v1.Model
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(legacyModel), &source))
	require.Equal(t, legacyModel.Spec, source.Spec)
	require.Equal(t, legacyModel, &source)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(legacyProvider), legacyProvider))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(alias), alias))
	require.Equal(t, target.Name, alias.Spec.Manifest.Model)
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(policy), policy))
	require.Equal(t, target.Name, policy.Spec.Manifest.Models[0].ID)
	// A normal resync keeps later edits, rather than replaying the retained source.
	target.Spec.Manifest.Active = true
	target.Spec.Manifest.DisplayName = "New administrator name"
	require.NoError(t, c.Update(t.Context(), &target))
	require.NoError(t, BackPopulateModels(t.Context(), c, d, mp))
	require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(&target), &target))
	require.True(t, target.Spec.Manifest.Active)
	require.Equal(t, "New administrator name", target.Spec.Manifest.DisplayName)
}
