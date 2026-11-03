package client

import (
	"crypto/aes"
	"encoding/json"
	"strings"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
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
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store := llmcoreclient.New(storage, c)
	require.NoError(t, store.PutProvider(t.Context(), "openai", llmcoreclient.Provider{Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{ExtraHeaders: map[string]string{"X-Secret": "header-secret"}}}}))
	require.NoError(t, store.PutKey(t.Context(), "openai", schemas.Key{ID: "key-1", Name: "test", Value: *schemas.NewSecretVar("provider-secret")}))
	settingValue := json.RawMessage(`{"token":"setting-secret"}`)
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["sensitive"] = settingValue
		return nil
	}))

	var providers v1.LLMGatewayProviderList
	require.NoError(t, storage.List(t.Context(), &providers))
	require.Len(t, providers.Items, 1)
	provider := providers.Items[0]
	require.NotContains(t, provider.Spec.Payload, "header-secret")
	require.NotContains(t, provider.Spec.Payload, "provider-secret")
	var settings v1.LLMGatewayConfigList
	require.NoError(t, storage.List(t.Context(), &settings))
	require.NotContains(t, settings.Items[0].Spec.Payload, "setting-secret")

	cfg, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, "provider-secret", cfg.Keys[schemas.OpenAI][0].Value.GetValue())
	require.Equal(t, "header-secret", cfg.Providers[schemas.OpenAI].NetworkConfig.ExtraHeaders["X-Secret"])
	require.JSONEq(t, string(settingValue), string(cfg.Settings["sensitive"]))

	_, err = c.OpenGatewayConfig(t.Context(), "key/wrong", []byte(provider.Spec.Payload))
	require.ErrorContains(t, err, "identity")
	var malformed gatewayConfigEnvelope
	require.NoError(t, json.Unmarshal([]byte(provider.Spec.Payload), &malformed))
	malformed.Secrets["config"] = "unexpected plaintext"
	raw, err := json.Marshal(malformed)
	require.NoError(t, err)
	_, err = c.OpenGatewayConfig(t.Context(), "provider/openai", raw)
	require.ErrorContains(t, err, "malformed encrypted")
	c.encryptionConfig = nil
	_, err = store.Load(t.Context())
	require.ErrorContains(t, err, "encryption provider")
}
