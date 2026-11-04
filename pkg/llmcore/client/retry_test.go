package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestWritesReturnConflictsWithoutRetry(t *testing.T) {
	for _, operation := range []string{"provider", "put-key", "delete-key", "mutate-rule", "put-rule", "settings", "create-rule", "create-settings"} {
		t.Run(operation, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			base := newStore(t, storage)
			require.NoError(t, base.CreateProvider(t.Context(), "openai", client.Provider{Keys: []schemas.Key{{ID: "one"}}}))
			if operation != "create-rule" {
				require.NoError(t, base.PutRule(t.Context(), tables.TableRoutingRule{ID: "rule", Name: "original"}))
			}
			if operation != "create-settings" {
				require.NoError(t, base.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
					settings["initial"] = json.RawMessage(`true`)
					return nil
				}))
			}
			failure := apierrors.NewConflict(schema.GroupResource{Resource: "gateway"}, "target", fmt.Errorf("concurrent update"))
			if operation == "create-rule" || operation == "create-settings" {
				failure = apierrors.NewAlreadyExists(schema.GroupResource{Resource: "gateway"}, "target")
			}
			reads, attempts, calls := 0, 0, 0
			wrapped := interceptor.NewClient(storage, interceptor.Funcs{
				Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
					reads++
					return c.Get(ctx, key, obj, opts...)
				},
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					attempts++
					if attempts == 1 {
						return failure
					}
					return c.Update(ctx, obj, opts...)
				},
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					attempts++
					if attempts == 1 {
						return failure
					}
					return c.Create(ctx, obj, opts...)
				},
			})
			store := newStore(t, wrapped)
			var err error
			switch operation {
			case "provider":
				err = store.MutateProvider(t.Context(), "openai", func(p *client.Provider) error {
					calls++
					p.Config.NetworkConfig.BaseURL = "https://example.com"
					return nil
				})
			case "put-key":
				err = store.PutKey(t.Context(), "openai", schemas.Key{ID: "one", Name: "updated"})
			case "delete-key":
				err = store.DeleteKey(t.Context(), "openai", "one")
			case "mutate-rule":
				err = store.MutateRule(t.Context(), "rule", func(rule *tables.TableRoutingRule) error { calls++; rule.Name = "updated"; return nil })
			case "put-rule", "create-rule":
				err = store.PutRule(t.Context(), tables.TableRoutingRule{ID: "rule", Name: "updated"})
			case "settings", "create-settings":
				err = store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
					calls++
					settings["updated"] = json.RawMessage(`true`)
					return nil
				})
			}
			require.ErrorIs(t, err, failure)
			require.Equal(t, 1, reads)
			require.Equal(t, 1, attempts)
			if operation == "provider" || operation == "mutate-rule" || operation == "settings" || operation == "create-settings" {
				require.Equal(t, 1, calls)
			}
		})
	}
}
