package provider

import (
	"context"
	"os"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	openai "github.com/obot-platform/chat-completion-client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmengine"
	"github.com/obot-platform/obot/pkg/modeldata"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
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

// The engine tests exercise ARM transport. Here an empty upstream listing forces
// the controller's saved-resource retention path without any network request.
type azureRetentionLister struct {
	*llmengine.Engine
	models []openai.Model
}

func (l azureRetentionLister) ListModels(context.Context) (*openai.ModelsList, error) {
	return &openai.ModelsList{Models: l.models}, nil
}

func TestAzureFreshEngineControllerRetention(t *testing.T) {
	for _, outcome := range []string{"outage", "credentials changed", "account changed", "confirmed empty", "base model deprecated", "healthy duplicate first", "healthy duplicate last"} {
		t.Run(outcome, func(t *testing.T) {
			mp := llmengine.ModelProviderResource()
			mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{v1.SchemeGroupVersion})

			for _, kind := range []string{"Model", "ModelInfo", "ModelProvider"} {
				mapper.Add(v1.SchemeGroupVersion.WithKind(kind), meta.RESTScopeNamespace)
			}

			storage := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithRESTMapper(mapper).WithObjects(mp).
				WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string { return []string{o.(*v1.Model).Spec.Manifest.ModelProvider} }).WithInterceptorFuncs(interceptor.Funcs{List: listControllerObjects}).Build()
			store, err := llmcoreclient.New(storage, availabilityTestCodec{})
			require.NoError(t, err)

			key := schemas.Key{ID: "key", Name: "key", Weight: 1, Models: schemas.WhiteList{"*"}, AzureKeyConfig: &schemas.AzureKeyConfig{
				Endpoint: *schemas.NewSecretVar("https://account.openai.azure.com"), ClientID: schemas.NewSecretVar("client"), ClientSecret: schemas.NewSecretVar("secret"), TenantID: schemas.NewSecretVar("tenant"),
			}}
			account := llmcoreclient.AzureAccount{SubscriptionID: "sub", ResourceGroup: "group", AccountName: "account"}
			require.NoError(t, store.CreateProvider(t.Context(), "azure", llmcoreclient.Provider{Keys: []schemas.Key{key}, AzureAccounts: map[string]llmcoreclient.AzureAccount{key.ID: account}}))

			snapshot := llmcoreclient.AzureDiscoverySnapshot{
				Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(key, account),
				Names:       []string{"deployment"},
				BaseModels:  map[string]string{"deployment": "gpt-4o"},
			}
			_, version, err := store.GetProviderWithVersion(t.Context(), "azure")
			require.NoError(t, err)
			require.NoError(t, store.SaveAzureDiscoveries(t.Context(), "azure", version, map[string]llmcoreclient.AzureDiscoverySnapshot{key.ID: snapshot}))

			dir := t.TempDir()
			modeldatatest.Write(t, dir)
			engine := llmengine.New()
			require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: dir}))

			defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()

			d := dispatcher.New(nil, storage, nil, nil, "", "", "")
			lister := azureRetentionLister{Engine: engine, models: []openai.Model{{ID: "azure/deployment", Metadata: map[string]string{
				"displayName": "Deployment", "usage": "chat", "dialect": "openai", llmengine.MetadataInputCost: "0.001",
			}}}}
			d.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, lister)
			require.NoError(t, BackPopulateModels(t.Context(), storage, d, mp))

			var before v1.ModelList
			require.NoError(t, storage.List(t.Context(), &before))
			require.Len(t, before.Items, 1)

			before.Items[0].Spec.Manifest.DisplayName = "Admin edit"
			require.NoError(t, storage.Update(t.Context(), &before.Items[0]))

			var oldInfo v1.ModelInfoList
			require.NoError(t, storage.List(t.Context(), &oldInfo))
			require.Len(t, oldInfo.Items, 1)

			require.NoError(t, engine.Shutdown(context.Background()))

			switch outcome {
			case "credentials changed":
				require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *llmcoreclient.Provider) error {
					p.Keys[0].AzureKeyConfig.ClientSecret = schemas.NewSecretVar("replacement")

					return nil
				}))
			case "account changed":
				require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *llmcoreclient.Provider) error {
					a := p.AzureAccounts[key.ID]
					a.AccountName = "other"
					p.AzureAccounts[key.ID] = a

					return nil
				}))
			case "healthy duplicate first", "healthy duplicate last":
				pricingPath, _ := modeldata.Paths(dir)
				require.NoError(t, os.WriteFile(pricingPath, []byte(`{
 "gpt-4o":{"provider":"azure","mode":"chat","is_deprecated":true},
 "gpt-4o-mini":{"provider":"azure","mode":"chat"}
 }`), 0600))
				require.NoError(t, store.MutateProvider(t.Context(), "azure", func(p *llmcoreclient.Provider) error {
					healthy := key
					healthy.ID, healthy.Name = "healthy", "healthy"
					healthyAccount := account
					healthyAccount.AccountName = "healthy-account"
					p.Keys = append(p.Keys, healthy)
					if outcome == "healthy duplicate first" {
						p.Keys = []schemas.Key{healthy, key}
					}
					p.AzureAccounts[healthy.ID] = healthyAccount
					p.AzureDiscoveries[healthy.ID] = llmcoreclient.AzureDiscoverySnapshot{
						Fingerprint: llmcoreclient.AzureDiscoveryFingerprint(healthy, healthyAccount),
						Names:       []string{"deployment"},
						BaseModels:  map[string]string{"deployment": "gpt-4o-mini"},
					}

					return nil
				}))
			case "base model deprecated":
				pricingPath, _ := modeldata.Paths(dir)
				require.NoError(t, os.WriteFile(pricingPath, []byte(`{"gpt-4o":{"provider":"azure","mode":"chat","is_deprecated":true}}`), 0600))
			case "confirmed empty":
				snapshot.Names = []string{}
				_, version, err = store.GetProviderWithVersion(t.Context(), "azure")
				require.NoError(t, err)
				require.NoError(t, store.SaveAzureDiscoveries(t.Context(), "azure", version, map[string]llmcoreclient.AzureDiscoverySnapshot{key.ID: snapshot}))
			}

			engine = llmengine.New()
			require.NoError(t, engine.Start(t.Context(), llmengine.Options{Store: store, DataDir: dir}))

			lister.Engine, lister.models = engine, nil
			d.RegisterBuiltinModelProvider(mp.Namespace, mp.Name, lister)
			require.NoError(t, BackPopulateModels(t.Context(), storage, d, mp))

			var models v1.ModelList
			var info v1.ModelInfoList
			require.NoError(t, storage.List(t.Context(), &models, kclient.InNamespace(system.DefaultNamespace)))
			require.NoError(t, storage.List(t.Context(), &info, kclient.InNamespace(system.DefaultNamespace)))

			if outcome == "outage" || outcome == "healthy duplicate first" || outcome == "healthy duplicate last" {
				require.Len(t, models.Items, 1)
				require.Equal(t, "Admin edit", models.Items[0].Spec.Manifest.DisplayName)
				require.Len(t, info.Items, 1)
				require.Equal(t, oldInfo.Items[0].Spec, info.Items[0].Spec)
			} else {
				require.Empty(t, models.Items)
				require.Empty(t, info.Items)
			}
		})
	}
}
