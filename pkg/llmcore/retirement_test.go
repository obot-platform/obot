package llmcore

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type retirementCodec struct{}

func (retirementCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (retirementCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func TestFailedRetirementDoesNotStopApply(t *testing.T) {
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), retirementCodec{})
	require.NoError(t, err)
	for _, provider := range []string{"failed", "healthy"} {
		require.NoError(t, store.CreateProvider(t.Context(), provider, llmcoreclient.Provider{
			Config: schemas.ProviderConfig{
				CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI},
			},
			Keys: []schemas.Key{{ID: "key", Name: "key", Value: *schemas.NewSecretVar("old-secret"), Models: schemas.WhiteList{"*"}, Weight: 1}},
		}))
	}
	engine, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer func() { require.NoError(t, engine.Shutdown(context.Background())) }()
	oldFailed := engine.client.GetProviderByKey("failed")
	oldHealthy := engine.client.GetProviderByKey("healthy")
	for _, provider := range []string{"failed", "healthy"} {
		require.NoError(t, store.MutateProvider(t.Context(), provider, func(p *llmcoreclient.Provider) error {
			p.Config.NetworkConfig.BaseURL = "https://replacement.example"
			p.Keys[0].Value = *schemas.NewSecretVar("new-secret")
			return nil
		}))
	}
	require.NoError(t, store.PutRule(t.Context(), policyRule("new-route")))
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["client"] = json.RawMessage(`{"max_request_body_size_mb":7}`)
		return nil
	}))
	// Inject the rare upstream removal failure at its completion boundary,
	// leaving the real runtime intact as happens when removal fails early.
	failure := errors.New("injected removal failure")
	done := make(chan struct{})
	close(done)
	engine.retirements = map[schemas.ModelProvider]*providerRetirement{
		"failed": {
			done: done,
			err:  failure,
		},
	}
	changed, err := engine.Sync(t.Context())
	require.True(t, changed)
	require.ErrorIs(t, err, failure)
	require.Same(t, oldFailed, engine.client.GetProviderByKey("failed"))
	require.NotSame(t, oldHealthy, engine.client.GetProviderByKey("healthy"), "other providers must still be replaced")
	live := engine.account.config.Load()
	require.Equal(t, "old-secret", live.Keys["failed"][0].Value.Val)
	require.NotEqual(t, "https://replacement.example", live.Providers["failed"].NetworkConfig.BaseURL)
	require.Equal(t, "new-secret", live.Keys["healthy"][0].Value.Val)
	require.Equal(t, "https://replacement.example", live.Providers["healthy"].NetworkConfig.BaseURL)
	require.Equal(t, int64(7<<20), engine.MaxRequestBodyBytes())
	require.Contains(t, engine.applied.Rules, "policy")
	_, blocked := engine.account.transitioning.Load(schemas.ModelProvider("failed"))
	require.True(t, blocked)
	_, blocked = engine.account.transitioning.Load(schemas.ModelProvider("healthy"))
	require.False(t, blocked)
	_, err = engine.Sync(t.Context())
	require.NoError(t, err, "the failed removal is retried")
	require.NotSame(t, oldFailed, engine.client.GetProviderByKey("failed"))
	require.Equal(t, "new-secret", engine.account.config.Load().Keys["failed"][0].Value.Val)
	changed, err = engine.Sync(t.Context())
	require.NoError(t, err)
	require.False(t, changed)
}

func TestRetirementCancellationReusesPendingDrain(t *testing.T) {
	engine := &Engine{}
	release, entered := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer func() {
		releaseOnce.Do(func() { close(release) })
		for _, drain := range engine.retirements {
			<-drain.done
		}
	}()
	var removals atomic.Int32
	remove := func(schemas.ModelProvider) error {
		removals.Add(1)
		close(entered)
		<-release
		return nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	providers := map[schemas.ModelProvider]struct{}{"pending": {}}
	blocked, errs := engine.retireProviders(ctx, providers, remove)
	require.Contains(t, blocked, schemas.ModelProvider("pending"))
	require.ErrorIs(t, errors.Join(errs...), context.Canceled)
	<-entered
	blocked, errs = engine.retireProviders(ctx, providers, remove)
	require.Contains(t, blocked, schemas.ModelProvider("pending"))
	require.ErrorIs(t, errors.Join(errs...), context.Canceled)
	require.EqualValues(t, 1, removals.Load())

	releaseOnce.Do(func() { close(release) })
	<-engine.retirements["pending"].done
	blocked, errs = engine.retireProviders(ctx, providers, remove)
	require.Empty(t, blocked, "completed removals must advance even when the wait budget has expired")
	require.Empty(t, errs)
	require.Empty(t, engine.retirements)
	require.EqualValues(t, 1, removals.Load(), "joining a completed removal must not start another")
}
