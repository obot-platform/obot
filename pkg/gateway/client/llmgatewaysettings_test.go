package client

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestLLMGatewaySettingsConcurrentUpdates(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		t.Run(map[bool]string{false: "plaintext", true: "encrypted"}[encrypted], func(t *testing.T) {
			// Separate connections and clients exercise database coordination across replicas.
			dsn := "sqlite://file:" + filepath.Join(t.TempDir(), "settings.db") + "?_journal=WAL&_busy_timeout=5000"
			first := newTestClientWithDSN(t, dsn)
			second := newTestClientWithDSN(t, dsn)
			if encrypted {
				first.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
					Transformers: map[schema.GroupResource]value.Transformer{propertyGroupResource: testTransformer{}},
				}
				second.encryptionConfig = first.encryptionConfig
			}

			updateTogether := func(allowed bool) {
				t.Helper()
				start := make(chan struct{})
				errs := make(chan error, 2)
				go func() {
					<-start
					_, _, err := first.UpdateLLMGatewaySettings(t.Context(), types.LLMGatewaySettingsUpdate{ClaudeSubscriptionPassthrough: &allowed})
					errs <- err
				}()
				go func() {
					<-start
					_, _, err := second.UpdateLLMGatewaySettings(t.Context(), types.LLMGatewaySettingsUpdate{CodexSubscriptionPassthrough: &allowed})
					errs <- err
				}()
				close(start)
				require.NoError(t, <-errs)
				require.NoError(t, <-errs)

				settings, err := first.GetLLMGatewaySettings(t.Context())
				require.NoError(t, err)
				require.Equal(t, types.LLMGatewaySettings{
					ClaudeSubscriptionPassthrough: allowed,
					CodexSubscriptionPassthrough:  allowed,
				}, settings)
			}

			// Includes concurrent first creation, then repeated independent revocations.
			for range 8 {
				updateTogether(true)
				updateTogether(false)
			}
			var stored types.Property
			require.NoError(t, first.db.WithContext(t.Context()).Where("key = ?", llmGatewaySettingsKey).First(&stored).Error)
			require.Equal(t, encrypted, stored.Encrypted)
			if encrypted {
				require.NotContains(t, stored.Value, "subscriptionPassthrough")
			}
		})
	}
}

func TestLLMGatewaySettingsPartialUpdates(t *testing.T) {
	c := newTestClient(t)
	settings, changed, err := c.UpdateLLMGatewaySettings(t.Context(), types.LLMGatewaySettingsUpdate{
		ClaudeSubscriptionPassthrough: new(true),
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, settings.ClaudeSubscriptionPassthrough)
	require.False(t, settings.CodexSubscriptionPassthrough)

	settings, changed, err = c.UpdateLLMGatewaySettings(t.Context(), types.LLMGatewaySettingsUpdate{
		CodexSubscriptionPassthrough: new(true),
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, settings.ClaudeSubscriptionPassthrough)
	require.True(t, settings.CodexSubscriptionPassthrough)

	unchanged, changed, err := c.UpdateLLMGatewaySettings(t.Context(), types.LLMGatewaySettingsUpdate{})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, settings, unchanged)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err = c.UpdateLLMGatewaySettings(ctx, types.LLMGatewaySettingsUpdate{ClaudeSubscriptionPassthrough: new(false)})
	require.Error(t, err)
	unchanged, err = c.GetLLMGatewaySettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, settings, unchanged)
}
