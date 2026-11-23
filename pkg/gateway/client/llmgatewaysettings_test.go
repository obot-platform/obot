package client

import (
	"context"
	"errors"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/gateway/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func legacySubscriptionProvider(t *testing.T, store *llmcoreclient.Client, name string) {
	t.Helper()
	require.NoError(t, store.CreateProvider(t.Context(), name, llmcoreclient.Provider{
		Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{MaxRetries: 7}},
		Keys:   []schemas.Key{{ID: "key", Value: *schemas.NewSecretVar("fixture-secret")}},
	}))
	// Model an aggregate written by old main, before the metadata field existed.
	require.NoError(t, store.MutateProvider(t.Context(), name, func(p *llmcoreclient.Provider) error {
		p.SubscriptionPassthrough = nil
		return nil
	}))
}

func TestMigrateProviderSubscriptions(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			legacy string
			claude bool
		}{
			{
				name: "absent property",
			},
			{
				name:   "main enabled",
				legacy: `{"subscriptionPassthrough":true}`,
				claude: true,
			},
			{
				name:   "main disabled",
				legacy: `{"subscriptionPassthrough":false}`,
			},
		} {
			t.Run(tc.name+map[bool]string{false: "/plain", true: "/encrypted"}[encrypted], func(t *testing.T) {
				c := newTestClient(t)
				if encrypted {
					c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{
						propertyGroupResource:   testTransformer{},
						credentialGroupResource: testTransformer{},
					}}
				}
				storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
				store, err := llmcoreclient.New(storage, c)
				require.NoError(t, err)
				for _, name := range []string{"anthropic", "openai"} {
					legacySubscriptionProvider(t, store, name)
				}
				if tc.legacy != "" {
					_, err = c.SetProperty(t.Context(), "llm-gateway-settings", tc.legacy)
					require.NoError(t, err)
				}
				openaiBefore, err := store.GetProvider(t.Context(), "openai")
				require.NoError(t, err)
				require.NoError(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
				anthropic, err := store.GetProvider(t.Context(), "anthropic")
				require.NoError(t, err)
				require.Equal(t, new(tc.claude), anthropic.SubscriptionPassthrough)
				require.Equal(t, 7, anthropic.Config.NetworkConfig.MaxRetries)
				require.Equal(t, "fixture-secret", anthropic.Keys[0].Value.GetValue())
				openaiAfter, err := store.GetProvider(t.Context(), "openai")
				require.NoError(t, err)
				require.Equal(t, openaiBefore, openaiAfter, "migration must not modify OpenAI")
				if tc.legacy != "" {
					p, err := c.GetProperty(t.Context(), "llm-gateway-settings")
					require.NoError(t, err)
					require.Equal(t, tc.legacy, p.Value, "retain the migration source")
				}
				// A completed migration never restores revoked flags, even with a broken old source.
				require.NoError(t, store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
					p.SubscriptionPassthrough = new(false)
					return nil
				}))
				_, err = c.SetProperty(t.Context(), "llm-gateway-settings", "broken")
				require.NoError(t, err)
				require.NoError(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
				p, err := store.GetProvider(t.Context(), "anthropic")
				require.NoError(t, err)
				require.Equal(t, new(false), p.SubscriptionPassthrough)
			})
		}
	}
}

func TestSubscriptionMigrationExplicitAndMissingProviders(t *testing.T) {
	c := newTestClient(t)
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), c)
	require.NoError(t, err)
	_, err = c.SetProperty(t.Context(), "llm-gateway-settings", `{"subscriptionPassthrough":true}`)
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{SubscriptionPassthrough: new(false)}))
	require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{SubscriptionPassthrough: new(true)}))
	require.NoError(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
	p, err := store.GetProvider(t.Context(), "anthropic")
	require.NoError(t, err)
	require.Equal(t, new(false), p.SubscriptionPassthrough)
	p, err = store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Equal(t, new(true), p.SubscriptionPassthrough)

	// Simulate another replica already running the callback after this provider was deleted.
	require.NoError(t, c.db.WithContext(t.Context()).Where("name = ?", "provider-owned-subscription-passthrough").Delete(&types.Migration{}).Error)
	require.NoError(t, store.DeleteProvider(t.Context(), "anthropic"))
	require.NoError(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
	_, err = store.GetProvider(t.Context(), "anthropic")
	require.True(t, apierrors.IsNotFound(err))
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{}))
	require.NoError(t, c.db.WithContext(t.Context()).Where("name = ?", "provider-owned-subscription-passthrough").Delete(&types.Migration{}).Error)
	require.NoError(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
	p, err = store.GetProvider(t.Context(), "anthropic")
	require.NoError(t, err)
	require.Equal(t, new(false), p.SubscriptionPassthrough, "recreation must not inherit the old property")
}

