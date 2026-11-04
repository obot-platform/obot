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
	store, err := llmcoreclient.New(storage, c)
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{ExtraHeaders: map[string]string{"X-Secret": "header-secret"}}}}))
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
	require.Equal(t, "provider-secret", cfg.Config.Keys[schemas.OpenAI][0].Value.GetValue())
	require.Equal(t, "header-secret", cfg.Config.Providers[schemas.OpenAI].NetworkConfig.ExtraHeaders["X-Secret"])
	require.JSONEq(t, string(settingValue), string(cfg.Config.Settings["sensitive"]))

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

func TestCoreConfigurationWithoutEncryption(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = nil
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, c)
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", llmcoreclient.Provider{Keys: []schemas.Key{{ID: "one", Value: *schemas.NewSecretVar("unencrypted-value")}}}))
	var providers v1.LLMGatewayProviderList
	require.NoError(t, storage.List(t.Context(), &providers))
	require.Len(t, providers.Items, 1)
	var envelope gatewayConfigEnvelope
	require.NoError(t, json.Unmarshal([]byte(providers.Items[0].Spec.Payload), &envelope))
	require.False(t, envelope.Encrypted)
	require.Equal(t, "llm-core", envelope.Context)
	provider, err := store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Equal(t, "unencrypted-value", provider.Keys[0].Value.GetValue())

	// Enabling encryption keeps existing plaintext readable without rewriting it.
	plaintextPayload := providers.Items[0].Spec.Payload
	block, err := aes.NewCipher([]byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	transformer, err := aesstorage.NewGCMTransformer(block)
	require.NoError(t, err)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{credentialGroupResource: transformer}}
	provider, err = store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Equal(t, "unencrypted-value", provider.Keys[0].Value.GetValue())
	require.NoError(t, storage.List(t.Context(), &providers))
	require.Equal(t, plaintextPayload, providers.Items[0].Spec.Payload)

	// A persisted change uses the newly enabled encryption policy.
	require.NoError(t, store.PutKey(t.Context(), "openai", schemas.Key{ID: "one", Value: *schemas.NewSecretVar("rotated-value")}))
	require.NoError(t, storage.List(t.Context(), &providers))
	require.NoError(t, json.Unmarshal([]byte(providers.Items[0].Spec.Payload), &envelope))
	require.True(t, envelope.Encrypted)
	require.NotContains(t, providers.Items[0].Spec.Payload, "rotated-value")
	provider, err = store.GetProvider(t.Context(), "openai")
	require.NoError(t, err)
	require.Equal(t, "rotated-value", provider.Keys[0].Value.GetValue())
}

func TestLoadReportsProviderCodecFailures(t *testing.T) {
	c := newTestClient(t)
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	store, err := llmcoreclient.New(storage, c)
	require.NoError(t, err)
	for _, name := range []string{"healthy", "swapped", "source"} {
		require.NoError(t, store.CreateProvider(t.Context(), name, llmcoreclient.Provider{Keys: []schemas.Key{{ID: "key"}}}))
	}
	var rows v1.LLMGatewayProviderList
	require.NoError(t, storage.List(t.Context(), &rows))
	payload := ""
	for _, row := range rows.Items {
		if row.Spec.ID == "source" {
			payload = row.Spec.Payload
		}
	}
	for i := range rows.Items {
		row := &rows.Items[i]
		if row.Spec.ID == "swapped" {
			row.Spec.Payload = payload
			require.NoError(t, storage.Update(t.Context(), row))
		}
	}
	loaded, err := store.Load(t.Context())
	require.NoError(t, err)
	require.Len(t, loaded.Config.Providers, 2)
	require.Len(t, loaded.ProviderErrors, 1)
	require.ErrorContains(t, loaded.ProviderErrors[0], "identity")
	block, err := aes.NewCipher([]byte(strings.Repeat("k", 32)))
	require.NoError(t, err)
	transformer, err := aesstorage.NewGCMTransformer(block)
	require.NoError(t, err)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{credentialGroupResource: transformer}}
	require.NoError(t, store.CreateProvider(t.Context(), "encrypted", llmcoreclient.Provider{}))
	c.encryptionConfig = nil
	loaded, err = store.Load(t.Context())
	require.NoError(t, err)
	require.Len(t, loaded.Config.Providers, 2)
	require.Len(t, loaded.ProviderErrors, 2)
}
