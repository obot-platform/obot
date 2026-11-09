package llmengine

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// admissionContext signals entry before the import lock, without sleeps or storage timing.
type admissionContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func TestMigrationCompletedPoolDeletion(t *testing.T) {
	for _, tc := range []struct {
		family  string
		secrets map[string]map[string]string
	}{
		{
			family: "azure",
			secrets: map[string]map[string]string{
				system.AzureModelProvider: azureSecrets("https://one.openai.azure.com"),
				system.AzureEntraModelProvider: {
					"OBOT_AZURE_ENTRA_MODEL_PROVIDER_ENDPOINT":      "https://two.services.ai.azure.com",
					"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_ID":     "client",
					"OBOT_AZURE_ENTRA_MODEL_PROVIDER_CLIENT_SECRET": "secret",
					"OBOT_AZURE_ENTRA_MODEL_PROVIDER_TENANT_ID":     "tenant",
				},
			},
		},
		{
			family: "bedrock_mantle",
			secrets: map[string]map[string]string{
				system.AmazonBedrockModelProvider: {
					"OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID":     "access",
					"OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY": "secret",
					"OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_REGION":            "eu-west-1",
				},
				system.AmazonBedrockAPIKeyModelProvider: {
					"OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_API_KEY": "api-key",
					"OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_REGION":  "us-east-1",
				},
			},
		},
	} {
		t.Run(tc.family, func(t *testing.T) {
			ctx := t.Context()
			g := migrationCredentials(t)
			for name, secrets := range tc.secrets {
				putLegacy(t, g, name, secrets)
			}
			creates, familyReads, outcomes := 0, 0, 0
			deleted := false
			var persisted MigrationStatus
			var store *llmcoreclient.Client
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, c kclient.WithWatch, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
					if _, ok := obj.(*v1.LLMGatewayProvider); ok {
						familyReads++
					}
					return c.Get(ctx, key, obj, opts...)
				},
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					if _, ok := obj.(*v1.LLMGatewayProvider); ok {
						creates++
					}
					return c.Create(ctx, obj, opts...)
				},
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					if err := c.Update(ctx, obj, opts...); err != nil {
						return err
					}
					resource, ok := obj.(*v1.LLMGatewayConfig)
					if !ok || deleted {
						return nil
					}
					var settings map[string]json.RawMessage
					require.NoError(t, json.Unmarshal([]byte(resource.Spec.Payload), &settings))
					state, err := decodeMigration(settings)
					require.NoError(t, err)
					for _, source := range state.Sources {
						if source.Imported {
							outcomes++
							persisted = state
							// Delete immediately after the successful outcome is durable, before this pass returns.
							deleted = true
							require.NoError(t, store.DeleteProvider(ctx, tc.family))
							break
						}
					}
					return nil
				},
			}).Build()
			var err error
			store, err = llmcoreclient.New(storage, plaintextConfigCodec{})
			require.NoError(t, err)
			e := New()
			e.store = store
			results, err := e.ImportLegacyModelProviders(ctx, nil, g)
			require.NoError(t, err)
			require.Len(t, results, 2)
			require.True(t, deleted, "the regression must reach deletion during the import pass")
			require.Equal(t, 1, creates, "a later source must not recreate the deleted pool")
			for name := range tc.secrets {
				require.True(t, persisted.Sources[name].Imported, "all family outcomes must be committed together")
				require.Equal(t, "imported", persisted.Sources[name].Status)
			}
			require.Equal(t, 1, familyReads, "each family must reconcile once per attempt")
			require.Equal(t, 1, outcomes)
			for _, result := range results {
				require.Equal(t, "imported", result.Status)
			}

			recovered := New()
			recovered.store = store
			for _, engine := range []*Engine{e, recovered} {
				results, err = engine.ImportLegacyModelProviders(ctx, nil, g)
				require.NoError(t, err)
				for _, result := range results {
					require.Equal(t, "blocked", result.Status)
				}
				_, err = store.GetProvider(ctx, tc.family)
				require.True(t, apierrors.IsNotFound(err))
			}
			require.Equal(t, 1, creates)
			var providers v1.LLMGatewayProviderList
			require.NoError(t, storage.List(ctx, &providers))
			require.Empty(t, providers.Items, "deletion must leave no hidden provider record")

			manual := llmcoreclient.Provider{}
			manual.Config.NetworkConfig.BaseURL = "https://admin.example.com"
			require.NoError(t, store.CreateProvider(ctx, tc.family, manual))
			_, err = recovered.ImportLegacyModelProviders(ctx, nil, g)
			require.NoError(t, err)
			got, err := store.GetProvider(ctx, tc.family)
			require.NoError(t, err)
			require.Equal(t, manual, got, "explicit administrator configuration must survive")
		})
	}
}

func (c *admissionContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.entered) })
	return err
}

