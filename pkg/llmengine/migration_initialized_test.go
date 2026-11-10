package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// Assert the wire field too, so false cannot disappear through omitempty.
func requireMigrationInitialized(t *testing.T, e *Engine, want bool) MigrationStatus {
	t.Helper()
	status, err := e.MigrationStatus(t.Context())
	require.NoError(t, err)
	var wire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(testJSON(t, status), &wire))
	require.Equal(t, string(testJSON(t, want)), string(wire["initialized"]))
	return status
}

func TestMigrationInitializedOldSettings(t *testing.T) {
	for _, raw := range []string{"", `{"mappings":{"old":"target"}}`, `{"sources":{"old":{"status":"imported"}}}`} {
		t.Run(raw, func(t *testing.T) {
			e := New()
			e.store = testOptions(t, Options{}).Store
			if raw != "" {
				require.NoError(t, e.store.MutateSettings(t.Context(), func(s map[string]json.RawMessage) error {
					s[migrationSetting] = json.RawMessage(raw)
					return nil
				}))
			}
			state := requireMigrationInitialized(t, e, false)
			if raw == "" {
				require.Empty(t, state.Sources)
			}
			require.NoError(t, e.mutateMigration(t.Context(), func(s *MigrationStatus) error {
				s.Mappings["new"] = "target"
				return nil
			}))
			requireMigrationInitialized(t, e, false)
			before, err := e.MigrationStatus(t.Context())
			require.NoError(t, err)
			_, err = e.ImportLegacyModelProviders(t.Context(), nil, migrationCredentials(t))
			require.NoError(t, err)
			after := requireMigrationInitialized(t, e, true)
			require.Equal(t, before.Sources, after.Sources)
			require.Equal(t, before.Mappings, after.Mappings)
		})
	}
}

func TestMigrationInitializedEnumerationSnapshot(t *testing.T) {
	for _, name := range []string{"empty", "sources", "old outcomes"} {
		withSources := name != "empty"
		t.Run(name, func(t *testing.T) {
			t.Setenv(system.OpenAIAPIKeyEnvVar, "")
			t.Setenv(system.AnthropicAPIKeyEnvVar, "")
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			g := migrationCredentials(t)
			if withSources {
				putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"invalid": "credential"})
				putLegacy(t, g, system.AnthropicModelProvider, map[string]string{"OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY": "legacy"})
			}
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			if withSources {
				require.NoError(t, storage.Create(ctx, &v1.ModelProvider{
					Namespace: system.DefaultNamespace,
					Name:      "unsupported-provider",
					Status:    v1.ModelProviderStatus{Configured: true},
				}))
				require.NoError(t, storage.Create(ctx, &v1.Model{
					Namespace: system.DefaultNamespace,
					Name:      "unsupported-model",
					Spec:      v1.ModelSpec{Manifest: types.ModelManifest{ModelProvider: "model-only"}},
				}))
			}
			if name == "old outcomes" {
				writer, err := llmcoreclient.New(storage, plaintextConfigCodec{})
				require.NoError(t, err)
				require.NoError(t, writer.MutateSettings(ctx, func(s map[string]json.RawMessage) error {
					// The imported pool was deleted before upgrading from status without the flag.
					s[migrationSetting] = testJSON(t, map[string]any{"sources": map[string]MigrationSource{
						system.AnthropicModelProvider: {
							Name:     "Anthropic",
							Provider: "anthropic",
							Status:   "imported",
							Imported: true,
						},
					}})
					return nil
				}))
			}
			gathering, gathered, committed, finish := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			var gatherOnce, finishOnce, commitOnce sync.Once
			unblock := func() {
				gatherOnce.Do(func() { close(gathered) })
				finishOnce.Do(func() { close(finish) })
			}
			defer unblock()
			pauseCommit := func(ctx context.Context, obj kclient.Object, err error) error {
				if _, ok := obj.(*v1.LLMGatewayConfig); ok && err == nil {
					// Expose the gathered snapshot before any current outcomes can be persisted.
					commitOnce.Do(func() {
						close(committed)
						select {
						case <-finish:
						case <-ctx.Done():
						}
					})
					return ctx.Err()
				}
				return err
			}
			wrapped := interceptor.NewClient(storage, interceptor.Funcs{
				List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
					if _, ok := list.(*v1.ModelProviderList); ok {
						close(gathering)
						select {
						case <-gathered:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					return c.List(ctx, list, opts...)
				},
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					return pauseCommit(ctx, obj, c.Create(ctx, obj, opts...))
				},
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					return pauseCommit(ctx, obj, c.Update(ctx, obj, opts...))
				},
			})
			store, err := llmcoreclient.New(wrapped, plaintextConfigCodec{})
			require.NoError(t, err)
			e := New()
			require.NoError(t, e.Start(ctx, Options{Store: store, DataDir: t.TempDir()}))
			defer func() { require.NoError(t, e.Shutdown(context.Background())) }()
			if name == "sources" {
				require.NoError(t, store.CreateProvider(ctx, "anthropic", llmcoreclient.Provider{}))
			}
			// A separate reader models another replica, without importing or syncing locally.
			reader := New()
			reader.store, err = llmcoreclient.New(storage, plaintextConfigCodec{})
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() {
				defer close(done)
				_, err := e.ImportLegacyModelProviders(ctx, wrapped, g)
				done <- err
			}()
			defer func() { unblock(); cancel(); <-done }()
			select {
			case <-gathering:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.True(t, e.Ready(), "runtime readiness must not imply enumeration completion")
			initial := requireMigrationInitialized(t, reader, false)
			if name == "old outcomes" {
				require.Equal(t, "imported", initial.Sources[system.AnthropicModelProvider].Status)
			} else {
				require.Empty(t, initial.Sources)
			}
			gatherOnce.Do(func() { close(gathered) })
			select {
			case <-committed:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			state := requireMigrationInitialized(t, reader, false)
			if withSources {
				require.Len(t, state.Sources, 4)
				require.Equal(t, "pending", state.Sources[system.OpenAIModelProvider].Status)
				if name == "old outcomes" {
					require.Equal(t, "imported", state.Sources[system.AnthropicModelProvider].Status)
				} else {
					require.Equal(t, "pending", state.Sources[system.AnthropicModelProvider].Status)
				}
				require.Equal(t, "unsupported", state.Sources["unsupported-provider"].Status)
				require.Equal(t, "unsupported", state.Sources["model-only"].Status)
			} else {
				require.Empty(t, state.Sources)
			}
			finishOnce.Do(func() { close(finish) })
			err = <-done
			if withSources {
				require.ErrorContains(t, err, "missing _MODEL_PROVIDER_API_KEY")
				state = requireMigrationInitialized(t, reader, true)
				require.Equal(t, "error", state.Sources[system.OpenAIModelProvider].Status)
				require.Equal(t, "blocked", state.Sources[system.AnthropicModelProvider].Status)
			} else {
				require.NoError(t, err)
				require.Empty(t, requireMigrationInitialized(t, reader, true).Sources)
			}
			require.NoError(t, e.mutateMigration(ctx, func(s *MigrationStatus) error {
				s.Mappings["old"] = "target"
				return nil
			}))
			require.Equal(t, "target", requireMigrationInitialized(t, reader, true).Mappings["old"])
		})
	}
}