func TestSubscriptionMigrationMalformedSettings(t *testing.T) {
	for _, body := range []string{"broken", "null", "[]", `{"subscriptionPassthrough":"true"}`, `{"subscriptionPassthrough":null}`, `{"subscriptionPassthrough":true} trailing`} {
		t.Run(body, func(t *testing.T) {
			c := newTestClient(t)
			store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), c)
			require.NoError(t, err)
			legacySubscriptionProvider(t, store, "anthropic")
			_, err = c.SetProperty(t.Context(), "llm-gateway-settings", body)
			require.NoError(t, err)
			require.NoError(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
			provider, err := store.GetProvider(t.Context(), "anthropic")
			require.NoError(t, err)
			require.Equal(t, new(false), provider.SubscriptionPassthrough)

			var count int64
			require.NoError(t, c.db.WithContext(t.Context()).Model(&types.Migration{}).Where("name = ?", "provider-owned-subscription-passthrough").Count(&count).Error)
			require.EqualValues(t, 1, count)
			_, err = c.SetProperty(t.Context(), "llm-gateway-settings", `{"subscriptionPassthrough":true}`)
			require.NoError(t, err)
			require.NoError(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
			provider, err = store.GetProvider(t.Context(), "anthropic")
			require.NoError(t, err)
			require.Equal(t, new(false), provider.SubscriptionPassthrough, "completed migration must not reread the legacy setting")
		})
	}
}

func TestSubscriptionMigrationFailures(t *testing.T) {
	t.Run("decryption", func(t *testing.T) {
		c := newTestClient(t)
		c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{propertyGroupResource: testTransformer{}}}
		require.NoError(t, c.db.WithContext(t.Context()).Create(&types.Property{Key: "llm-gateway-settings", Value: "invalid base64!", Encrypted: true}).Error)
		store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), c)
		require.NoError(t, err)
		require.Error(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
	})
	t.Run("resource storage", func(t *testing.T) {
		c := newTestClient(t)
		storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(context.Context, kclient.WithWatch, kclient.ObjectKey, kclient.Object, ...kclient.GetOption) error {
				return errors.New("resource read failed")
			},
		}).Build()
		store, err := llmcoreclient.New(storage, c)
		require.NoError(t, err)
		require.ErrorContains(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store), "resource read failed")
	})
}

func TestSubscriptionMigrationConcurrentResourceEdits(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "key edit", true: "revocation"}[explicit], func(t *testing.T) {
			c := newTestClient(t)
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			base, err := llmcoreclient.New(storage, c)
			require.NoError(t, err)
			legacySubscriptionProvider(t, base, "anthropic")
			_, err = c.SetProperty(t.Context(), "llm-gateway-settings", `{"subscriptionPassthrough":true}`)
			require.NoError(t, err)
			injected := false
			wrapped := interceptor.NewClient(storage, interceptor.Funcs{
				Update: func(ctx context.Context, client kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					if _, ok := obj.(*v1.LLMGatewayProvider); ok && !injected {
						injected = true
						require.NoError(t, base.MutateProvider(ctx, "anthropic", func(p *llmcoreclient.Provider) error {
							p.Keys[0].Name = "concurrent edit"
							if explicit {
								p.SubscriptionPassthrough = new(false)
							}
							return nil
						}))
					}
					return client.Update(ctx, obj, opts...)
				},
			})
			store, err := llmcoreclient.New(wrapped, c)
			require.NoError(t, err)
			require.NoError(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
			require.True(t, injected)
			p, err := base.GetProvider(t.Context(), "anthropic")
			require.NoError(t, err)
			require.Equal(t, "concurrent edit", p.Keys[0].Name)
			require.Equal(t, new(!explicit), p.SubscriptionPassthrough)
		})
	}
}

func TestSubscriptionMigrationConcurrentReplicas(t *testing.T) {
	c := newTestClient(t)
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), c)
	require.NoError(t, err)
	legacySubscriptionProvider(t, store, "anthropic")
	_, err = c.SetProperty(t.Context(), "llm-gateway-settings", `{"subscriptionPassthrough":true}`)
	require.NoError(t, err)
	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errs <- c.MigrateLLMGatewaySubscriptions(t.Context(), store)
		}()
	}
	close(start)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	p, err := store.GetProvider(t.Context(), "anthropic")
	require.NoError(t, err)
	require.Equal(t, new(true), p.SubscriptionPassthrough)
}

func TestSubscriptionMigrationUnreadableSource(t *testing.T) {
	t.Run("property storage", func(t *testing.T) {
		c := newTestClient(t)
		store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), c)
		require.NoError(t, err)
		require.NoError(t, c.db.WithContext(t.Context()).Migrator().DropTable(&types.Property{}))
		require.Error(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
	})
	t.Run("provider decoding", func(t *testing.T) {
		c := newTestClient(t)
		storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		store, err := llmcoreclient.New(storage, c)
		require.NoError(t, err)
		legacySubscriptionProvider(t, store, "anthropic")
		var rows v1.LLMGatewayProviderList
		require.NoError(t, storage.List(t.Context(), &rows))
		rows.Items[0].Spec.Payload = "corrupt aggregate"
		require.NoError(t, storage.Update(t.Context(), &rows.Items[0]))
		require.Error(t, c.MigrateLLMGatewaySubscriptions(t.Context(), store))
		var count int64
		require.NoError(t, c.db.WithContext(t.Context()).Model(&types.Migration{}).Where("name = ?", "provider-owned-subscription-passthrough").Count(&count).Error)
		require.Zero(t, count)
	})
}
