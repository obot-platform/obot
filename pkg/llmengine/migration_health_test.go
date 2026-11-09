package llmengine

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/apiclient/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// Use real import provenance, reconciliation and local provider listings.
func migrationHealthFixture(t *testing.T, hooks interceptor.Funcs, plugins ...schemas.LLMPlugin) (*Engine, kclient.Client) {
	t.Helper()
	var e *Engine
	hooks.Delete = func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.DeleteOption) error {
		if model, ok := obj.(*v1.Model); ok && model.Spec.Manifest.ModelProvider != system.LLMGatewayModelProvider {
			state, err := e.MigrationStatus(ctx)
			require.NoError(t, err)
			require.Equal(t, "m1-"+model.Spec.Manifest.TargetModel+"-gateway", state.Mappings[model.Name])
			published, err := e.MigratedReference(ctx, model.Name)
			require.NoError(t, err)
			require.Equal(t, state.Mappings[model.Name], published, "mapping must be locally published before deletion")
		}
		return c.Delete(ctx, obj, opts...)
	}
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(hooks).
		WithIndex(&v1.Model{}, "spec.manifest.modelProvider", func(o kclient.Object) []string {
			return []string{o.(*v1.Model).Spec.Manifest.ModelProvider}
		}).Build()
	store, err := llmcoreclient.New(c, plaintextConfigCodec{})
	require.NoError(t, err)
	e = New()
	var lastPlugin schemas.LLMPlugin
	if len(plugins) > 0 {
		lastPlugin = plugins[0]
	}
	require.NoError(t, e.Start(t.Context(), Options{
		Store:      store,
		DataDir:    t.TempDir(),
		LastPlugin: lastPlugin,
	}))
	t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
	g := migrationCredentials(t)
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "openai-key"})
	putLegacy(t, g, system.AnthropicModelProvider, map[string]string{"OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY": "anthropic-key"})
	for _, provider := range []string{"openai", "anthropic"} {
		source := provider + "-model-provider"
		alias := "llm"
		if provider == "anthropic" {
			alias = "llm-mini"
		}
		for _, obj := range []kclient.Object{
			&v1.ModelProvider{
				Namespace: system.DefaultNamespace,
				Name:      source,
			},
			&v1.Model{
				Namespace: system.DefaultNamespace,
				Name:      "m1-" + provider,
				Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: source,
					TargetModel:   provider,
					Active:        true,
				}},
			},
			&v1.Model{
				Namespace: system.DefaultNamespace,
				Name:      "m1-" + provider + "-gateway",
				Spec: v1.ModelSpec{Manifest: types.ModelManifest{
					ModelProvider: system.LLMGatewayModelProvider,
					TargetModel:   provider + "/" + provider,
					Active:        true,
				}},
			},
			&v1.DefaultModelAlias{
				Namespace: system.DefaultNamespace,
				Name:      provider,
				Spec: v1.DefaultModelAliasSpec{Manifest: types.DefaultModelAliasManifest{
					Alias: alias,
					Model: "m1-" + provider,
				}},
			},
			&v1.ModelAccessPolicy{
				Namespace: system.DefaultNamespace,
				Name:      provider,
				Spec: v1.ModelAccessPolicySpec{Manifest: types.ModelAccessPolicyManifest{
					Subjects: []types.Subject{{
						Type: types.SubjectTypeUser,
						ID:   "migration-user",
					}},
					Models: []types.ModelResource{{ID: "m1-" + provider}},
				}},
			},
		} {
			if policy, ok := obj.(*v1.ModelAccessPolicy); ok {
				require.NoError(t, policy.Spec.Manifest.Validate())
			}
			require.NoError(t, c.Create(t.Context(), obj))
		}
	}
	_, err = e.ImportLegacyModelProviders(t.Context(), c, g)
	require.NoError(t, err)
	for _, provider := range []string{"openai", "anthropic"} {
		up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"` + provider + `","type":"model"}],"has_more":false}`))
		}))
		t.Cleanup(up.Close)
		require.NoError(t, e.store.MutateProvider(t.Context(), provider, func(p *llmcoreclient.Provider) error {
			p.Config.NetworkConfig.BaseURL = up.URL
			p.Config.NetworkConfig.AllowPrivateNetwork = true
			p.Config.NetworkConfig.MaxRetries = 0
			p.Config.ProxyConfig = nil
			return nil
		}))
	}
	_, err = e.Sync(t.Context())
	require.NoError(t, err)
	for _, provider := range []schemas.ModelProvider{schemas.OpenAI, schemas.Anthropic} {
		_, err = e.gatewayModels(t.Context(), provider)
		require.NoError(t, err)
	}
	return e, c
}

func assertMigrationHealthSource(t *testing.T, c kclient.Client, provider string, migrated bool) {
	t.Helper()
	key := kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      "m1-" + provider,
	}
	err := c.Get(t.Context(), key, &v1.Model{})
	if migrated {
		require.True(t, apierrors.IsNotFound(err), "healthy source must be deleted: %v", err)
	} else {
		require.NoError(t, err, "unsafe source must survive")
	}
	key.Name = provider + "-model-provider"
	err = c.Get(t.Context(), key, &v1.ModelProvider{})
	if migrated {
		require.True(t, apierrors.IsNotFound(err), "empty migrated source provider must be deleted")
	} else {
		require.NoError(t, err)
	}
	want := "m1-" + provider
	if migrated {
		want += "-gateway"
	}
	key.Name = provider
	var alias v1.DefaultModelAlias
	require.NoError(t, c.Get(t.Context(), key, &alias))
	require.Equal(t, want, alias.Spec.Manifest.Model)
	var policy v1.ModelAccessPolicy
	require.NoError(t, c.Get(t.Context(), key, &policy))
	require.Equal(t, []types.ModelResource{{ID: want}}, policy.Spec.Manifest.Models)
}

func TestMigrationHealthyDespiteDiagnostics(t *testing.T) {
	for _, diagnostic := range []string{"invalid provider", "unreadable provider", "dangling route"} {
		t.Run(diagnostic, func(t *testing.T) {
			e, c := migrationHealthFixture(t, interceptor.Funcs{})
			switch diagnostic {
			case "invalid provider":
				require.NoError(t, e.store.CreateProvider(t.Context(), "broken", llmcoreclient.Provider{}))
			case "unreadable provider":
				require.NoError(t, c.Create(t.Context(), &v1.LLMGatewayProvider{
					Namespace: system.DefaultNamespace,
					Name:      "broken",
					Spec: v1.LLMGatewayResourceSpec{
						ID:      "broken",
						Payload: "invalid JSON",
					},
				}))
			case "dangling route":
				require.NoError(t, e.store.PutRule(t.Context(), tables.TableRoutingRule{
					ID:            "dangling",
					Name:          "dangling",
					Enabled:       new(true),
					CelExpression: "false",
					Scope:         "global",
					Targets: []tables.TableRoutingTarget{{
						Provider: new("broken"),
						Weight:   1,
					}},
				}))
			}
			_, _ = e.Sync(t.Context())
			require.ErrorContains(t, e.Err(), "broken")
			require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
			assertMigrationHealthSource(t, c, "openai", true)
			assertMigrationHealthSource(t, c, "anthropic", true)
			require.ErrorContains(t, e.Err(), "broken", "independent diagnostics must remain visible")
			require.NoError(t, e.MigrateLegacyModels(t.Context(), c), "repeated migration is idempotent")
		})
	}
}

func TestMigrationFailedTargetDoesNotBlockHealthy(t *testing.T) {
	e, c := migrationHealthFixture(t, interceptor.Funcs{})
	require.NoError(t, e.store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
		p.Config.ConcurrencyAndBufferSize.Concurrency = -1
		return nil
	}))
	_, err := e.Sync(t.Context())
	require.Error(t, err)
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", true)
	assertMigrationHealthSource(t, c, "anthropic", false)
}

func TestMigrationSourceReadFailureDoesNotBlockHealthy(t *testing.T) {
	fail := false
	e, c := migrationHealthFixture(t, interceptor.Funcs{
		Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
			err := c.Get(ctx, key, obj, opts...)
			if p, ok := obj.(*v1.LLMGatewayProvider); ok && p.Spec.ID == "anthropic" && fail {
				return errors.New("source unreadable")
			}
			return err
		},
	})
	fail = true
	require.ErrorContains(t, e.MigrateLegacyModels(t.Context(), c), "source unreadable")
	assertMigrationHealthSource(t, c, "openai", true)
	assertMigrationHealthSource(t, c, "anthropic", false)
}

func TestMigrationBlockingLoadKeepsSources(t *testing.T) {
	for _, phase := range []string{"before migration", "after save with published mappings"} {
		for _, failure := range []string{"collection", "rule", "settings"} {
			t.Run(phase+"/"+failure, func(t *testing.T) {
				armed, blocked := false, false
				settingsReads := 0
				e, c := migrationHealthFixture(t, interceptor.Funcs{
					Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
						if _, ok := obj.(*v1.LLMGatewayConfig); ok && armed {
							if blocked && failure == "settings" {
								return errors.New("blocking settings load")
							}
							settingsReads++
							// Provenance and desired Load read first; mutateMigration reads third,
							// even when identical mappings make its write a no-op.
							if phase != "before migration" && settingsReads == 3 {
								blocked = true
								if failure == "rule" {
									require.NoError(t, c.Create(ctx, &v1.LLMGatewayRoutingRule{
										Namespace: system.DefaultNamespace,
										Name:      "broken-rule",
										Spec: v1.LLMGatewayResourceSpec{
											ID:      "broken-rule",
											Payload: "invalid JSON",
										},
									}))
								}
							}
						}
						return c.Get(ctx, key, obj, opts...)
					},
					List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
						if _, ok := list.(*v1.LLMGatewayProviderList); ok && blocked && failure == "collection" {
							return errors.New("blocking collection load")
						}
						return c.List(ctx, list, opts...)
					},
				})
				if phase != "before migration" {
					require.NoError(t, e.mutateMigration(t.Context(), func(s *MigrationStatus) error {
						for _, provider := range []string{"openai", "anthropic"} {
							s.Mappings["m1-"+provider] = "m1-" + provider + "-gateway"
						}
						return nil
					}))
					_, err := e.Sync(t.Context())
					require.NoError(t, err)
				} else {
					blocked = true
					if failure == "rule" {
						require.NoError(t, c.Create(t.Context(), &v1.LLMGatewayRoutingRule{
							Namespace: system.DefaultNamespace,
							Name:      "broken-rule",
							Spec: v1.LLMGatewayResourceSpec{
								ID:      "broken-rule",
								Payload: "invalid JSON",
							},
						}))
					}
				}
				armed = true
				require.Error(t, e.MigrateLegacyModels(t.Context(), c))
				assertMigrationHealthSource(t, c, "openai", false)
				assertMigrationHealthSource(t, c, "anthropic", false)
				if phase != "before migration" {
					require.Error(t, e.Err(), "the blocking reload must remain visible")
					mapped, err := e.MigratedReference(t.Context(), "m1-openai")
					require.NoError(t, err)
					require.Equal(t, "m1-openai-gateway", mapped, "cached publication alone must not authorize cleanup")
				}
				armed, blocked = false, false
				if failure == "rule" {
					require.NoError(t, c.Delete(t.Context(), &v1.LLMGatewayRoutingRule{
						Namespace: system.DefaultNamespace,
						Name:      "broken-rule",
					}))
				}
				_, err := e.Sync(t.Context())
				require.NoError(t, err)
				require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
				assertMigrationHealthSource(t, c, "openai", true)
				assertMigrationHealthSource(t, c, "anthropic", true)
			})
		}
	}
}

func TestMigrationProviderChangesDuringPublication(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{
			false: "successful replacement",
			true:  "failed replacement",
		}[invalid], func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"data":[{"id":"openai"}]}`))
			}))
			t.Cleanup(up.Close)
			armed := false
			var e *Engine
			e, c := migrationHealthFixture(t, interceptor.Funcs{
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					err := c.Update(ctx, obj, opts...)
					if _, ok := obj.(*v1.LLMGatewayConfig); ok && err == nil && armed {
						armed = false
						require.NoError(t, e.store.MutateProvider(ctx, "openai", func(p *llmcoreclient.Provider) error {
							p.Config.NetworkConfig.BaseURL = up.URL
							if invalid {
								p.Config.ConcurrencyAndBufferSize.Concurrency = -1
							}
							return nil
						}))
					}
					return err
				},
			})
			armed = true
			require.ErrorContains(t, e.MigrateLegacyModels(t.Context(), c), "no longer applied")
			assertMigrationHealthSource(t, c, "openai", false)
			assertMigrationHealthSource(t, c, "anthropic", false)
			state, err := e.MigrationStatus(t.Context())
			require.NoError(t, err)
			require.Equal(t, "m1-openai-gateway", state.Mappings["m1-openai"])
			published, err := e.MigratedReference(t.Context(), "m1-openai")
			require.NoError(t, err)
			require.Equal(t, state.Mappings["m1-openai"], published)
			if invalid {
				require.NoError(t, e.store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
					p.Config.ConcurrencyAndBufferSize.Concurrency = 0
					return nil
				}))
				_, err = e.Sync(t.Context())
				require.NoError(t, err)
			}
			require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
			assertMigrationHealthSource(t, c, "openai", false)
			assertMigrationHealthSource(t, c, "anthropic", true)
			_, err = e.gatewayModels(t.Context(), schemas.OpenAI)
			require.NoError(t, err)
			require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
			assertMigrationHealthSource(t, c, "openai", true)
			assertMigrationHealthSource(t, c, "anthropic", true)
		})
	}
}

