package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestProviderAggregate(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store := newStore(t, storage)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", client.Provider{}))
	require.NoError(t, store.PutKey(t.Context(), "openai", schemas.Key{ID: "key"}))
	var rows v1.LLMGatewayProviderList
	require.NoError(t, storage.List(t.Context(), &rows))
	require.Len(t, rows.Items, 1)
	before, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Error(t, store.PutKey(t.Context(), "missing", schemas.Key{ID: "bad"}))
	require.Error(t, store.PutKey(t.Context(), "openai", schemas.Key{}))
	after, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, store.MutateProvider(t.Context(), "openai", func(provider *client.Provider) error {
		provider.Keys = nil
		provider.Config.NetworkConfig.BaseURL = "https://example.com"
		return nil
	}))
	after, err = store.Load(t.Context())
	require.NoError(t, err)
	require.Empty(t, after.Config.Keys["openai"])
	require.NoError(t, store.DeleteProvider(t.Context(), "openai"))
	after, err = store.Load(t.Context())
	require.NoError(t, err)
	require.Empty(t, after.Config.Providers)
	require.Empty(t, after.Config.Keys)
}
func TestProviderEditPreservesRoutingTargets(t *testing.T) {
	store := newStore(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	require.NoError(t, store.CreateProvider(t.Context(), "openai", client.Provider{}))
	require.NoError(t, store.PutRule(t.Context(), tables.TableRoutingRule{ID: "route", Scope: "global", Targets: []tables.TableRoutingTarget{{RuleID: "route", Provider: new("openai")}}}))
	before, err := store.Load(t.Context())
	require.NoError(t, err)
	require.NoError(t, store.PutKey(t.Context(), "openai", schemas.Key{ID: "key"}))
	after, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, before.Config.Rules, after.Config.Rules)
}

// Reject reads of any other resource, as well as collection reads. This catches
// aggregate reads even when the unrelated configuration happens to be valid.
func TestWritesTouchOnlyOwningResource(t *testing.T) {
	for _, tc := range []struct {
		name  string
		kind  kclient.Object
		write func(*client.Client) error
	}{
		{
			name: "provider",
			kind: &v1.LLMGatewayProvider{},
			write: func(s *client.Client) error {
				return s.MutateProvider(t.Context(), "openai", func(p *client.Provider) error { p.Config.NetworkConfig.BaseURL = "https://example.com"; return nil })
			},
		},
		{
			name:  "key",
			kind:  &v1.LLMGatewayProvider{},
			write: func(s *client.Client) error { return s.PutKey(t.Context(), "openai", schemas.Key{ID: "key"}) },
		},
		{
			name:  "delete key",
			kind:  &v1.LLMGatewayProvider{},
			write: func(s *client.Client) error { return s.DeleteKey(t.Context(), "openai", "key") },
		},
		{
			name:  "delete provider",
			kind:  &v1.LLMGatewayProvider{},
			write: func(s *client.Client) error { return s.DeleteProvider(t.Context(), "openai") },
		},
		{
			name:  "rule",
			kind:  &v1.LLMGatewayRoutingRule{},
			write: func(s *client.Client) error { return s.PutRule(t.Context(), tables.TableRoutingRule{ID: "route"}) },
		},
		{
			name:  "delete rule",
			kind:  &v1.LLMGatewayRoutingRule{},
			write: func(s *client.Client) error { return s.DeleteRule(t.Context(), "route") },
		},
		{
			name: "settings",
			kind: &v1.LLMGatewayConfig{},
			write: func(s *client.Client) error {
				return s.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
					settings["client"] = json.RawMessage(`{}`)
					return nil
				})
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			base := newStore(t, storage)
			require.NoError(t, base.CreateProvider(t.Context(), "openai", client.Provider{Keys: []schemas.Key{{ID: "key"}}}))
			require.NoError(t, base.CreateProvider(t.Context(), "other", client.Provider{}))
			other := func(obj kclient.Object) bool { return reflect.TypeOf(obj) != reflect.TypeOf(tc.kind) }
			isolated := interceptor.NewClient(storage, interceptor.Funcs{
				List: func(context.Context, kclient.WithWatch, kclient.ObjectList, ...kclient.ListOption) error {
					return fmt.Errorf("unexpected collection read")
				},
				Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
					if other(obj) {
						return fmt.Errorf("unexpected read of %T", obj)
					}
					if p, ok := obj.(*v1.LLMGatewayProvider); ok {
						var rows v1.LLMGatewayProviderList
						require.NoError(t, storage.List(ctx, &rows))
						for _, row := range rows.Items {
							if row.Spec.ID == "other" && key.Name == row.Name {
								return fmt.Errorf("unexpected read of unrelated provider %T", p)
							}
						}
					}
					return c.Get(ctx, key, obj, opts...)
				},
			})
			require.NoError(t, tc.write(newStore(t, isolated)))
		})
	}
}

