package llmengine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/modeldata/modeldatatest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type listingConfigCodec struct {
	plaintextConfigCodec
	opens atomic.Int32
}

func (c *listingConfigCodec) OpenGatewayConfig(ctx context.Context, id string, data []byte) ([]byte, error) {
	if strings.HasPrefix(id, "provider/") {
		c.opens.Add(1)
	}

	return c.plaintextConfigCodec.OpenGatewayConfig(ctx, id, data)
}

func TestModelListingSharesConfigurationAcrossProviders(t *testing.T) {
	for _, tc := range []struct {
		count int
		azure bool
	}{
		{count: 1},
		{count: 4},
		{
			count: 1,
			azure: true,
		},
		{
			count: 4,
			azure: true,
		},
	} {
		t.Run(fmt.Sprintf("providers=%d/azure=%t", tc.count, tc.azure), func(t *testing.T) {
			count := tc.count
			var loads atomic.Int32
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
					if _, ok := list.(*v1.LLMGatewayProviderList); ok {
						loads.Add(1)
					}

					return c.List(ctx, list, opts...)
				},
			}).Build()
			codec := &listingConfigCodec{}
			store, err := llmcoreclient.New(storage, codec)
			require.NoError(t, err)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"id": "model"}}}))
			}))
			t.Cleanup(upstream.Close)

			for i := range count {
				name := fmt.Sprintf("provider-%d", i)
				provider := llmcoreclient.Provider{
					Config: schemas.ProviderConfig{
						CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI},
						NetworkConfig: schemas.NetworkConfig{
							BaseURL:             upstream.URL,
							AllowPrivateNetwork: true,
						},
					},
					Keys: []schemas.Key{{
						ID:     name,
						Name:   name,
						Value:  *schemas.NewSecretVar("secret"),
						Models: schemas.WhiteList{"*"},
						Weight: 1,
					}},
				}

				require.NoError(t, store.CreateProvider(t.Context(), name, provider))
			}

			dir := t.TempDir()
			modeldatatest.Write(t, dir)
			engine := New()
			require.NoError(t, engine.Start(t.Context(), Options{Store: store, DataDir: dir}))
			t.Cleanup(func() { require.NoError(t, engine.Shutdown(context.Background())) })

			if tc.azure {
				// Keep local OpenAI runtimes: Bifrost cannot initialize custom Azure providers,
				// but their hooks must still list persisted Azure deployments.
				for i := range count {
					name := fmt.Sprintf("provider-%d", i)
					require.NoError(t, store.MutateProvider(t.Context(), name, func(provider *llmcoreclient.Provider) error {
						*provider = azureTestProvider(azureTestKey(name))
						provider.Config.CustomProviderConfig = &schemas.CustomProviderConfig{BaseProviderType: schemas.Azure}

						return nil
					}))
				}

				mockAzureDiscovery(t, engine, func(*http.Request) (*http.Response, error) {
					return azureResponse(http.StatusOK, "deployment"), nil
				})
				require.NoError(t, engine.RefreshAzureDeployments(t.Context()))
				mockAzureDiscovery(t, engine, func(*http.Request) (*http.Response, error) {
					t.Error("public listing must not call ARM")
					return azureResponse(http.StatusForbidden), nil
				})
			}

			for _, provider := range []schemas.ModelProvider{"", "provider-0"} {
				loads.Store(0)
				codec.opens.Store(0)
				models, err := engine.gatewayModels(t.Context(), provider)
				require.NoError(t, err)
				if provider == "" {
					require.Len(t, models, count)
				} else {
					require.Len(t, models, 1)
				}
				if tc.azure {
					require.Equal(t, int32(1), loads.Load(), "one persisted snapshot load, regardless of Azure provider fan-out")
					for _, model := range models {
						require.True(t, strings.HasSuffix(model.ID, "/deployment"), "newly discovered deployments must be visible")
					}
				} else {
					require.Equal(t, int32(1), loads.Load(), "one configuration load, regardless of provider fan-out")
					require.Equal(t, int32(count), codec.opens.Load(), "each provider aggregate is decoded once")
				}
			}
		})
	}
}