func TestMigrationRequiresExpectedPublication(t *testing.T) {
	for _, tc := range []struct {
		failure      string
		prepublished bool
	}{
		{
			failure: "wrong mapping",
		},
		{
			failure: "invalid snapshot",
		},
		{
			failure:      "invalid snapshot with cached mappings",
			prepublished: true,
		},
	} {
		t.Run(tc.failure, func(t *testing.T) {
			armed := false
			settingsReads := 0
			e, c := migrationHealthFixture(t, interceptor.Funcs{
				Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
					err := c.Get(ctx, key, obj, opts...)
					if config, ok := obj.(*v1.LLMGatewayConfig); ok && err == nil && armed {
						settingsReads++
						// Inject at Sync's reload after provenance, desired state and mutateMigration reads.
						if settingsReads != 4 {
							return nil
						}
						// Only the reload is damaged; the committed mappings remain durable for retry.
						var settings map[string]json.RawMessage
						require.NoError(t, json.Unmarshal([]byte(config.Spec.Payload), &settings))
						raw := `{"mappings":{"m1-openai":"wrong-target"}}`
						if tc.failure != "wrong mapping" {
							raw = `{"sources":[],"mappings":{"m1-openai":"m1-openai-gateway"}}`
						}
						settings[migrationSetting] = json.RawMessage(raw)
						config.Spec.Payload = string(mustJSON(t, settings))
					}
					return err
				},
			})
			if tc.prepublished {
				require.NoError(t, e.mutateMigration(t.Context(), func(s *MigrationStatus) error {
					s.Mappings["m1-openai"] = "m1-openai-gateway"
					s.Mappings["m1-anthropic"] = "m1-anthropic-gateway"
					return nil
				}))
				_, err := e.Sync(t.Context())
				require.NoError(t, err)
				published, err := e.MigratedReference(t.Context(), "m1-openai")
				require.NoError(t, err)
				require.Equal(t, "m1-openai-gateway", published)
			}
			armed = true
			require.ErrorContains(t, e.MigrateLegacyModels(t.Context(), c), "was not published")
			assertMigrationHealthSource(t, c, "openai", false)
			assertMigrationHealthSource(t, c, "anthropic", false)
			if tc.failure != "wrong mapping" {
				require.ErrorContains(t, e.Err(), "legacy model mappings", "publication diagnostics must remain visible")
			}
			armed = false
			state, err := e.MigrationStatus(t.Context())
			require.NoError(t, err)
			require.Equal(t, "m1-openai-gateway", state.Mappings["m1-openai"])
			require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
			assertMigrationHealthSource(t, c, "openai", true)
			assertMigrationHealthSource(t, c, "anthropic", true)
		})
	}
}