func TestProviderConflictRetriesLatestAggregate(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	base := newStore(t, storage)
	require.NoError(t, base.CreateProvider(t.Context(), "openai", client.Provider{}))
	conflict := true
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if conflict {
				conflict = false
				require.NoError(t, base.PutKey(ctx, "openai", schemas.Key{ID: "concurrent"}))
				return apierrors.NewConflict(schema.GroupResource{Resource: "llmgatewayproviders"}, obj.GetName(), fmt.Errorf("concurrent key update"))
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	calls := 0
	require.NoError(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return newStore(t, wrapped).MutateProvider(t.Context(), "openai", func(provider *client.Provider) error {
			calls++
			provider.Config.NetworkConfig.BaseURL = "https://example.com"
			return nil
		})
	}))
	require.Equal(t, 2, calls)
	provider, err := base.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Equal(t, "https://example.com", provider.Config.NetworkConfig.BaseURL)
	require.Len(t, provider.Keys, 1)
	require.Equal(t, "concurrent", provider.Keys[0].ID)
}

func TestProviderMutationValidationRollsBack(t *testing.T) {
	store := newStore(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	require.NoError(t, store.CreateProvider(t.Context(), "openai", client.Provider{Keys: []schemas.Key{{ID: "one"}}}))
	before, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.ErrorContains(t, store.MutateProvider(t.Context(), "openai", func(provider *client.Provider) error {
		provider.Keys = append(provider.Keys, schemas.Key{ID: "one"})
		return nil
	}), "duplicate key ID")
	require.Error(t, store.MutateProvider(t.Context(), "openai", func(provider *client.Provider) error { provider.Keys = nil; return fmt.Errorf("rejected") }))
	after, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestUnrelatedInvalidProviderDoesNotBlockWrites(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store := newStore(t, storage)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", client.Provider{}))
	require.NoError(t, store.CreateProvider(t.Context(), "broken", client.Provider{}))
	var providers v1.LLMGatewayProviderList
	require.NoError(t, storage.List(t.Context(), &providers))
	for _, provider := range providers.Items {
		if provider.Spec.ID == "broken" {
			provider.Spec.Payload = "invalid JSON"
			require.NoError(t, storage.Update(t.Context(), &provider))
		}
	}
	require.NoError(t, store.PutKey(t.Context(), "openai", schemas.Key{ID: "key"}))
	require.NoError(t, store.PutRule(t.Context(), tables.TableRoutingRule{ID: "route"}))
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["client"] = json.RawMessage(`{}`)
		return nil
	}))
	provider, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Len(t, provider.Keys, 1)
	result, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Len(t, result.ProviderErrors, 1, "runtime assembly reports the unreadable provider")
}

func TestSettingsConflictPreservesOtherSettings(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	base := newStore(t, storage)
	require.NoError(t, base.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["initial"] = json.RawMessage(`true`)
		return nil
	}))
	conflict := true
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if conflict {
				conflict = false
				require.NoError(t, base.MutateSettings(ctx, func(settings map[string]json.RawMessage) error {
					settings["concurrent"] = json.RawMessage(`true`)
					return nil
				}))
				return apierrors.NewConflict(schema.GroupResource{Resource: "llmgatewayconfigs"}, obj.GetName(), fmt.Errorf("concurrent settings update"))
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	require.NoError(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return newStore(t, wrapped).MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
			settings["edited"] = json.RawMessage(`true`)
			return nil
		})
	}))
	settings, err := base.GetSettings(t.Context())
	require.NoError(t, err)
	require.Len(t, settings, 3)
}

func TestCreateProviderPreservesExistingAggregate(t *testing.T) {
	store := newStore(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	original := client.Provider{Keys: []schemas.Key{{ID: "existing", Value: *schemas.NewSecretVar("existing-secret")}}}
	require.NoError(t, store.CreateProvider(t.Context(), "openai", original))
	require.True(t, apierrors.IsAlreadyExists(store.CreateProvider(t.Context(), "openai", client.Provider{})))
	got, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Equal(t, original, got)
	require.Error(t, store.CreateProvider(t.Context(), "invalid", client.Provider{Keys: []schemas.Key{{}}}))
	require.Error(t, store.CreateProvider(t.Context(), "duplicate", client.Provider{Keys: []schemas.Key{{ID: "same"}, {ID: "same"}}}))
}

func TestRuleConflictRetriesLatestFields(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	base := newStore(t, storage)
	require.NoError(t, base.PutRule(t.Context(), tables.TableRoutingRule{ID: "route", Name: "original", Priority: 1}))
	conflict := true
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if conflict {
				conflict = false
				require.NoError(t, base.MutateRule(ctx, "route", func(rule *tables.TableRoutingRule) error { rule.Priority = 7; return nil }))
				return apierrors.NewConflict(schema.GroupResource{Resource: "llmgatewayroutingrules"}, obj.GetName(), fmt.Errorf("concurrent edit"))
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	calls := 0
	require.NoError(t, retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return newStore(t, wrapped).MutateRule(t.Context(), "route", func(rule *tables.TableRoutingRule) error { calls++; rule.Name = "renamed"; return nil })
	}))
	rule, err := base.GetRule(t.Context(), "route")
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, "renamed", rule.Name)
	require.Equal(t, 7, rule.Priority)
}
