package client

import (
	"crypto/aes"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	llmcoretypes "github.com/obot-platform/obot/pkg/llmcore/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	aesstorage "k8s.io/apiserver/pkg/storage/value/encrypt/aes"
)

func TestCoreConfigurationUsesCredentialEncryption(t *testing.T) {
	c := newTestClient(t)
	block, err := aes.NewCipher([]byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	transformer, err := aesstorage.NewGCMTransformer(block)
	require.NoError(t, err)

	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{credentialGroupResource: transformer}}
	store := llmcoreclient.New(c.db.WithContext(t.Context()), c)
	require.NoError(t, store.PutProvider(t.Context(), "openai", schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{ExtraHeaders: map[string]string{"X-Secret": "header-secret"}}}))
	require.NoError(t, store.PutKey(t.Context(), "openai", schemas.Key{ID: "key-1", Name: "test", Value: *schemas.NewSecretVar("provider-secret")}))

	var provider llmcoretypes.Provider
	require.NoError(t, c.db.WithContext(t.Context()).First(&provider).Error)
	require.NotContains(t, provider.Config, "header-secret")

	var key llmcoretypes.ProviderKey
	require.NoError(t, c.db.WithContext(t.Context()).First(&key).Error)
	require.NotContains(t, key.Key, "provider-secret")

	cfg, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, "provider-secret", cfg.Keys[schemas.OpenAI][0].Value.GetValue())
	require.Equal(t, "header-secret", cfg.Providers[schemas.OpenAI].NetworkConfig.ExtraHeaders["X-Secret"])

	_, err = c.OpenGatewayConfig(t.Context(), "key/wrong", []byte(key.Key))
	require.ErrorContains(t, err, "identity")
	c.encryptionConfig = nil
	_, err = store.Load(t.Context())
	require.ErrorContains(t, err, "encryption provider")
}