func TestMigrationRequiresDiscoveryForAppliedConfiguration(t *testing.T) {
	e, c := migrationHealthFixture(t, interceptor.Funcs{})
	var fail atomic.Bool
	fail.Store(true)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"listing unavailable"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"openai"}]}`))
	}))
	t.Cleanup(up.Close)
	require.NoError(t, e.store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
		p.Config.NetworkConfig.BaseURL = up.URL
		p.Keys[0].Value = *schemas.NewSecretVar("rotated-key")
		return nil
	}))
	_, err := e.Sync(t.Context())
	require.NoError(t, err)
	cfg, err := e.config(t.Context())
	require.NoError(t, err)
	require.True(t, e.core.Load().ProviderApplied(schemas.OpenAI, cfg))
	_, err = e.gatewayModels(t.Context(), schemas.OpenAI)
	require.Error(t, err)
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", false)
	assertMigrationHealthSource(t, c, "anthropic", true)
	fail.Store(false)
	_, err = e.gatewayModels(t.Context(), schemas.OpenAI)
	require.NoError(t, err)
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", true)
}

func TestMigrationConfigChangesAfterSyncSnapshot(t *testing.T) {
	armed, afterSave := false, false
	var e *Engine
	e, c := migrationHealthFixture(t, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			err := c.Update(ctx, obj, opts...)
			if _, ok := obj.(*v1.LLMGatewayConfig); ok && err == nil && armed {
				armed, afterSave = false, true
			}
			return err
		},
		List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			err := c.List(ctx, list, opts...)
			if _, ok := list.(*v1.LLMGatewayProviderList); ok && err == nil && afterSave {
				afterSave = false
				// Sync has captured A; the persisted desired keys advance to B before it returns.
				require.NoError(t, e.store.MutateProvider(ctx, "openai", func(p *llmcoreclient.Provider) error {
					p.Keys[0].Value = *schemas.NewSecretVar("after-sync-snapshot")
					return nil
				}))
			}
			return err
		},
	})
	armed = true
	require.ErrorContains(t, e.MigrateLegacyModels(t.Context(), c), "configuration changed")
	assertMigrationHealthSource(t, c, "openai", false)
	assertMigrationHealthSource(t, c, "anthropic", false)
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", false)
	assertMigrationHealthSource(t, c, "anthropic", true)
	_, err := e.gatewayModels(t.Context(), schemas.OpenAI)
	require.NoError(t, err)
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", true)
}

func TestMigrationDiscoveryChangesDuringPublication(t *testing.T) {
	armed := false
	var empty atomic.Bool
	var e *Engine
	e, c := migrationHealthFixture(t, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			err := c.Update(ctx, obj, opts...)
			if _, ok := obj.(*v1.LLMGatewayConfig); ok && err == nil && armed {
				armed = false
				empty.Store(true)
				require.Nil(t, directDiscoveryListing(ctx, e))
			}
			return err
		},
	})
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if empty.Load() {
			_, _ = w.Write([]byte(`{"data":[]}`))
		} else {
			_, _ = w.Write([]byte(`{"data":[{"id":"openai"}]}`))
		}
	}))
	t.Cleanup(up.Close)
	require.NoError(t, e.store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
		p.Config.NetworkConfig.BaseURL = up.URL
		return nil
	}))
	_, err := e.Sync(t.Context())
	require.NoError(t, err)
	require.Nil(t, directDiscoveryListing(t.Context(), e))
	armed = true
	require.ErrorContains(t, e.MigrateLegacyModels(t.Context(), c), "no longer discovered")
	assertMigrationHealthSource(t, c, "openai", false)
	assertMigrationHealthSource(t, c, "anthropic", false)
	empty.Store(false)
	require.Nil(t, directDiscoveryListing(t.Context(), e))
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", true)
	assertMigrationHealthSource(t, c, "anthropic", true)
}

func TestMigrationRequiresDiscoveryForResolvedProviderCA(t *testing.T) {
	e, c := migrationHealthFixture(t, interceptor.Funcs{})
	var listings atomic.Int32
	up := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		listings.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"openai"}]}`))
	}))
	t.Cleanup(up.Close)
	trusted := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: up.Certificate().Raw}))
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(99),
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	require.NoError(t, err)
	untrusted := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	const caEnv = "OBOT_DISCOVERY_PROVIDER_CA"
	t.Setenv(caEnv, trusted)
	require.NoError(t, e.store.MutateProvider(t.Context(), "openai", func(p *llmcoreclient.Provider) error {
		p.Config.NetworkConfig.BaseURL = up.URL
		p.Config.NetworkConfig.CACertPEM = schemas.NewSecretVar("env." + caEnv)
		return nil
	}))
	_, err = e.Sync(t.Context())
	require.NoError(t, err)
	_, err = e.gatewayModels(t.Context(), schemas.OpenAI)
	require.NoError(t, err)
	cfgA, err := e.config(t.Context())
	require.NoError(t, err)
	require.Equal(t, trusted, cfgA.Providers[schemas.OpenAI].NetworkConfig.CACertPEM.GetValue())
	require.Contains(t, e.availability.discoveredModels(cfgA), "openai/openai")
	require.EqualValues(t, 1, listings.Load())

	// Resolve the same stored reference to a valid CA that cannot trust this upstream.
	t.Setenv(caEnv, untrusted)
	_, err = e.Sync(t.Context())
	require.NoError(t, err)
	cfgB, err := e.config(t.Context())
	require.NoError(t, err)
	require.Equal(t, untrusted, cfgB.Providers[schemas.OpenAI].NetworkConfig.CACertPEM.GetValue())
	require.True(t, e.core.Load().ProviderApplied(schemas.OpenAI, cfgB))
	_, err = e.gatewayModels(t.Context(), schemas.OpenAI)
	require.Error(t, err, "the incorrect CA must fail a real TLS listing")
	require.EqualValues(t, 1, listings.Load(), "TLS verification must fail before the HTTP handler")
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", false)
	assertMigrationHealthSource(t, c, "anthropic", true)

	// A new trusted bundle differs from both earlier inputs and needs its own successful poll.
	t.Setenv(caEnv, trusted+untrusted)
	_, err = e.Sync(t.Context())
	require.NoError(t, err)
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", false)
	_, err = e.gatewayModels(t.Context(), schemas.OpenAI)
	require.NoError(t, err)
	require.EqualValues(t, 2, listings.Load(), "the new trusted bundle allows a real provider response")
	require.NoError(t, e.MigrateLegacyModels(t.Context(), c))
	assertMigrationHealthSource(t, c, "openai", true)
}
