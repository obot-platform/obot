package provider

import (
	"context"
	"testing"

	openai "github.com/obot-platform/chat-completion-client"
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
)

type migrationTestProvider struct{}

func (migrationTestProvider) Ready() bool { return true }

func (migrationTestProvider) HasUpstreamProviders(context.Context) bool { return true }

func (migrationTestProvider) ListModels(context.Context) (*openai.ModelsList, error) {
	return &openai.ModelsList{Models: []openai.Model{{ID: "openai/gpt-5.4"}}}, nil
}

func TestGatewayModelSyncPreservesLegacyConfiguration(t *testing.T) {
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
			Active:        true,
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
	c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithRESTMapper(mapper).
		WithObjects(append(objects, mp)...).
		WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
			return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
		}).Build()
	for _, original := range objects {
		require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(original), original))
	}
	d := dispatcher.New(nil, c, nil, nil, "", "", "")
	d.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, migrationTestProvider{})
	require.NoError(t, BackPopulateModels(t.Context(), c, d, mp))

	for _, original := range objects {
		got := original.DeepCopyObject().(kclient.Object)
		require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(original), got))
		require.Equal(t, original, got)
	}
}