func TestMigrationInitializedFailures(t *testing.T) {
	for _, initialized := range []bool{false, true} {
		for _, failure := range []string{"models", "providers", "cancel", "settings", "create"} {
			if initialized && failure == "create" {
				continue // A prior initialized flag requires an existing settings resource.
			}
			t.Run(string(testJSON(t, initialized))+"/"+failure, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				g := migrationCredentials(t)
				storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
				store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
				require.NoError(t, err)
				if failure != "create" {
					require.NoError(t, store.MutateSettings(ctx, func(s map[string]json.RawMessage) error {
						s[migrationSetting] = json.RawMessage(`{"initialized":` + string(testJSON(t, initialized)) + `}`)
						return nil
					}))
				}
				wrapped := interceptor.NewClient(storage, interceptor.Funcs{
					Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
						if _, ok := obj.(*v1.LLMGatewayConfig); ok && failure == "create" {
							return errors.New("settings create failed")
						}
						return c.Create(ctx, obj, opts...)
					},
					List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
						_, models := list.(*v1.ModelList)
						_, providers := list.(*v1.ModelProviderList)
						if (models && failure == "models") || (providers && failure == "providers") {
							return errors.New("enumeration failed")
						}
						if providers && failure == "cancel" {
							cancel()
						}
						return c.List(ctx, list, opts...)
					},
					Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
						if _, ok := obj.(*v1.LLMGatewayConfig); ok && failure == "settings" {
							return errors.New("settings write failed")
						}
						return c.Update(ctx, obj, opts...)
					},
				})
				e := New()
				e.store, err = llmcoreclient.New(wrapped, plaintextConfigCodec{})
				require.NoError(t, err)
				// Ensure a write is attempted even when a prior true flag is already persisted.
				putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"invalid": "credential"})
				_, err = e.ImportLegacyModelProviders(ctx, wrapped, g)
				require.ErrorContains(t, err, map[string]string{
					"models":    "enumeration failed",
					"providers": "enumeration failed",
					"cancel":    "context canceled",
					"settings":  "settings write failed",
					"create":    "settings create failed",
				}[failure])
				require.Empty(t, requireMigrationInitialized(t, e, initialized).Sources)
			})
		}
	}
}

func TestMigrationInitializedFinalWriteFailure(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "")
	g := migrationCredentials(t)
	putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"invalid": "credential"})
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if config, ok := obj.(*v1.LLMGatewayConfig); ok {
				var settings map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(config.Spec.Payload), &settings))
				state, err := decodeMigration(settings)
				require.NoError(t, err)
				if state.Initialized {
					return errors.New("final metadata write failed")
				}
			}
			return c.Update(ctx, obj, opts...)
		},
	}).Build()
	e := New()
	var err error
	e.store, err = llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	_, err = e.ImportLegacyModelProviders(t.Context(), nil, g)
	require.ErrorContains(t, err, "missing _MODEL_PROVIDER_API_KEY")
	require.ErrorContains(t, err, "final metadata write failed")
	state := requireMigrationInitialized(t, e, false)
	require.Equal(t, "error", state.Sources[system.OpenAIModelProvider].Status)
}
