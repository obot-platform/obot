package llmengine

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthenticationEditPreservesWrappedSecrets(t *testing.T) {
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: testKeyDNS}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })
	e, ctx := startTestEngine(t)
	for _, provider := range []string{"azure", "bedrock"} {
		_, err := e.CreateProvider(ctx, mustJSON(t, map[string]any{"provider": provider}))
		require.NoError(t, err)
		for _, method := range []string{"api", "credentials"} {
			t.Run(provider+"/"+method, func(t *testing.T) {
				value, config := swapTestCredentials(provider, method, "source")
				key, err := e.CreateKey(ctx, provider, mustJSON(t, map[string]any{
					"name": provider + "-" + method, "enabled": false,
					"value": value, provider + "_key_config": config,
				}))
				require.NoError(t, err)
				view, err := e.GetKey(ctx, provider, key.ID)
				require.NoError(t, err)
				// The dialog keeps unchanged SecretVar wrappers, while editing metadata.
				body := map[string]any{
					"name":  "renamed-" + provider + "-" + method,
					"value": view.Value,
				}
				if provider == "azure" {
					body["azure_key_config"] = view.AzureKeyConfig
				} else {
					body["bedrock_key_config"] = view.BedrockKeyConfig
				}
				_, err = e.UpdateKey(ctx, provider, key.ID, mustJSON(t, body))
				require.NoError(t, err)
				persisted, err := e.store.GetProvider(ctx, provider)
				require.NoError(t, err)
				for _, stored := range persisted.Keys {
					if stored.ID != key.ID {
						continue
					}
					raw := mustJSON(t, stored)
					got, _ := valueAt(raw, []string{"value"})
					require.Equal(t, value, SecretValue(got))
					for field, want := range config {
						got, _ := valueAt(raw, []string{provider + "_key_config", field})
						require.Equal(t, want, SecretValue(got), field)
					}
					return
				}
				t.Fatal("updated key is missing")
			})
		}
	}
}

func TestAzureServerIdentitySwapClearsCustomScopes(t *testing.T) {
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: testKeyDNS}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })
	e, ctx := startTestEngine(t)
	_, err := e.CreateProvider(ctx, []byte(`{"provider":"azure"}`))
	require.NoError(t, err)
	key, err := e.CreateKey(ctx, "azure", []byte(`{
		"name":"scoped","enabled":false,"value":"",
		"azure_key_config":{"endpoint":"https://source.openai.azure.com",
		"client_id":"client","client_secret":"secret","tenant_id":"tenant",
		"scopes":["https://ai.azure.com/.default"]}}`))
	require.NoError(t, err)
	_, err = e.UpdateKey(ctx, "azure", key.ID, []byte(`{
		"value":"","azure_key_config":{"endpoint":"https://source.openai.azure.com",
		"client_id":"","client_secret":"","tenant_id":"","scopes":[]}}`))
	require.NoError(t, err)
	persisted, err := e.store.GetProvider(ctx, "azure")
	require.NoError(t, err)
	require.Len(t, persisted.Keys, 1)
	require.Empty(t, persisted.Keys[0].AzureKeyConfig.Scopes)
	require.Empty(t, persisted.Keys[0].AzureKeyConfig.ClientID.GetValue())
	require.Empty(t, persisted.Keys[0].AzureKeyConfig.ClientSecret.GetValue())
	require.Empty(t, persisted.Keys[0].AzureKeyConfig.TenantID.GetValue())
}

func TestAuthenticationEditKeepsExistingReferenceMetadata(t *testing.T) {
	t.Setenv("OBOT_TEST_EDIT_KEY", "stored-reference-secret")
	e, ctx := startTestEngine(t)
	_, err := e.CreateProvider(ctx, []byte(`{"provider":"openai"}`))
	require.NoError(t, err)
	// Startup-imported credentials can reference the server's environment.
	key, err := e.CreateKey(ctx, "openai", []byte(`{"name":"referenced","enabled":false,"value":"env.OBOT_TEST_EDIT_KEY"}`))
	require.NoError(t, err)
	view, err := e.GetKey(ctx, "openai", key.ID)
	require.NoError(t, err)
	_, err = e.UpdateKey(ctx, "openai", key.ID, mustJSON(t, map[string]any{
		"name": "renamed", "value": map[string]string{"value": view.Value.GetValue()},
	}))
	require.NoError(t, err)
	persisted, err := e.store.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.True(t, persisted.Keys[0].Value.IsFromSecret())
	require.Equal(t, "stored-reference-secret", persisted.Keys[0].Value.GetValue())
}
