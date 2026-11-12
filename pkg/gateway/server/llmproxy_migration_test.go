package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	aliascontroller "github.com/obot-platform/obot/pkg/controller/handlers/alias"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestMigratedPoliciesAuthorizeNativeNamesAndDenyOtherUsersAndModels(t *testing.T) {
	source := &v1.Model{
		Namespace: system.DefaultNamespace,
		Name:      "m1-legacy-sonnet",
		Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			ModelProvider: system.AnthropicModelProvider,
			TargetModel:   "claude-sonnet-4-5",
			Alias:         "claude-sonnet-4-5",
			Active:        true,
			Usage:         types.ModelUsageLLM,
		}},
	}
	target := source.DeepCopy()
	target.Name = "m1-gateway-sonnet"
	target.Spec.Manifest.Alias = ""
	target.Spec.Manifest.ModelProvider = system.LLMGatewayModelProvider
	target.Spec.Manifest.TargetModel = "anthropic/claude-sonnet-4-5-20250929"
	denied := target.DeepCopy()
	denied.Name = "m1-gateway-gpt"
	denied.Spec.Manifest.TargetModel = "openai/gpt-5"
	dma := &v1.DefaultModelAlias{
		Namespace: system.DefaultNamespace,
		Name:      "llm",
		Spec:      v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{Alias: "llm", Model: source.Name}},
	}
	binding := &v1.Alias{
		Name: alias.KeyFromScopeID("Model", "llm"),
		Spec: v1.AliasSpec{Name: "llm", TargetKind: "DefaultModelAlias", TargetNamespace: source.Namespace, TargetName: dma.Name},
	}
	explicit := &v1.ModelAccessPolicy{
		Namespace: system.DefaultNamespace,
		Name:      "explicit",
		Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
			Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "explicit-user"}},
			Models:   []types.ModelResource{{ID: source.Name}},
		}},
	}
	wildcard := explicit.DeepCopy()
	wildcard.Name = "wildcard"
	wildcard.Spec.Manifest.Subjects[0].ID = "wildcard-user"
	wildcard.Spec.Manifest.Models[0].ID = "claude-*"
	nativeBinding := &v1.Alias{
		Name: alias.KeyFromScopeID("Model", source.Spec.Manifest.Alias),
		Spec: v1.AliasSpec{Name: source.Spec.Manifest.Alias, TargetKind: "Model", TargetNamespace: source.Namespace, TargetName: source.Name},
	}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(llmengine.ModelProviderResource(), source, target, denied, dma, binding, nativeBinding, explicit, wildcard).
		WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			// Production clients unwrap the alias controller's uncached reads.
			return c.Get(ctx, key, untriggered.Unwrap(obj).(kclient.Object), opts...)
		}, List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			return c.List(ctx, untriggered.UnwrapList(list), opts...)
		}}).Build()
	require.NoError(t, llmengine.MigrateLegacyModels(t.Context(), c, llmengine.ModelProviderResource()))
	// Normal alias reconciliation releases retained sources' derived bindings.
	require.NoError(t, aliascontroller.UnassignAlias(router.Request{Ctx: t.Context(), Client: c, Object: nativeBinding}, nil))
	require.True(t, apierrors.IsNotFound(c.Get(t.Context(), kclient.ObjectKeyFromObject(nativeBinding), &v1.Alias{})))
	access := &directAccessBackend{storage: c, informers: map[string]cache.SharedIndexInformer{}}
	helper, err := modelaccesspolicy.NewHelper(t.Context(), access)
	require.NoError(t, err)
	for _, obj := range []kclient.Object{source, target, denied, explicit, wildcard, dma} {
		require.NoError(t, c.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj))
		gvk, err := c.GroupVersionKindFor(obj)
		require.NoError(t, err)
		require.NoError(t, access.informers[gvk.Kind].GetIndexer().Add(obj))
	}
	proxy := &llmProviderProxy{mapHelper: helper}
	backend := &bifrostProviderBackend{}
	for _, tc := range []struct {
		name      string
		principal string
		reference string
		allowed   bool
	}{
		{name: "explicit grant with native name", principal: "explicit-user", reference: "claude-sonnet-4-5", allowed: true},
		{name: "explicit grant with qualified name", principal: "explicit-user", reference: target.Spec.Manifest.TargetModel, allowed: true},
		{name: "migrated default", principal: "explicit-user", reference: "llm", allowed: true},
		{name: "native wildcard retains its meaning", principal: "wildcard-user", reference: "claude-sonnet-4-5", allowed: true},
		{name: "other user denied", principal: "other-user", reference: "claude-sonnet-4-5"},
		{name: "ungranted model denied", principal: "explicit-user", reference: "gpt-5"},
		{name: "wildcard does not grant GPT", principal: "wildcard-user", reference: "gpt-5"},
		{name: "no old ID mapping", principal: "explicit-user", reference: source.Name},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := api.Context{Storage: c, User: &user.DefaultInfo{UID: tc.principal}, Request: httptest.NewRequest(http.MethodPost, "/api/llm-proxy/v1/messages", nil)}
			model, err := backend.authorizeModel(proxy, req, tc.reference, "model")
			if tc.allowed {
				require.NoError(t, err)
				require.Equal(t, target.Name, model.Name)
			} else {
				require.Error(t, err)
			}
		})
	}
	allowed, _, err := helper.GetUserAllowedModels(&user.DefaultInfo{UID: "wildcard-user"})
	require.NoError(t, err)
	require.True(t, allowed[target.Name])
	require.False(t, allowed[source.Name], "retained models must not appear as usable models")
}
