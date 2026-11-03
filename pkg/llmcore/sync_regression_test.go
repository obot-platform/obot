package llmcore_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type plaintextCodec struct{}

type providerUpdateLogger struct {
	schemas.Logger
	mu       sync.Mutex
	attempts map[string]int
}

func (l *providerUpdateLogger) Info(msg string, args ...any) {
	const prefix = "Updating provider configuration for provider "
	if provider, ok := strings.CutPrefix(msg, prefix); ok {
		l.mu.Lock()
		l.attempts[provider]++
		l.mu.Unlock()
	}
	l.Logger.Info(msg, args...)
}

func (l *providerUpdateLogger) count(provider string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.attempts[provider]
}

func TestPartialSyncDoesNotRestartUpdatedProviders(t *testing.T) {
	store := newTestStore(t)
	require.NoError(t, store.PutProvider(t.Context(), "openai", llmcoreclient.Provider{Config: schemas.ProviderConfig{}}))
	log := &providerUpdateLogger{Logger: bifrost.NewDefaultLogger(schemas.LogLevelWarn), attempts: map[string]int{}}
	e, err := llmcore.New(t.Context(), store, llmcore.Options{Logger: log})
	require.NoError(t, err)
	t.Cleanup(e.Shutdown)
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["proxy"] = json.RawMessage(`{"enabled":false,"url":"http://proxy.example"}`)
		return nil
	}))
	require.NoError(t, store.PutProvider(t.Context(), "broken", llmcoreclient.Provider{}))
	for range 3 {
		changed, err := e.Sync(t.Context())
		require.True(t, changed)
		require.Error(t, err)
	}
	require.Equal(t, 3, log.count("broken"), "the failed provider must retry")
	require.Equal(t, 1, log.count("openai"), "a successful provider update must not restart on retry")
	require.NoError(t, store.DeleteProvider(t.Context(), "broken"))
	changed, err := e.Sync(t.Context())
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = e.Sync(t.Context())
	require.NoError(t, err)
	require.False(t, changed, "successful reconciliation must converge")
	require.Equal(t, 1, log.count("openai"))
}

func (plaintextCodec) SealGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}
func (plaintextCodec) OpenGatewayConfig(_ context.Context, _ string, data []byte) ([]byte, error) {
	return data, nil
}

func newTestStore(t *testing.T) *llmcoreclient.Client {
	t.Helper()
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), plaintextCodec{})
	require.NoError(t, err)
	return store
}