func TestMigrationQueuedImportRereadsCompletion(t *testing.T) {
	for _, cancelQueued := range []bool{false, true} {
		t.Run(map[bool]string{false: "resume", true: "canceled"}[cancelQueued], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			g := migrationCredentials(t)
			putLegacy(t, g, system.OpenAIModelProvider, map[string]string{"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": "key"})
			persisted, release := make(chan struct{}), make(chan struct{})
			var creates, writes atomic.Int64
			var pause sync.Once
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
					writes.Add(1)
					if _, ok := obj.(*v1.LLMGatewayProvider); ok {
						creates.Add(1)
					}
					return c.Create(ctx, obj, opts...)
				},
				Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
					writes.Add(1)
					if err := c.Update(ctx, obj, opts...); err != nil {
						return err
					}
					if resource, ok := obj.(*v1.LLMGatewayConfig); ok {
						var settings map[string]json.RawMessage
						if err := json.Unmarshal([]byte(resource.Spec.Payload), &settings); err != nil {
							return err
						}
						state, err := decodeMigration(settings)
						if err != nil {
							return err
						}
						if state.Sources[system.OpenAIModelProvider].Imported {
							pause.Do(func() {
								close(persisted)
								select {
								case <-release:
								case <-ctx.Done():
								}
							})
						}
					}
					return nil
				},
			}).Build()
			store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
			require.NoError(t, err)
			e := New()
			e.store = store
			first := make(chan error, 1)
			go func() { _, err := e.ImportLegacyModelProviders(ctx, nil, g); first <- err }()
			select {
			case <-persisted:
			case <-ctx.Done():
				t.Fatal("first import did not persist")
			}

			queuedCtx, cancelCall := context.WithCancel(ctx)
			defer cancelCall()
			admission := &admissionContext{Context: queuedCtx, entered: make(chan struct{})}
			queued := make(chan error, 1)
			go func() { _, err := e.ImportLegacyModelProviders(admission, nil, g); queued <- err }()
			select {
			case <-admission.entered:
			case <-ctx.Done():
				t.Fatal("queued import did not enter")
			}
			// The first pass still owns the lock after completion is durable. Delete the
			// aggregate before either call can finish, then let the waiter reload outcomes.
			require.NoError(t, store.DeleteProvider(ctx, "openai"))
			if cancelQueued {
				cancelCall()
			}
			before := writes.Load()
			close(release)
			require.NoError(t, <-first)
			if cancelQueued {
				require.ErrorIs(t, <-queued, context.Canceled)
				require.Equal(t, before, writes.Load(), "canceled admission must not write")
			} else {
				require.NoError(t, <-queued)
			}
			require.EqualValues(t, 1, creates.Load())
			_, err = store.GetProvider(ctx, "openai")
			require.True(t, apierrors.IsNotFound(err))
			state, err := e.MigrationStatus(ctx)
			require.NoError(t, err)
			require.True(t, state.Sources[system.OpenAIModelProvider].Imported)
			if !cancelQueued {
				require.Equal(t, "blocked", state.Sources[system.OpenAIModelProvider].Status)
			}
		})
	}
}

func TestStartupTasksStopAtLeadershipCancellation(t *testing.T) {
	t.Setenv(system.OpenAIAPIKeyEnvVar, "environment-key")
	g := migrationCredentials(t)
	putLegacy(t, g, system.AnthropicModelProvider, map[string]string{"OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY": "legacy-key"})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	writesAfterCancellation, modelWrites := 0, 0
	nextLeader := false
	var activeCancel = cancel
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ModelProviderResource()).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.CreateOption) error {
			if ctx.Err() != nil {
				writesAfterCancellation++
			}
			return c.Create(ctx, obj, opts...)
		},
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if ctx.Err() != nil {
				writesAfterCancellation++
			}
			if err := c.Update(ctx, obj, opts...); err != nil {
				return err
			}
			switch resource := obj.(type) {
			case *v1.LLMGatewayConfig:
				var settings map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(resource.Spec.Payload), &settings))
				state, err := decodeMigration(settings)
				require.NoError(t, err)
				if !nextLeader && state.Sources[system.AnthropicModelProvider].Imported {
					activeCancel()
				}
			case *v1.ModelProvider:
				modelWrites++
				if resource.Annotations[v1.ModelProviderSyncAnnotation] != "" {
					activeCancel()
				}
			}
			return nil
		},
	}).Build()
	store, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	start := func() *Engine {
		e := New()
		require.NoError(t, e.Start(t.Context(), Options{Store: store, DataDir: t.TempDir()}))
		t.Cleanup(func() { require.NoError(t, e.Shutdown(context.Background())) })
		return e
	}
	e := start()
	e.RunStartupTasks(ctx, storage, g)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Zero(t, writesAfterCancellation)
	require.Zero(t, modelWrites, "old leadership must not record completion or trigger model sync after cancellation")
	_, err = store.GetProvider(t.Context(), "openai")
	require.True(t, apierrors.IsNotFound(err), "canceled startup must not proceed to environment import")

	// A fresh engine resumes against shared storage and still applies environment
	// configuration and the initial periodic model-sync trigger automatically.
	nextLeader = true
	ctx, activeCancel = context.WithCancel(t.Context())
	defer activeCancel()
	recovered := start()
	recovered.RunStartupTasks(ctx, storage, g)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Zero(t, writesAfterCancellation)
	require.Positive(t, modelWrites)
	p, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Len(t, p.Keys, 1)
	var mp v1.ModelProvider
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.LLMGatewayModelProvider}, &mp))
	require.True(t, LegacyImportDone(&mp))
	require.Equal(t, "true", mp.Annotations[v1.ModelProviderSyncAnnotation])
}
